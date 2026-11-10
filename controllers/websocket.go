package controllers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"example.com/live-stream/internal/auth"
	"example.com/live-stream/internal/cache"
	"example.com/live-stream/internal/kafka"
	"example.com/live-stream/internal/model"
	"example.com/live-stream/internal/service"
	"example.com/live-stream/internal/ws"
	"github.com/beego/beego/v2/server/web"
	beecontext "github.com/beego/beego/v2/server/web/context"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

// 根据上下文和 redis中client 以及sid确认连接状态 判断是否用户或者是游客 游客返回0，用户则返回其userid
func websocketUserID(ctx context.Context, client *redis.Client, sid string) (int64, error) {
	if sid == "" {
		return 0, nil
	}

	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return auth.GetSessionUserID(lookupCtx, client, sid)
}

func NewWebSocketHandler(redisClient *redis.Client, hub *ws.Hub, producer *kafka.Producer, bus *service.RoomBus) web.HandleFunc {

	submitter := service.NewDanmaSubmitter(redisClient, producer)

	activity := cache.NewRoomActivity(redisClient)
	recordActivity := func(
		parent context.Context,
		roomID int64,
		kind cache.ActivityKind,
		at time.Time,
	) {
		metricCtx, cancel := context.WithTimeout(parent, 200*time.Millisecond)
		defer cancel()
		if err := activity.Record(metricCtx, roomID, kind, at); err != nil {
			log.Printf("room activity room=%d kind=%s: %v", roomID, kind, err)
		}
	}

	upgrader := websocket.Upgrader{
		HandshakeTimeout: 5 * time.Second,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			return origin == "" ||
				origin == "http://127.0.0.1:13000" ||
				origin == "http://localhost:13000"
		},
	}

	return func(ctx *beecontext.Context) {
		//握手失败时，返回 HTTP JSON 错误给前端。
		fail := func(status int, message string) {
			ctx.Output.SetStatus(status)
			_ = ctx.JSONResp(map[string]string{"message": message})
		}
		//从请求中拿房间id
		roomID, err := strconv.ParseInt(ctx.Input.Param(":room_id"), 10, 64)

		if err != nil || roomID <= 0 {
			fail(http.StatusBadRequest, "房间 ID 必须是正整数")
			return
		}

		//从beego的上下文中取出请求对象
		requestCtx := ctx.Request.Context()
		room, err := service.GetRoomDetail(requestCtx, roomID)

		if errors.Is(err, service.ErrRoomNotFound) {
			fail(http.StatusNotFound, "房间不存在")
			return
		}

		if err != nil {
			log.Printf("websocket find room: %v", err)
			fail(http.StatusInternalServerError, "查询房间失败")
			return
		}
		if room.Status != model.RoomStatusLive {
			fail(http.StatusConflict, "房间尚未开播")
			return
		}

		//从请求 Cookie 拿 `sid`，查登录用户 ID
		sid := ctx.GetCookie("sid")
		userID, err := websocketUserID(requestCtx, redisClient, sid)
		if errors.Is(err, auth.ErrInvalidSession) {
			fail(http.StatusUnauthorized, "登录已失效，请重新登录")
			return
		}
		if err != nil {
			log.Printf("websocket lookup session: %v", err)
			fail(http.StatusInternalServerError, "查询会话失败")
			return
		}
		// 将http升级为websocket
		/*
					http与websocket的取舍：
					hhtp是一问一答；**一次握手，之后双工长连接**
			先通过一次 HTTP 请求做握手，握手成功返回 `101 Switching Protocols`，协议切换，TCP 连接**不断开，持续复用**

		*/
		conn, err := upgrader.Upgrade(ctx.ResponseWriter, ctx.Request, nil)
		if err != nil {
			log.Printf("websocket upgrade: %v", err)
			return
		}
		ctx.ResponseWriter.Started = true
		ctx.ResponseWriter.Status = http.StatusSwitchingProtocols

		// 使用房间id和用户id建立连接，每个client对应一个用户
		client := ws.NewClient(conn, roomID, userID)
		client.Send(ws.ServerMessage{
			Type:    ws.TypeSystem,
			RoomID:  roomID,
			Content: "已进入房间",
			SentAt:  time.Now().UTC(),
		})
		// 如果没有新增连接就返回
		if !hub.Add(client) {
			return
		}
		//发完信息最后关闭连接
		defer hub.Remove(client)

		writerDone := make(chan struct{})
		// 往client写信息
		go func() {
			defer close(writerDone)
			if err := client.WriteLoop(); err != nil {
				log.Printf("websocket write room=%d: %v", roomID, err)
			}
		}()

		defer func() {
			client.Close()
			<-writerDone
		}()

		sendError := func(
			input ws.ClientMessage,
			code, content string,
			retryable bool,
		) {
			messageID := ""
			if input.Type == ws.TypeDanma {
				messageID = input.MessageID
			}

			client.Send(ws.ServerMessage{
				Type:      ws.TypeError,
				RoomID:    roomID,
				MessageID: messageID,
				Code:      code,
				Retryable: &retryable,
				Content:   content,
				SentAt:    time.Now().UTC(),
			})
		}
		// 从client中取信息

		var nextLikeAt time.Time
		err = client.ReadLoop(func(message ws.ClientMessage) {
			if message.Type != ws.TypeDanma && message.Type != ws.TypeLike {

				sendError(message, "unsupported_type", "不支持的消息类型", false)
				return
			}
			// 游客登录才可以放弹幕
			if client.UserID == 0 {
				sendError(message, "unauthenticated", "请先登录后发送互动消息", false)
				return
			}
			//二次鉴权
			currentID, err := websocketUserID(requestCtx, redisClient, sid)
			if errors.Is(err, auth.ErrInvalidSession) {
				sendError(message, "session_expired", "登录已失效，请重新登录", false)

				return
			}
			if err != nil {
				log.Printf("websocket validate session: %v", err)

				sendError(message, "session_unavailable", "会话校验失败，请稍后重试", true)
				return
			}
			if currentID != client.UserID {

				sendError(message, "identity_changed", "登录身份已变化，请重新连接", false)
				return
			}
			if message.Type == ws.TypeLike {
				now := time.Now()

				if now.Before(nextLikeAt) {

					sendError(message, "like_rate_limited", "点赞太频繁，请稍后再试", false)
					return
				}

				nextLikeAt = now.Add(200 * time.Millisecond)

				likeCtx, cancel := context.WithTimeout(
					requestCtx,
					3*time.Second,
				) // 超时处理 取弹幕的时间不能太长；超频不执行数据库更新。

				total, err := service.LikeRoom(
					likeCtx,
					client.RoomID,
					client.UserID,
				)

				cancel()

				if errors.Is(err, service.ErrInvalidLikeIdentity) ||
					errors.Is(err, service.ErrRoomCannotLike) {

					sendError(message, "like_rejected", err.Error(), false)

					return
				}

				if err != nil {
					log.Printf(
						"like room=%d user=%d: %v",
						client.RoomID,
						client.UserID,
						err,
					)

					sendError(message,
						"点赞保存失败，请稍后重试",
						err.Error(),
						false)
					return
				}

				publishCtx, publishCancel := context.WithTimeout(requestCtx, time.Second)
				publishErr := bus.Publish(publishCtx, ws.ServerMessage{
					Type: ws.TypeRoomState, RoomID: client.RoomID,
					LikeCount: &total, SentAt: time.Now().UTC(),
				})
				publishCancel()
				if publishErr != nil {
					log.Printf("like broadcast room=%d: %v", client.RoomID, publishErr)
					client.Send(ws.ServerMessage{
						Type: ws.TypeRoomState, RoomID: client.RoomID,
						LikeCount: &total, SentAt: time.Now().UTC(),
					})
					client.Send(ws.ServerMessage{
						Type: ws.TypeSystem, RoomID: client.RoomID,
						Content: "点赞已保存，广播暂不可用，请刷新点赞数", SentAt: time.Now().UTC(),
					})
				}

				recordActivity(requestCtx, client.RoomID, cache.ActivityLike, time.Now().UTC())
				return
			}

			submitCtx, cancel := context.WithTimeout(requestCtx, 5*time.Second)
			result, err := submitter.Submit(
				submitCtx, client.RoomID, client.UserID,
				service.DanmaInput{MessageID: message.MessageID, Content: message.Content},
			)
			cancel()
			if errors.Is(err, service.ErrDanmaConflict) {
				sendError(message, "danma_conflict", err.Error(), false)
				return
			}
			if errors.Is(err, service.ErrInvalidDanmaID) ||
				errors.Is(err, service.ErrInvalidDanmaContent) ||
				errors.Is(err, service.ErrInvalidDanmaIdentity) {
				sendError(message, "invalid_danma", err.Error(), false)
				return
			}
			if err != nil {
				log.Printf("submit danma room=%d user=%d: %v", client.RoomID, client.UserID, err)
				sendError(message, "danma_submit_unavailable", "暂时无法确认接收结果，请使用原消息重试", true)
				return
			}
			if saved := result.Saved; saved != nil {
				client.Send(ws.ServerMessage{
					Type: ws.TypeAck, ID: saved.ID, MessageID: saved.MessageID,
					RoomID: saved.RoomID, UserID: saved.UserID,
					Content: saved.Content, SentAt: saved.SentAt,
				})
				return
			}
			event := result.Event
			client.Send(ws.ServerMessage{
				Type: ws.TypeAccepted, MessageID: event.MessageID,
				RoomID: event.RoomID, UserID: event.UserID,
				Content: event.Content, SentAt: event.OccurredAt,
			})
		})
		if err != nil {
			log.Printf("websocket read ended room=%d user=%d: %v", roomID, userID, err)
		}
	}
}
