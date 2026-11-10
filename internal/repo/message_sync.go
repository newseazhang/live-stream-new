package repo

import (
	"context"
	"fmt"

	"example.com/live-stream/internal/model"
	"github.com/beego/beego/v2/client/orm"
)

func GetRoomMaxMessageID(ctx context.Context, roomID int64) (int64, error) {
	var rows []*model.Message
	_, err := orm.NewOrm().QueryTable(new(model.Message)).
		Filter("RoomID", roomID).OrderBy("-ID").Limit(1).
		AllWithCtx(ctx, &rows, "ID")
	if err != nil {
		return 0, fmt.Errorf("读取房间补拉上界: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].ID, nil
}

// ListRoomMessageIDsAfter 查询 afterID < id <= untilID，按 id 升序。
func ListRoomMessageIDsAfter(ctx context.Context, roomID, afterID, untilID int64, limit int) ([]int64, error) {
	var rows []*model.Message
	_, err := orm.NewOrm().QueryTable(new(model.Message)).
		Filter("RoomID", roomID).
		Filter("ID__gt", afterID).Filter("ID__lte", untilID).
		OrderBy("ID").Limit(limit).AllWithCtx(ctx, &rows, "ID")
	if err != nil {
		return nil, fmt.Errorf("读取房间补拉ID: %w", err)
	}
	ids := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	return ids, nil
}