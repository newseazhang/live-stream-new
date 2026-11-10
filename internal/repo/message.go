package repo

import (
	"context"
	"errors"
	"fmt"

	"example.com/live-stream/internal/model"

	"github.com/beego/beego/v2/client/orm"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var ErrMessageConflict = errors.New("message_id 已存在")

func InsertMessage(
	ctx context.Context,
	message *model.Message,
) error {
	id, err := orm.NewOrm().InsertWithCtx(ctx, message)
	if err == nil {
		message.ID = id
		return nil
	}

	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) &&
		sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		return ErrMessageConflict
	}

	return fmt.Errorf("插入弹幕失败: %w", err)
}

func FindMessageByMessageID(
	ctx context.Context,
	messageID string,
) (*model.Message, error) {
	message := &model.Message{
		MessageID: messageID,
	}

	err := orm.NewOrm().ReadWithCtx(
		ctx,
		message,
		"MessageID",
	)

	if errors.Is(err, orm.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询弹幕失败: %w", err)
	}

	return message, nil
}