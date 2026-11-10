package service

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"example.com/live-stream/internal/model"
	"example.com/live-stream/internal/repo"
)

var (
	ErrInvalidRoomTitle  = errors.New("房间标题须为 1–120 个字符")
	ErrRoomAlreadyExists = errors.New("当前账号已有房间")
	ErrRoomNotFound = errors.New("房间不存在")
)

func CreateRoom(ctx context.Context, ownerID int64, title string) (*model.Room, error) {
	title = strings.TrimSpace(title)

	if title == "" || utf8.RuneCountInString(title) > 120 {
		return nil, ErrInvalidRoomTitle
	}

	room := &model.Room{
		OwnerID:   ownerID,
		RoomTitle: title,
		Status:    model.RoomStatusOffline,
	}

	err := repo.InsertRoom(ctx, room)
	if errors.Is(err, repo.ErrRoomOwnerConflict) {
		return nil, ErrRoomAlreadyExists
	}
	if err != nil {
		return nil, err
	}
	return room, nil
}

// 按登录用户 ID 查询，直接调用 Repository。
func GetMyRoom(ctx context.Context, ownerID int64) (*model.Room, error) {
	return repo.FindRoomByOwnerID(ctx, ownerID)
}

//房间详情
func GetRoomDetail(ctx context.Context, roomID int64) (*model.Room, error) {
	room, err := repo.FindRoomByID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if room == nil {
		return nil, ErrRoomNotFound
	}
	return room, nil
}

const publicRoomLimit = 20

func ListRooms(ctx context.Context) ([]model.Room, error) {
	return repo.ListLiveRooms(ctx, publicRoomLimit)
}



//开播
var (
	ErrNotRoomOwner = errors.New("只有房主可以操作房间")
	ErrCannotStart  = errors.New("房间当前无法开播")
)

func StartRoom(ctx context.Context, roomID, userID int64) error {
	started, err := repo.StartRoom(ctx, roomID, userID)
	if err != nil {
		return err
	}
	if started {
		return nil
	}

	room, err := GetRoomDetail(ctx, roomID)
	if err != nil {
		return err
	}
	if room.OwnerID != userID {
		return ErrNotRoomOwner
	}
	return ErrCannotStart
}


