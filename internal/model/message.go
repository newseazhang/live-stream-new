package model

import "time"

type Message struct {
	ID        int64     `orm:"column(id);auto"`
	MessageID string    `orm:"column(message_id);size(36);unique"`
	RoomID    int64     `orm:"column(room_id)"`
	UserID    int64     `orm:"column(user_id)"`
	Content   string    `orm:"column(content);type(text)"`
	SentAt    time.Time `orm:"column(sent_at);type(datetime)"`
}

func (m *Message) TableName() string {
	return "messages"
}

func (m *Message) TableIndex() [][]string {
	return [][]string{
		{"RoomID", "ID"},
	}
}
