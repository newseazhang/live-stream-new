package controllers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"example.com/live-stream/internal/cache"
	"example.com/live-stream/internal/service"
	"github.com/beego/beego/v2/server/web"
	beecontext "github.com/beego/beego/v2/server/web/context"
	"github.com/redis/go-redis/v9"
)

func NewMessageSyncHandler(redisClient *redis.Client) web.HandleFunc {
	recent := cache.NewRecentMessages(redisClient)
	return func(ctx *beecontext.Context) {
		ctx.Output.Header("Cache-Control", "no-store")
		fail := func(status int, message string) {
			ctx.Output.SetStatus(status)
			_ = ctx.JSONResp(map[string]string{"message": message})
		}
		roomID, err := strconv.ParseInt(ctx.Input.Param(":room_id"), 10, 64)
		if err != nil || roomID <= 0 {
			fail(http.StatusBadRequest, "房间 ID 必须是正整数")
			return
		}
		query := ctx.Request.URL.Query()
		afterID, err := strconv.ParseInt(query.Get("after_id"), 10, 64)
		if err != nil || afterID < 0 {
			fail(http.StatusBadRequest, "after_id 必填，且必须是非负整数")
			return
		}
		var untilID *int64
		if query.Has("until_id") {
			value, err := strconv.ParseInt(query.Get("until_id"), 10, 64)
			if err != nil || value < 0 {
				fail(http.StatusBadRequest, "until_id 必须是非负整数")
				return
			}
			untilID = &value
		}
		limit := 20
		if query.Has("limit") {
			value, err := strconv.Atoi(query.Get("limit"))
			if err != nil || value < 1 || value > 100 {
				fail(http.StatusBadRequest, "limit 必须是 1–100 的整数")
				return
			}
			limit = value
		}
		queryCtx, cancel := context.WithTimeout(ctx.Request.Context(), 3*time.Second)
		defer cancel()
		page, err := service.GetRoomMessageChanges(queryCtx, recent, roomID, afterID, untilID, limit)
		if errors.Is(err, service.ErrRoomNotFound) {
			fail(http.StatusNotFound, "房间不存在")
			return
		}
		if errors.Is(err, service.ErrInvalidMessageSyncQuery) {
			fail(http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			log.Printf("sync room messages room=%d: %v", roomID, err)
			fail(http.StatusInternalServerError, "补拉消息失败，请重试")
			return
		}
		source := "MISS"
		if len(page.Messages) == 0 {
			source = "EMPTY"
		} else if page.CacheHit {
			source = "HIT"
		}
		ctx.Output.Header("X-Message-Cache", source)
		data := make([]messageHistoryItem, 0, len(page.Messages))
		for _, message := range page.Messages {
			data = append(data, messageHistoryItem{
				ID: message.ID, Type: "danma", MessageID: message.MessageID,
				RoomID: message.RoomID, UserID: message.UserID,
				Content: message.Content, SentAt: message.SentAt.UTC(),
			})
		}
		_ = ctx.JSONResp(map[string]interface{}{
			"data": data, "has_more": page.HasMore,
			"next_after_id": page.NextAfterID, "until_id": page.UntilID,
		})
	}
}