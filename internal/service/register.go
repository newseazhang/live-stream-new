package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"example.com/live-stream/internal/model"
	"example.com/live-stream/internal/repo"
)

var (
	ErrInvalidRegistration = errors.New("注册参数无效")
	ErrUserAlreadyExists   = errors.New("用户名或邮箱已注册")
)

type RegisterInput struct {
	UserName string
	Email    string
	Password string
}

func RegisterUser(ctx context.Context, input RegisterInput) (*model.User, error) {
	username := strings.TrimSpace(input.UserName)
	email := strings.ToLower(strings.TrimSpace(input.Email))

	nameLength := utf8.RuneCountInString(username)
	if nameLength < 2 || nameLength > 32 {
		return nil, fmt.Errorf("%w: 用户名须为 2–32 个字符", ErrInvalidRegistration)
	}

	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 {
		return nil, fmt.Errorf("%w: 邮箱格式无效", ErrInvalidRegistration)
	}

	passwordLength := utf8.RuneCountInString(input.Password)
	if passwordLength < 15 || passwordLength > 128 {
		return nil, fmt.Errorf("%w: 密码须为 15–128 个字符", ErrInvalidRegistration)
	}

	existing, err := repo.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrUserAlreadyExists
	}

	hash, err := HashPassword(input.Password)
	if err != nil {
		return nil, fmt.Errorf("生成密码哈希失败: %w", err)
	}

	user := &model.User{
		UserName:     username,
		Email:        email,
		PasswordHash: hash,
	}
	// 重复注册的处理逻辑放在新建用户中 使用数据库的唯一约束（用户名和邮箱都是唯一的）拒绝重复注册
	created, err := repo.CreateUser(ctx, user)
	if errors.Is(err, repo.ErrUserConflict) {
		return nil, ErrUserAlreadyExists
	}
	return created, err
}
