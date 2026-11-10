package cache

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	ActivityWindowMinutes = 5
	ActivityTTL           = 6 * time.Minute
)

type ActivityKind string

const (
	ActivityDanma ActivityKind = "danmas"
	ActivityLike  ActivityKind = "likes"
)

var ErrInvalidActivity = errors.New("房间热度指标参数或计数无效")

type ActivityCounts struct {
	Danmas int64
	Likes  int64
}

type RoomActivity struct {
	client *redis.Client
}

func NewRoomActivity(client *redis.Client) *RoomActivity {
	return &RoomActivity{client: client}
}

func activityMinuteKey(roomID, minute int64) string {
	return fmt.Sprintf("live:heat:v1:{room:%d}:minute:%d", roomID, minute)
}

var recordActivityScript = redis.NewScript(`
local keyType = redis.call('TYPE', KEYS[1]).ok
if keyType ~= 'none' and keyType ~= 'hash' then
    return redis.error_reply('activity bucket has wrong key type')
end
local count = redis.call('HINCRBY', KEYS[1], ARGV[1], 1)
redis.call('EXPIRE', KEYS[1], ARGV[2])
return count
`)

// Record 记录已经成功的业务事件；指标为尽力采集，不能作为结算数据。
func (a *RoomActivity) Record(ctx context.Context, roomID int64, kind ActivityKind, at time.Time) error {
	if roomID <= 0 || at.IsZero() || (kind != ActivityDanma && kind != ActivityLike) {
		return ErrInvalidActivity
	}
	key := activityMinuteKey(roomID, at.Unix()/60)
	if err := recordActivityScript.Run(ctx, a.client, []string{key},
		string(kind), int64(ActivityTTL/time.Second),
	).Err(); err != nil {
		return fmt.Errorf("记录房间近期互动: %w", err)
	}
	return nil
}

// ReadRecent 汇总当前分钟及之前四个分钟桶，不是精确滚动的 300 秒窗口。 批量方案：使用 Redis Pipeline 打包多条 HMGET，减少网络往返 RTT
func (a *RoomActivity) ReadRecent(ctx context.Context, roomID int64, now time.Time) (ActivityCounts, error) {
	if roomID <= 0 || now.IsZero() {
		return ActivityCounts{}, ErrInvalidActivity
	}
	commands := make([]*redis.SliceCmd, 0, ActivityWindowMinutes)
	_, err := a.client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for offset := 0; offset < ActivityWindowMinutes; offset++ {
			key := activityMinuteKey(roomID, now.Unix()/60-int64(offset))
			commands = append(commands, pipe.HMGet(ctx, key, "danmas", "likes"))
		}
		return nil
	})
	if err != nil {
		return ActivityCounts{}, fmt.Errorf("读取房间近期互动: %w", err)
	}
	var counts ActivityCounts
	for _, command := range commands {
		values, err := command.Result()
		if err != nil {
			return ActivityCounts{}, fmt.Errorf("读取分钟桶: %w", err)
		}
		if len(values) != 2 {
			return ActivityCounts{}, ErrInvalidActivity
		}
		if err := addActivityCount(&counts.Danmas, values[0]); err != nil {
			return ActivityCounts{}, err
		}
		if err := addActivityCount(&counts.Likes, values[1]); err != nil {
			return ActivityCounts{}, err
		}
	}
	return counts, nil
}

func addActivityCount(total *int64, value interface{}) error {
	if value == nil {
		return nil
	}
	raw, ok := value.(string)
	if !ok {
		return ErrInvalidActivity
	}
	number, err := strconv.ParseInt(raw, 10, 64)
	const maxCount int64 = 1<<63 - 1
	if err != nil || number < 0 || number > maxCount-*total {
		return ErrInvalidActivity
	}
	*total += number
	return nil
}