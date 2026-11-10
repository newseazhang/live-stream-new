package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"example.com/live-stream/internal/model"
	"example.com/live-stream/internal/repo"
)

var (
	ErrInvalidDanmaIdentity = errors.New("发送弹幕需要有效的房间和登录用户")
	ErrDanmaConflict        = errors.New("message_id 已用于其他弹幕，请为新消息生成新 ID")
)

// SaveDanma 的第二个返回值表示本次是否新插入了记录。
func SaveDanma(
	ctx context.Context,
	roomID, userID int64,
	input DanmaInput,
) (*model.Message, bool, error) {
	return SaveDanmaAt(ctx, roomID, userID, input, time.Now().UTC())
}

// SaveDanmaAt 保留 Kafka 事件的服务端时间，广播与落库使用同一时间。
func SaveDanmaAt(
	ctx context.Context,
	roomID, userID int64,
	input DanmaInput,
	sentAt time.Time,
) (*model.Message, bool, error) {
	if sentAt.IsZero() {
		return nil, false, errors.New("弹幕事件时间无效")
	}
	if roomID <= 0 || userID <= 0 {
		return nil, false, ErrInvalidDanmaIdentity
	}

	input, err := ValidateDanma(input)
	if err != nil {
		return nil, false, err
	}

	message := &model.Message{
		MessageID: input.MessageID,
		RoomID:    roomID,
		UserID:    userID,
		Content:   input.Content,
		SentAt:    sentAt.UTC(),
	}
	if err := repo.InsertMessage(ctx, message); err != nil {
		if !errors.Is(err, repo.ErrMessageConflict) {
			return nil, false, fmt.Errorf("保存弹幕: %w", err)
		}

		existing, err := repo.FindMessageByMessageID(ctx, input.MessageID)
		if err != nil {
			return nil, false, fmt.Errorf("查询重复弹幕: %w", err)
		}
		if existing == nil {
			return nil, false, errors.New("唯一约束冲突后未查到弹幕记录")
		}
		if existing.RoomID != roomID ||
			existing.UserID != userID ||
			existing.Content != input.Content {
			return nil, false, ErrDanmaConflict
		}
		return existing, false, nil
	}

	return message, true, nil
}
