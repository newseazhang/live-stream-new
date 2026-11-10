package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"example.com/live-stream/filters"
	"example.com/live-stream/internal/model"
	"example.com/live-stream/internal/service"
	beecontext "github.com/beego/beego/v2/server/web/context"
)

// 从前端请求中读上下文
func CreateRoom(ctx *beecontext.Context) {
	ownerID, ok := filters.CurrentUserID(ctx)
	if !ok {
		ctx.Output.SetStatus(http.StatusUnauthorized)
		_ = ctx.JSONResp(map[string]string{"message": "请先登录"})
		return
	}
	var req struct {
		Title string `json:"title"`
	}

	body := http.MaxBytesReader(ctx.ResponseWriter, ctx.Request.Body, 4096)
	defer body.Close()

	raw, err := io.ReadAll(body)
	if err != nil || json.Unmarshal(raw, &req) != nil {
		ctx.Output.SetStatus(http.StatusBadRequest)
		_ = ctx.JSONResp(map[string]string{"message": "无效的请求体"})
		return
	}
	// 创建房间
	room, err := service.CreateRoom(ctx.Request.Context(), ownerID, req.Title)

	switch {
	case errors.Is(err, service.ErrInvalidRoomTitle):
		ctx.Output.SetStatus(http.StatusBadRequest)
		_ = ctx.JSONResp(map[string]string{"message": err.Error()})
	case errors.Is(err, service.ErrRoomAlreadyExists):
		ctx.Output.SetStatus(http.StatusConflict)
		_ = ctx.JSONResp(map[string]string{"message": err.Error()})
	case err != nil:
		log.Printf("create room: %v", err)
		ctx.Output.SetStatus(http.StatusInternalServerError)
		_ = ctx.JSONResp(map[string]string{"message": "创建房间失败"})
	default:
		ctx.Output.SetStatus(http.StatusCreated)
		_ = ctx.JSONResp(map[string]any{
			"room_id":    room.RoomID,
			"owner_id":   room.OwnerID,
			"room_title": room.RoomTitle,
			"status":     room.Status,
			"start_at":   nil,
			"like_count": room.LikeCount,
		})
	}
}



type roomResponse struct {
	RoomID    int64      `json:"room_id"`
	OwnerID   int64      `json:"owner_id"`
	RoomTitle string     `json:"room_title"`
	Status    int        `json:"status"`
	StartAt   *time.Time `json:"start_at"`
	LikeCount int64      `json:"like_count"`
}


func toRoomResponse(room *model.Room) *roomResponse {
	if room == nil {
		return nil
	}

	response := &roomResponse{
		RoomID:    room.RoomID,
		OwnerID:   room.OwnerID,
		RoomTitle: room.RoomTitle,
		Status:    room.Status,
		LikeCount: room.LikeCount,
	}
	if !room.StartAt.IsZero() {
		response.StartAt = &room.StartAt
	}
	return response
}

func GetMyRoom(ctx *beecontext.Context) {
	//鉴权
	ownerID, ok := filters.CurrentUserID(ctx)
	if !ok {
		ctx.Output.SetStatus(http.StatusUnauthorized)
		_ = ctx.JSONResp(map[string]string{"message": "请先登录"})
		return
	}

	room, err := service.GetMyRoom(ctx.Request.Context(), ownerID)
	if err != nil {
		log.Printf("get my room: %v", err)
		ctx.Output.SetStatus(http.StatusInternalServerError)
		_ = ctx.JSONResp(map[string]string{"message": "查询房间失败"})
		return
	}

	_ = ctx.JSONResp(map[string]*roomResponse{"data": toRoomResponse(room)})
}


// 解析路径参数并返回 JSON
func GetRoomDetail(ctx *beecontext.Context) {
	roomID, err := strconv.ParseInt(ctx.Input.Param(":room_id"), 10, 64)
	if err != nil || roomID <= 0 {
		ctx.Output.SetStatus(http.StatusBadRequest)
		_ = ctx.JSONResp(map[string]string{"message": "房间 ID 必须是正整数"})
		return
	}

	room, err := service.GetRoomDetail(ctx.Request.Context(), roomID)
	switch {
	case errors.Is(err, service.ErrRoomNotFound):
		ctx.Output.SetStatus(http.StatusNotFound)
		_ = ctx.JSONResp(map[string]string{"message": err.Error()})
	case err != nil:
		log.Printf("get room detail: %v", err)
		ctx.Output.SetStatus(http.StatusInternalServerError)
		_ = ctx.JSONResp(map[string]string{"message": "查询房间失败"})
	default:
		_ = ctx.JSONResp(map[string]*roomResponse{"data": toRoomResponse(room)})
	}
}



func ListRooms(ctx *beecontext.Context) {
	rooms, err := service.ListRooms(ctx.Request.Context())
	if err != nil {
		log.Printf("list rooms: %v", err)
		ctx.Output.SetStatus(http.StatusInternalServerError)
		_ = ctx.JSONResp(map[string]string{"message": "查询房间列表失败"})
		return
	}

	data := make([]*roomResponse, 0, len(rooms))
	for i := range rooms {
		data = append(data, toRoomResponse(&rooms[i]))
	}

	_ = ctx.JSONResp(map[string][]*roomResponse{"data": data})
}

func StartRoom(ctx *beecontext.Context) {
	userID, ok := filters.CurrentUserID(ctx)
	if !ok {
		ctx.Output.SetStatus(http.StatusUnauthorized)
		_ = ctx.JSONResp(map[string]string{"message": "请先登录"})
		return
	}

	roomID, err := strconv.ParseInt(ctx.Input.Param(":room_id"), 10, 64)
	if err != nil || roomID <= 0 {
		ctx.Output.SetStatus(http.StatusBadRequest)
		_ = ctx.JSONResp(map[string]string{"message": "房间 ID 必须是正整数"})
		return
	}

	err = service.StartRoom(ctx.Request.Context(), roomID, userID)
	switch {
	case err == nil:
		ctx.ResponseWriter.WriteHeader(http.StatusNoContent)
		return
	case errors.Is(err, service.ErrRoomNotFound):
		ctx.Output.SetStatus(http.StatusNotFound)
	case errors.Is(err, service.ErrNotRoomOwner):
		ctx.Output.SetStatus(http.StatusForbidden)
	case errors.Is(err, service.ErrCannotStart):
		ctx.Output.SetStatus(http.StatusConflict)
	default:
		log.Printf("start room: %v", err)
		ctx.Output.SetStatus(http.StatusInternalServerError)
		_ = ctx.JSONResp(map[string]string{"message": "开播失败"})
		return
	}

	_ = ctx.JSONResp(map[string]string{"message": err.Error()})
}