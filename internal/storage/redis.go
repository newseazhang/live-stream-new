package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

func InitRedis(cfg RedisConfig) (*redis.Client, error) {

	// 创建可复用的客户端，内部管理连接池
	client := redis.NewClient(&redis.Options{
		Addr:                  cfg.Addr,
		Password:              cfg.Password,
		DB:                    cfg.DB,
		DialTimeout:           3 * time.Second,
		ReadTimeout:           3 * time.Second,
		WriteTimeout:          3 * time.Second,
		ContextTimeoutEnabled: true,
	})

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)

	// 关闭
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("连接 Redis 失败: %w", err)
	}

	return client, nil
}
