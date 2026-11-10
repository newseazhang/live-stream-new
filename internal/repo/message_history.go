package repo

import (
	"context"
	"fmt"

	"example.com/live-stream/internal/model"
	"github.com/beego/beego/v2/client/orm"
)

// beforeID 为 0 时查询最新消息。
// 返回结果按数据库 ID 降序排列。
func ListRoomMessagesBefore(
	ctx context.Context,
	roomID, beforeID int64,
	limit int,
) ([]*model.Message, error) {
	query := orm.NewOrm().
		QueryTable(new(model.Message)).
		Filter("RoomID", roomID)

	if beforeID > 0 {
		query = query.Filter("ID__lt", beforeID)
	}

	messages := make([]*model.Message, 0)

	_, err := query.
		OrderBy("-ID"). // -ID 代表按ID倒序（新消息在前）
		Limit(limit).
		AllWithCtx(ctx, &messages)

	if err != nil {
		return nil, fmt.Errorf("查询房间弹幕历史: %w", err)
	}

	return messages, nil
}

// ListRoomMessageIDsBefore 仅查询 id，使用现有 (room_id, id) 索引确定准确页范围。
func ListRoomMessageIDsBefore(ctx context.Context, roomID, beforeID int64, limit int) ([]int64, error) {
	query := orm.NewOrm().QueryTable(new(model.Message)).Filter("RoomID", roomID)
	if beforeID > 0 {
		query = query.Filter("ID__lt", beforeID)
	}
	rows := make([]*model.Message, 0)
	_, err := query.OrderBy("-ID").Limit(limit).AllWithCtx(ctx, &rows, "ID")
	if err != nil {
		return nil, fmt.Errorf("查询房间消息 ID: %w", err)
	}
	ids := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	return ids, nil
}

// **根据房间 ID + 一批消息 ID 数组，去数据库批量查出完整消息记录**。
func ListRoomMessagesByIDs(ctx context.Context, roomID int64, ids []int64) ([]*model.Message, error) {
	messages := make([]*model.Message, 0)
	if len(ids) == 0 {
		return messages, nil
	}
	_, err := orm.NewOrm().QueryTable(new(model.Message)).
		Filter("RoomID", roomID).Filter("ID__in", ids).
		OrderBy("-ID").AllWithCtx(ctx, &messages)
	if err != nil {
		return nil, fmt.Errorf("按 ID 查询房间消息正文: %w", err)
	}
	return messages, nil
}
