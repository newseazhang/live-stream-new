package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const DanmaSubmissionTTL = 24 * time.Hour

type DanmaSubmissions struct {
	client *redis.Client
}
/***弹幕防重提交（幂等）组件**：用 Redis + Lua 脚本实现，根据 `messageID（UUID）`保证：**同一个 messageID 第一次进来才写入 payload，之后重复调用直接返回第一次存入的值，不会覆盖，实现幂等，防止同一条弹幕重复入库、重复广播**。
典型场景：Kafka 重复消费、前端重试提交弹幕，避免一条弹幕发多次。*/
func NewDanmaSubmissions(client *redis.Client) *DanmaSubmissions {
	return &DanmaSubmissions{client: client}
}

// 为什么使用lua脚本 而不是go的先get再set ：Lua 脚本在 Redis 里是**原子执行**！
/*如果在 Go 代码里分开写 `Get` + 判断 + `Set`：中间会有网络 RTT，并发重试的时候，两个请求同时 GET 发现 key 不存在，然后都执行 Set，就会击穿防重，出现重复。Lua 保证这一串动作不会被别的命令打断。*/
var bindDanmaScript = redis.NewScript(`
local value = redis.call('GET', KEYS[1])
if value then
    return value
end
redis.call('SET', KEYS[1], ARGV[1], 'EX', ARGV[2])
return ARGV[1]
`)

// Bind 保留同一 UUID 首次提交的事件；后续调用返回原值。；`messageID`：全局唯一 id（UUID，幂等键），前端 / 生产者生成，同一条弹幕永远带同一个 messageID
func (s *DanmaSubmissions) Bind(ctx context.Context, messageID, payload string) (string, error) {
	key := "live:danma:v1:submission:" + messageID
	value, err := bindDanmaScript.Run(ctx, s.client, []string{key},
		payload, int64(DanmaSubmissionTTL/time.Second),
	).Text()
	if err != nil {
		return "", fmt.Errorf("绑定弹幕事件: %w", err)
	}
	return value, nil
}