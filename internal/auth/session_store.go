package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"errors"
	"github.com/redis/go-redis/v9"
)

const SessionTTL = 24 * time.Hour

var ErrInvalidSession = errors.New("会话无效或已过期")

func SessionKey(sessionID string) string {
	digest := sha256.Sum256([]byte(sessionID))
	return "session:" + hex.EncodeToString(digest[:])
}

func CreateSession(
	ctx context.Context,
	client *redis.Client,
	userID int64,
) (string, error) {
	sessionID, err := NewSessionID()
	if err != nil {
		return "", fmt.Errorf("生成会话 ID 失败: %w", err)
	}

	if err := client.Set(ctx, SessionKey(sessionID), userID, SessionTTL).Err(); err != nil {
		return "", fmt.Errorf("保存 Redis 会话失败: %w", err)
	}

	return sessionID, nil
}

// 刷新之后 根据sid返回登录后的页面 不需要重新登录
func GetSessionUserID(
	ctx context.Context,
	client *redis.Client,
	sid string,
) (int64, error) {
	if !isValidSessionID(sid) {
		return 0, ErrInvalidSession
	}

	userID, err := client.Get(ctx, SessionKey(sid)).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, ErrInvalidSession
	}
	if err != nil {
		return 0, fmt.Errorf("读取 Redis 会话失败: %w", err)
	}
	if userID <= 0 {
		return 0, errors.New("Redis 会话中的用户 ID 无效")
	}

	return userID, nil
}

func isValidSessionID(sid string) bool {
	if len(sid) != 64 {
		return false
	}

	_, err := hex.DecodeString(sid)
	return err == nil
}

func DeleteSession(ctx context.Context, client *redis.Client, sid string) error {
	if !isValidSessionID(sid) {
		return nil
	}

	if err := client.Del(ctx, SessionKey(sid)).Err(); err != nil {
		return fmt.Errorf("删除 Redis 会话失败: %w", err)
	}

	return nil
}
