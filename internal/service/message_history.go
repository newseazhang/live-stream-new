package service

import (
	"context"
	"errors"
	"log"
	"time"

	"example.com/live-stream/internal/cache"
	"example.com/live-stream/internal/model"
	"example.com/live-stream/internal/repo"
)

var ErrInvalidMessageQuery = errors.New("房间或历史分页参数无效")

type MessageHistoryPage struct {
	Messages     []*model.Message
	HasMore      bool
	NextBeforeID int64
	CacheHit     bool
}

func GetRoomMessages(
	ctx context.Context,
	recent *cache.RecentMessages,
	roomID, beforeID int64,
	limit int,
) (MessageHistoryPage, error) {
	if roomID <= 0 || beforeID < 0 ||
		limit < 1 || limit > 100 {
		return MessageHistoryPage{}, ErrInvalidMessageQuery
	}

	// 区分“房间不存在”和“房间没有弹幕”。
	if _, err := GetRoomDetail(ctx, roomID); err != nil {
		return MessageHistoryPage{}, err
	}

	// 多取一条，判断是否还有更早的消息。
	// ListRoomMessageIDsBefore根据房间id和最新的id数 找出id之前的若干id组成ids
	ids, err := repo.ListRoomMessageIDsBefore(ctx, roomID, beforeID, limit+1)
	if err != nil {
		return MessageHistoryPage{}, err
	}

	page := MessageHistoryPage{HasMore: len(ids) > limit}
	if page.HasMore {
		ids = ids[:limit]
		page.NextBeforeID = ids[len(ids)-1]
	}
	if len(ids) == 0 {
		page.Messages = make([]*model.Message, 0)
		return page, nil
	}

	// 数据库倒序取最近一页，返回时转为升序。
	messages, hit := readHistoryCache(ctx, recent, roomID, ids)
	page.CacheHit = hit
	if !hit {
		messages, err = repo.ListRoomMessagesByIDs(ctx, roomID, ids)
		if err != nil {
			return MessageHistoryPage{}, err
		}
	}
	if len(messages) != len(ids) {
		return MessageHistoryPage{}, errors.New("分页期间消息记录发生变化，请重试")
	}
	for i, message := range messages {
		if message.ID != ids[i] || message.RoomID != roomID {
			return MessageHistoryPage{}, errors.New("消息正文与分页 ID 不一致")
		}
	}
	// 只补最近页；浏览很旧的页不应反复挤占最近缓存。
	if !hit && beforeID == 0 && recent != nil {
		batch := make([]cache.RecentMessage, 0, len(messages))
		for _, message := range messages {
			batch = append(batch, cache.RecentMessage{
				ID: message.ID, MessageID: message.MessageID, RoomID: message.RoomID,
				UserID: message.UserID, Content: message.Content, SentAt: message.SentAt,
			})
		}
		cacheCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		cacheErr := recent.PutBatch(cacheCtx, batch)
		cancel()
		if cacheErr != nil {
			log.Printf("history refill cache room=%d: %v", roomID, cacheErr)
		}
	}

	// 数据库降序取最近一页，响应升序供页面展示。
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	page.Messages = messages
	return page, nil
}

// 根据ids和房间id读取历史消息缓存，并且做类型转换
func readHistoryCache(ctx context.Context, recent *cache.RecentMessages, roomID int64, ids []int64) ([]*model.Message, bool) {
	if recent == nil {
		return nil, false
	}
	cacheCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	records, err := recent.GetByIDs(cacheCtx, roomID, ids)
	cancel()
	if err != nil {
		if !errors.Is(err, cache.ErrRecentCacheMiss) {
			log.Printf("history read cache room=%d: %v", roomID, err)
		}
		return nil, false
	}
	messages := make([]*model.Message, 0, len(records))
	for _, record := range records {
		messages = append(messages, &model.Message{
			ID: record.ID, MessageID: record.MessageID, RoomID: record.RoomID,
			UserID: record.UserID, Content: record.Content, SentAt: record.SentAt,
		})
	}
	return messages, true
}
