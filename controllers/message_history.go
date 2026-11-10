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

type messageHistoryItem struct {
	ID        int64     `json:"id"`
	Type      string    `json:"type"`
	MessageID string    `json:"message_id"`
	RoomID    int64     `json:"room_id"`
	UserID    int64     `json:"user_id"`
	Content   string    `json:"content"`
	SentAt    time.Time `json:"sent_at"`
}

func NewMessageHistoryHandler(redisClient *redis.Client) web.HandleFunc {
	recent := cache.NewRecentMessages(redisClient)
	return func(ctx *beecontext.Context) {
		fail := func(status int, message string) {
			ctx.Output.SetStatus(status)
			_ = ctx.JSONResp(map[string]string{
				"message": message,
			})
		}

		roomID, err := strconv.ParseInt(
			ctx.Input.Param(":room_id"),
			10,
			64,
		)
		if err != nil || roomID <= 0 {
			fail(http.StatusBadRequest, "房间 ID 必须是正整数")
			return
		}

		var beforeID int64
		if raw := ctx.Input.Query("before_id"); raw != "" {
			beforeID, err = strconv.ParseInt(raw, 10, 64)
			if err != nil || beforeID <= 0 {
				fail(http.StatusBadRequest, "before_id 必须是正整数")
				return
			}
		}

		limit := 20
		if raw := ctx.Input.Query("limit"); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 100 {
				fail(http.StatusBadRequest, "limit 必须是 1–100 的整数")
				return
			}
			limit = value
		}
		// 超时取消
		queryCtx, cancel := context.WithTimeout(
			ctx.Request.Context(),
			3*time.Second,
		)
		defer cancel()

		page, err := service.GetRoomMessages(
			queryCtx,
			recent, // 补上这一项
			roomID,
			beforeID,
			limit,
		)

		if errors.Is(err, service.ErrRoomNotFound) {
			fail(http.StatusNotFound, "房间不存在")
			return
		}
		if errors.Is(err, service.ErrInvalidMessageQuery) {
			fail(http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			log.Printf("get room messages room=%d: %v", roomID, err)
			fail(http.StatusInternalServerError, "查询弹幕历史失败")
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
				ID:        message.ID,
				Type:      "danma",
				MessageID: message.MessageID,
				RoomID:    message.RoomID,
				UserID:    message.UserID,
				Content:   message.Content,
				SentAt:    message.SentAt.UTC(),
			})
		}

		_ = ctx.JSONResp(map[string]interface{}{
			"data":           data,
			"has_more":       page.HasMore,
			"next_before_id": page.NextBeforeID,
		})
	}
}
