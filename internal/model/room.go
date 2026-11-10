package model

import "time"

// 房间状态
const (
	RoomStatusOffline = 0
	RoomStatusLive    = 1
)

type Room struct {
	RoomID    int64     `orm:"column(room_id);pk;auto"`
	OwnerID   int64     `orm:"column(owner_id);unique"`
	RoomTitle string    `orm:"column(room_title);size(120)"`
	Status    int       `orm:"column(status);default(0)"`
	StartAt   time.Time `orm:"column(start_at);null"`
	LikeCount int64     `orm:"column(like_count);default(0)"`
	CreatedAt time.Time `orm:"column(created_at);auto_now_add"`
	UpdatedAt time.Time `orm:"column(updated_at);auto_now"`
}

func (*Room) TableName() string {
	return "rooms"
}
