package kafka

import "time"

// DanmaEvent 是后端校验后写入 Kafka 的事件。
type DanmaEvent struct {
	MessageID  string    `json:"message_id"`
	RoomID     int64     `json:"room_id"`
	UserID     int64     `json:"user_id"`
	Content    string    `json:"content"`
	OccurredAt time.Time `json:"occurred_at"`
}
