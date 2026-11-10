package repo

import (
	"context"
	"fmt"

	"example.com/live-stream/internal/model"
	"github.com/beego/beego/v2/client/orm"
)

// ListLiveRoomIDs 只读取开播房间的 ID，供热度任务使用。
func ListLiveRoomIDs(ctx context.Context) ([]int64, error) {
	var rooms []model.Room
	_, err := orm.NewOrm().QueryTable(new(model.Room)).
		Filter("Status", model.RoomStatusLive).
		Limit(-1). /// `Limit(-1)`：Beego ORM 特殊含义 —— **不限制条数，查出全部满足条件的数据**   优化方向：改成游标分页，分批获取，不要一次性全拉
		AllWithCtx(ctx, &rooms, "RoomID")
	if err != nil {
		return nil, fmt.Errorf("查询开播房间ID: %w", err)
	}

	ids := make([]int64, 0, len(rooms))
	for _, room := range rooms {
		ids = append(ids, room.RoomID)
	}
	return ids, nil
}