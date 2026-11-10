package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.com/live-stream/internal/model"
	"example.com/live-stream/internal/repo"

	"github.com/alexedwards/argon2id"
)

var ErrInvalidCredentials = errors.New("邮箱或密码错误")

func LoginUser(ctx context.Context, email, password string) (*model.User, error) {
	// 沿用注册时的邮箱处理规则。
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return nil, ErrInvalidCredentials
	}

	user, err := repo.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("查询登录用户失败: %w", err)
	}
	if user == nil {
		return nil, ErrInvalidCredentials
	}

	// 使用数据库里保存的哈希验证用户输入的密码。
	match, err := argon2id.ComparePasswordAndHash(password, user.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("校验密码哈希失败: %w", err)
	}
	if !match {
		return nil, ErrInvalidCredentials
	}

	return user, nil
}
