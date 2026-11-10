package ws

import "time"

const (
	TypeDanma     = "danma"
	TypeSystem    = "system"
	TypeError     = "error"
	TypeLike      = "like"
	TypeRoomState = "room_state"
	TypeAck       = "ack"
	TypeAccepted = "accepted"
)

type ClientMessage struct {
	Type      string `json:"type"`
	MessageID string `json:"message_id"`
	Content   string `json:"content"`
}

type ServerMessage struct {
	Type        string    `json:"type"`
	MessageID   string    `json:"message_id,omitempty"` // 客户端生成的 UUID，同一次发送重试时保持不变。
	RoomID      int64     `json:"room_id"`
	UserID      int64     `json:"user_id,omitempty"`
	Content     string    `json:"content"`
	SentAt      time.Time `json:"sent_at"`
	LikeCount   *int64    `json:"like_count,omitempty"`
	OnlineCount *int      `json:"online_count,omitempty"` //在线人数
	ID          int64     `json:"id,omitempty"`           //SQLite 主键，由服务端保存后返回，用于排序和后续历史游标。

	Code      string `json:"code,omitempty"`      // 稳定的错误标识
	Retryable *bool  `json:"retryable,omitempty"` //是否允许原请求重试 加*之后由之前bool的两态变成三态，表示可选参数 如果不传参数就为nil，会走默认设置
}
