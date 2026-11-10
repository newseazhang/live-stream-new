package cache

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	HotRankingKey = "live:heat:v1:ranking"
	HotRankingTTL = 30 * time.Second
)

type HotRanking struct {
	client *redis.Client
}

func NewHotRanking(client *redis.Client) *HotRanking {
	return &HotRanking{client: client}
}
//放在 Lua 里的好处：**整个替换过程原子化**，不会出现删一半、写一半，别的请求读到半残榜单。Redis 单线程，Lua 脚本执行期间不会插入其他命令。
var replaceHotRankingScript = redis.NewScript(`
redis.call('DEL', KEYS[1])
for i = 2, #ARGV, 2 do
    redis.call('ZADD', KEYS[1], ARGV[i + 1], ARGV[i])
end
redis.call('EXPIRE', KEYS[1], ARGV[1])
return (#ARGV - 1) / 2
`)

// Replace 发布本轮完整榜单；空集合会删除旧榜单。
func (r *HotRanking) Replace(
	ctx context.Context,
	scores map[int64]float64,
) error {
	args := make([]interface{}, 0, 1+2*len(scores))
	args = append(args, int64(HotRankingTTL/time.Second))

	for roomID, score := range scores {
		if roomID <= 0 || score < 0 ||
			math.IsNaN(score) || math.IsInf(score, 0) {
			return fmt.Errorf(
				"热度排名参数无效: room=%d score=%v",
				roomID, score,
			)
		}
		// Redis ZSet 的 member 是字符串类型。
		args = append(args, strconv.FormatInt(roomID, 10), score)
	}

	if err := replaceHotRankingScript.Run(
		ctx, r.client, []string{HotRankingKey}, args...,
	).Err(); err != nil {
		return fmt.Errorf("更新热度排名: %w", err)
	}
	return nil
}

type HotRoom struct {
	RoomID int64
	Score  float64
}

// Top 按分数降序读取；榜单缺失时返回空切片。
func (r *HotRanking) Top(
	ctx context.Context,
	limit int,
) ([]HotRoom, error) {
	values, err := r.client.ZRevRangeWithScores(
		ctx, HotRankingKey, 0, int64(limit-1),
	).Result()
	if err != nil {
		return nil, fmt.Errorf("读取热度排名: %w", err)
	}

	rooms := make([]HotRoom, 0, len(values))
	for _, value := range values {
		member, ok := value.Member.(string)
		if !ok {
			return nil, fmt.Errorf("热度榜成员类型无效")
		}

		roomID, err := strconv.ParseInt(member, 10, 64)
		if err != nil || roomID <= 0 || value.Score < 0 ||
			math.IsNaN(value.Score) || math.IsInf(value.Score, 0) {
			return nil, fmt.Errorf("热度榜成员或分数无效: %q", member)
		}

		rooms = append(rooms, HotRoom{
			RoomID: roomID,
			Score:  value.Score,
		})
	}
	return rooms, nil
}