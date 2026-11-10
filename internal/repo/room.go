package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"example.com/live-stream/internal/model"
	"github.com/beego/beego/v2/client/orm"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var ErrRoomOwnerConflict = errors.New("房主已经在线")

func InsertRoom(ctx context.Context, room *model.Room) error {
	_, err := orm.NewOrm().InsertWithCtx(ctx, room)
	if err == nil {
		return nil
	}

	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		return ErrRoomOwnerConflict
	}

	return fmt.Errorf("insert room: %w", err)
}


func FindRoomByOwnerID(ctx context.Context, ownerID int64) (*model.Room, error) {
	room := &model.Room{OwnerID: ownerID}
	err := orm.NewOrm().ReadWithCtx(ctx, room, "OwnerID")
	if errors.Is(err, orm.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find room by owner: %w", err)
	}
	return room, nil
}


func FindRoomByID(ctx context.Context, roomID int64) (*model.Room, error) {
	room := &model.Room{RoomID: roomID}

	err := orm.NewOrm().ReadWithCtx(ctx, room)
	if errors.Is(err, orm.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find room by ID: %w", err)
	}
	return room, nil
}

// 查询开播中的房间
func ListLiveRooms(ctx context.Context, limit int) ([]model.Room, error) {
	var rooms []model.Room

	_, err := orm.NewOrm().QueryTable(new(model.Room)).
		Filter("status", model.RoomStatusLive).
		OrderBy("-start_at", "-room_id").
		Limit(limit).
		AllWithCtx(ctx, &rooms)
	if err != nil {
		return nil, fmt.Errorf("list live rooms: %w", err)
	}
	return rooms, nil
}


//开播功能

func StartRoom(ctx context.Context,roomID ,ownerID int64) (bool,error){
	now:=time.Now().UTC()
	affected, err := orm.NewOrm().QueryTable(new(model.Room)).
		Filter("room_id", roomID).
		Filter("owner_id", ownerID).
		Filter("status", model.RoomStatusOffline).
		UpdateWithCtx(ctx, orm.Params{
			"status":     model.RoomStatusLive,
			"start_at":   now,
			"updated_at": now,
		})
	if err != nil {
		return false, fmt.Errorf("start room: %w", err)
	}
	return affected == 1, nil
}