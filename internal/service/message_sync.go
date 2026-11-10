package service

import (
	"context"
	"errors"

	"example.com/live-stream/internal/cache"
	"example.com/live-stream/internal/model"
	"example.com/live-stream/internal/repo"
)

var ErrInvalidMessageSyncQuery = errors.New("补拉游标、上界或分页条数无效")

type MessageSyncPage struct {
	Messages    []*model.Message
	HasMore     bool
	NextAfterID int64
	UntilID     int64
	CacheHit    bool
}

func GetRoomMessageChanges(
	ctx context.Context,
	recent *cache.RecentMessages,
	roomID, afterID int64,
	untilID *int64,
	limit int,
) (MessageSyncPage, error) {
	if roomID <= 0 || afterID < 0 || limit < 1 || limit > 100 ||
		(untilID != nil && *untilID < afterID) {
		return MessageSyncPage{}, ErrInvalidMessageSyncQuery
	}
	if _, err := GetRoomDetail(ctx, roomID); err != nil {
		return MessageSyncPage{}, err
	}
	upper, err := repo.GetRoomMaxMessageID(ctx, roomID)
	if err != nil {
		return MessageSyncPage{}, err
	}
	if upper < afterID {
		upper = afterID
	}
	if untilID != nil {
		if *untilID > upper {
			return MessageSyncPage{}, ErrInvalidMessageSyncQuery
		}
		upper = *untilID
	}
	page := MessageSyncPage{
		Messages: make([]*model.Message, 0),
		UntilID: upper, NextAfterID: afterID,
	}
	if upper == afterID {
		return page, nil
	}
	ids, err := repo.ListRoomMessageIDsAfter(ctx, roomID, afterID, upper, limit+1)
	if err != nil {
		return MessageSyncPage{}, err
	}
	page.HasMore = len(ids) > limit
	if page.HasMore {
		ids = ids[:limit]
	}
	if len(ids) == 0 {
		page.NextAfterID = upper
		return page, nil
	}
	// 复用历史缓存读取；GetByIDs 按传入 ID 的顺序返回。
	messages, hit := readHistoryCache(ctx, recent, roomID, ids)
	page.CacheHit = hit
	if !hit {
		messages, err = repo.ListRoomMessagesByIDs(ctx, roomID, ids)
		if err != nil {
			return MessageSyncPage{}, err
		}
		// 原 Repository 正文查询是降序，此处转为补拉需要的升序。
		for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
			messages[left], messages[right] = messages[right], messages[left]
		}
	}
	if len(messages) != len(ids) {
		return MessageSyncPage{}, errors.New("补拉记录数量发生变化，请重试")
	}
	for i, message := range messages {
		if message.RoomID != roomID || message.ID != ids[i] {
			return MessageSyncPage{}, errors.New("补拉正文与ID不一致")
		}
	}
	page.Messages = messages
	page.NextAfterID = upper
	if page.HasMore {
		page.NextAfterID = ids[len(ids)-1]
	}
	return page, nil
}