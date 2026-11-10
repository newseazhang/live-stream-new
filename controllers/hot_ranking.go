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

type hotRoomItem struct {
	RoomID    int64   `json:"room_id"`
	HeatScore float64 `json:"heat_score"`
}

func NewHotRankingHandler(redisClient *redis.Client) web.HandleFunc {
	ranking := cache.NewHotRanking(redisClient)

	return func(ctx *beecontext.Context) {
		ctx.Output.Header("Cache-Control", "no-store")

		fail := func(status int, message string) {
			ctx.Output.SetStatus(status)
			_ = ctx.JSONResp(map[string]string{
				"message": message,
			})
		}

		limit := 10
		if raw := ctx.Input.Query("limit"); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil {
				fail(
					http.StatusBadRequest,
					service.ErrInvalidHotLimit.Error(),
				)
				return
			}
			limit = value
		}

		queryCtx, cancel := context.WithTimeout(
			ctx.Request.Context(), time.Second,
		)
		defer cancel()

		rooms, err := service.GetHotRooms(
			queryCtx, ranking, limit,
		)
		if errors.Is(err, service.ErrInvalidHotLimit) {
			fail(http.StatusBadRequest, err.Error())
			return
		}
		if err != nil {
			log.Printf("get hot rooms: %v", err)
			fail(
				http.StatusServiceUnavailable,
				"热度榜暂时不可用，请稍后重试",
			)
			return
		}

		data := make([]hotRoomItem, 0, len(rooms))
		for _, room := range rooms {
			data = append(data, hotRoomItem{
				RoomID:    room.RoomID,
				HeatScore: room.Score,
			})
		}

		_ = ctx.JSONResp(map[string]interface{}{
			"data": data,
		})
	}
}