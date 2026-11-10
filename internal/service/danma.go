package service

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	ErrInvalidDanmaID      = errors.New("message_id 必须是有效的非零 UUID")
	ErrInvalidDanmaContent = errors.New("弹幕内容须为 1–200 个字符")
)

type DanmaInput struct {
	MessageID string
	Content   string
}

// 主要是校验弹幕信息合法性
func ValidateDanma(input DanmaInput) (DanmaInput, error) {
	// 校验 MessageID：必须是合法UUID
	id, err := uuid.Parse(input.MessageID)
	if err != nil || id == uuid.Nil {
		return DanmaInput{}, ErrInvalidDanmaID
	}

	// 处理弹幕内容：去掉首尾空白
	content := strings.TrimSpace(input.Content)

	// 校验：不能为空，且字符数量不能超过200个（按utf8字符，不是字节）
	if content == "" || utf8.RuneCountInString(content) > 200 {
		return DanmaInput{}, ErrInvalidDanmaContent
	}

	return DanmaInput{
		MessageID: id.String(),
		Content:   content,
	}, nil
}
