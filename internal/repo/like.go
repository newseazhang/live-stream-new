package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"example.com/live-stream/internal/model"
	"github.com/beego/beego/v2/client/orm"
)

var ErrRoomCannotLike = errors.New("房间不存在或未开播")

// 统计总点赞数
func IncrementRoomLike(
	ctx context.Context,
	roomID int64,
) (int64, error) {
	var total int64

	err := orm.NewOrm().DoTxWithCtx( // SQL 要么全部成功提交，要么全部回滚
		ctx,
		func(txCtx context.Context, tx orm.TxOrmer) error {
			affected, err := tx.
				QueryTable(new(model.Room)).
				Filter("RoomID", roomID).
				Filter("Status", model.RoomStatusLive). // 开播的才可以统计点赞
				UpdateWithCtx(txCtx, orm.Params{
					"LikeCount": orm.ColValue(orm.ColAdd, 1), // `orm.ColAdd`：数据库层面自增，**原子操作，避免并发点赞覆盖问题**
					"UpdatedAt": time.Now().UTC(),
				})

			if err != nil {
				return fmt.Errorf("增加点赞数: %w", err)
			}
			if affected == 0 { // 没有匹配到行。两种场景：房间不存在 / 房间不在直
				return ErrRoomCannotLike
			}

			room := &model.Room{
				RoomID: roomID,
			}

			if err := tx.ReadWithCtx(txCtx, room); err != nil {
				return fmt.Errorf("读取点赞总数: %w", err)
			}

			total = room.LikeCount
			return nil
		},
	)

	if err != nil {
		return 0, fmt.Errorf("更新房间点赞: %w", err)
	}

	return total, nil
}
