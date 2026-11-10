package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"example.com/live-stream/internal/cache"
	"example.com/live-stream/internal/kafka"
	"example.com/live-stream/internal/model"
	"example.com/live-stream/internal/repo"
	"github.com/redis/go-redis/v9"
)

type DanmaSubmission struct {
	Event kafka.DanmaEvent
	Saved *model.Message
}

type DanmaSubmitter struct {
	bindings *cache.DanmaSubmissions
	producer *kafka.Producer
}

func NewDanmaSubmitter(client *redis.Client, producer *kafka.Producer) *DanmaSubmitter {
	return &DanmaSubmitter{bindings: cache.NewDanmaSubmissions(client), producer: producer}
}

func (s *DanmaSubmitter) Submit(ctx context.Context, roomID, userID int64, input DanmaInput) (DanmaSubmission, error) {
	if roomID <= 0 || userID <= 0 {
		return DanmaSubmission{}, ErrInvalidDanmaIdentity
	}
	input, err := ValidateDanma(input)
	if err != nil {
		return DanmaSubmission{}, err
	}

	//// 【第一重幂等：数据库（SQLite）作为最终可信源】
	// 已保存记录始终以 SQLite 为准，兼容迁移前的 UUID 及绑定过期后的重试。
	saved, err := repo.FindMessageByMessageID(ctx, input.MessageID)
	if err != nil {
		return DanmaSubmission{}, fmt.Errorf("查询弹幕保存结果: %w", err)
	}
	// 数据库里已经存在这个MessageID
	// 校验：库里这条记录，和当前提交的房间、用户、弹幕内容必须完全一致
	// 如果不一样 → ErrDanmaConflict：同一个MessageID提交了不同内容，属于恶意篡改/异常冲突，直接拒绝
	if saved != nil {
		if saved.RoomID != roomID || saved.UserID != userID || saved.Content != input.Content {
			return DanmaSubmission{}, ErrDanmaConflict
		}
		// 数据完全匹配，直接返回已保存记录，不再发Kafka
		return DanmaSubmission{Saved: saved}, nil
	}
	// 数据库没有这条消息 → 组装Kafka事件结构体
	event := kafka.DanmaEvent{
		MessageID: input.MessageID, RoomID: roomID, UserID: userID,
		Content: input.Content, OccurredAt: time.Now().UTC(),
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return DanmaSubmission{}, err
	}

	// 【第二重幂等：Redis Lua原子Bind】
	raw, err := s.bindings.Bind(ctx, event.MessageID, string(payload))
	if err != nil {
		return DanmaSubmission{}, err
	}
	var original kafka.DanmaEvent
	if err := json.Unmarshal([]byte(raw), &original); err != nil {
		return DanmaSubmission{}, fmt.Errorf("解析原弹幕事件: %w", err)
	}
	if original.MessageID != input.MessageID || original.OccurredAt.IsZero() ||
		original.RoomID <= 0 || original.UserID <= 0 || original.Content == "" {
		return DanmaSubmission{}, fmt.Errorf("原弹幕事件无效")
	}
	if original.RoomID != roomID || original.UserID != userID || original.Content != input.Content {
		return DanmaSubmission{}, ErrDanmaConflict
	}
	// 超时不删除绑定，原事件可能已到 Kafka；重试必须沿用它。
	if err := s.producer.Publish(ctx, original); err != nil {
		return DanmaSubmission{}, err
	}
	return DanmaSubmission{Event: original}, nil
}
