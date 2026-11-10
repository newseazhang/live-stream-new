package repo

import (
	"context"
	"errors"
	"fmt"
	"example.com/live-stream/internal/model"

	"github.com/beego/beego/v2/client/orm"

	"modernc.org/sqlite"

	"modernc.org/sqlite/lib"
)

var ErrUserConflict = errors.New("用户名或邮箱已注册")

// CreateUser 将用户写入数据库，并返回带有自增主键的用户。 //拒绝重复注册
func CreateUser(ctx context.Context, user *model.User) (*model.User, error) {
	if user == nil {
		return nil, errors.New("创建用户时必须提供用户对象")
	}
	// NewOrm 每次拿到的是一个新 ORM 实例，不是协程安全的，适合单次操作。

	if _, err := orm.NewOrm().InsertWithCtx(ctx, user); err != nil {

		var sqliteErr *sqlite.Error
		if errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return nil, fmt.Errorf("%w: %w", ErrUserConflict, err)
		}
		return nil, fmt.Errorf("创建用户失败: %w", err)
	} // InsertWithCtx（）返回值为affectedRows int64, err error 第一个是受影响的行数，插入成功后 user中的自增id会回填到结构体中

	return user, nil
}

// FindUserByEmail 按邮箱查找用户。查无记录时返回 nil, nil。
func FindUserByEmail(ctx context.Context, email string) (*model.User, error) {
	user := &model.User{Email: email}

	err := orm.NewOrm().ReadWithCtx(ctx, user, "Email")
	if errors.Is(err, orm.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("按邮箱查询用户失败: %w", err)
	}

	return user, nil
}

func FindUserByID(ctx context.Context,userid int64) (*model.User ,error){

	user := &model.User{UserID: userid}


	err := orm.NewOrm().ReadWithCtx(ctx,user)
	
    if errors.Is(err, orm.ErrNoRows) {
        return nil, nil
    }
	if err != nil {
        return nil, fmt.Errorf("按 ID 查询用户失败: %w", err)
    }

	return user,nil
}





