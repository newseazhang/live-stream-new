package model

import "time"

type User struct {
	UserID       int64     `orm:"column(user_id);pk;auto" json:"user_id"` //   `pk` = primary key 主键。
	UserName     string    `orm:"column(user_name);size(32);unique" json:"user_name"`
	Email        string    `orm:"size(254);unique" json:"email"`
	PasswordHash string    `orm:"size(255)" json:"-"`
	CreatedAt    time.Time `orm:"auto_now_add;type(datetime)" json:"created_at"`
	UpdatedAt    time.Time `orm:"auto_now;type(datetime)" json:"updated_at"`
}

func (u *User) TableName() string {
	return "users"
}

/*
| Go 字段 | 数据库列 | 用途 |
|---|---|---|
| `UserID` | `user_id` | 自增主键 |
| `UserName` | `user_name` | 用户名，要求唯一 |
| `Email` | `email` | 邮箱，要求唯一 |
| `PasswordHash` | `password_hash` | 保存密码哈希 |
| `CreatedAt` | `created_at` | 创建时间 |
| `UpdatedAt` | `updated_at` | 更新时间 |

*/
