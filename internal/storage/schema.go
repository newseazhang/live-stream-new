package storage

import (

	"errors"
	"fmt"

	"example.com/live-stream/internal/model"

	"github.com/beego/beego/v2/client/orm"
)


var ErrMessageConflict = errors.New("message_id 已存在")


func InitSchema() error {
	// 注册模型，每个模型只注册一次。
	orm.RegisterModel(
		new(model.User),
		new(model.Room),
		new(model.Message),
	)

	if err := orm.RunSyncdb("default", false, true); err != nil {
		return fmt.Errorf("初始化数据库表结构失败: %w", err)
	}

	return nil
}


