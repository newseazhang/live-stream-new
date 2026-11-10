package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	RecentMessageLimit = 200       // 单房间最多缓存消息条数上限
	RecentMessageTTL   = time.Hour // 缓存过期时间
	maxExactID         = 1<<53 - 1 //JS 安全整数上限；因为JSON序列化前端要用，防止ID溢出失真

)

var (
	ErrRecentCacheMiss    = errors.New("最近消息缓存未命中或不完整")
	ErrInvalidRecentInput = errors.New("最近消息缓存参数无效")
)

// RecentMessage 是缓存记录，不依赖 ORM 或 WebSocket 连接。
type RecentMessage struct {
	ID        int64     `json:"id"`
	MessageID string    `json:"message_id"` // 业务唯一消息标识，和ID不是一回事
	RoomID    int64     `json:"room_id"`
	UserID    int64     `json:"user_id"`
	Content   string    `json:"content"`
	SentAt    time.Time `json:"sent_at"`
}

type RecentMessages struct {
	client *redis.Client
}

// client 在初始化时需启用 ContextTimeoutEnabled。
func NewRecentMessages(client *redis.Client) *RecentMessages {
	return &RecentMessages{client: client}
}

func recentKeys(roomID int64) []string {
	prefix := fmt.Sprintf("live:recent:v1:{room:%d}", roomID) // **ZSet（有序集合）**，存消息 ID，score = 消息 ID（用自增 ID 做排序，不是时间戳）
	return []string{prefix + ":ids", prefix + ":items"}       // `live:recent:v1:{room:%d}:items` → **Hash**，key = 消息 ID 字符串
}

//HashTag 是什么？为什么要使用 {room:%d}? HashTag = 保证同一个房间的两套数据处于同一个 Redis Cluster 槽位。如果没有hashtag zset和hash可能无法落到同一个哈希槽中，就会导致无法使用联合的原子操作，可能增加网络通信开销和业务实现复杂度。
/*
	ZSet 已经保存了消息 ID 和时间，为什么还需要 Hash?
	ZSet 擅长排序，但不适合直接管理消息的详细业务字段。
	这样设计有三个明显的好处。

第一，排序与内容存储解耦。消息内容可以发生变化，而排序索引不需要改变。
第二，减少 ZSet 的成员大小。ZSet 只保存简短的 ID，不需要把整个 JSON 参与排序结构的存储。
第三，容易清理历史消息。当只需要保留最近 100 条时，可以找到并删除最旧的消息 ID，同时删除 Hash 中对应的内容。
*/

// 入参校验函数，防止脏数据写入 Redis。
func validRecent(message RecentMessage) bool {
	return message.ID > 0 && message.ID <= maxExactID &&
		message.RoomID > 0 && message.UserID > 0 &&
		message.MessageID != "" && message.Content != "" && !message.SentAt.IsZero()
}

/*
发消息 → Put()

	校验 → json.Marshal → Lua脚本
	  类型校验 → 不一致就删 → ZADD+HSET → 超量删旧 → PEXPIRE

拉最新消息 → Latest(room, 50)

	Lua校验数量一致 → ZRANGE取最后50个id → HGET拼id+body数组返回
	Go层循环反序列化+强校验ID顺序、合法性 → 返回 []RecentMessage（升序：早→晚）
*/
var putRecentScript = redis.NewScript(`
local indexType = redis.call('TYPE', KEYS[1]).ok
local bodyType = redis.call('TYPE', KEYS[2]).ok
if (indexType ~= 'none' and indexType ~= 'zset') or
   (bodyType ~= 'none' and bodyType ~= 'hash') then
    return redis.error_reply('recent cache has wrong key type')
end
if redis.call('ZCARD', KEYS[1]) ~= redis.call('HLEN', KEYS[2]) then
    redis.call('DEL', KEYS[1], KEYS[2])
end
for i = 3, #ARGV, 2 do
    redis.call('ZADD', KEYS[1], ARGV[i], ARGV[i])
    redis.call('HSET', KEYS[2], ARGV[i], ARGV[i + 1])
end
local excess = redis.call('ZCARD', KEYS[1]) - tonumber(ARGV[1])
if excess > 0 then
    local expired = redis.call('ZRANGE', KEYS[1], 0, excess - 1)
    for _, id in ipairs(expired) do
        redis.call('HDEL', KEYS[2], id)
    end
    redis.call('ZREMRANGEBYRANK', KEYS[1], 0, excess - 1)
end
redis.call('PEXPIRE', KEYS[1], ARGV[2])
redis.call('PEXPIRE', KEYS[2], ARGV[2])
return 1
`)

func (c *RecentMessages) Put(ctx context.Context, message RecentMessage) error {
	return c.PutBatch(ctx, []RecentMessage{message})
}

// PutBatch 合并同房间的一批记录，不覆盖整份缓存，保留并发写入的较新消息。
func (c *RecentMessages) PutBatch(ctx context.Context, messages []RecentMessage) error {
	if len(messages) == 0 || len(messages) > RecentMessageLimit {
		return ErrInvalidRecentInput
	}
	roomID := messages[0].RoomID
	args := []interface{}{RecentMessageLimit, RecentMessageTTL.Milliseconds()}
	for _, message := range messages {
		if !validRecent(message) || message.RoomID != roomID {
			return ErrInvalidRecentInput
		}
		body, err := json.Marshal(message)
		if err != nil {
			return fmt.Errorf("编码最近消息: %w", err)
		}
		args = append(args, strconv.FormatInt(message.ID, 10), body)
	}
	err := putRecentScript.Run(ctx, c.client, recentKeys(roomID), args...).Err()
	if err != nil {
		return fmt.Errorf("写入最近消息缓存: %w", err)
	}
	return nil
}

var latestRecentScript = redis.NewScript(`
if redis.call('ZCARD', KEYS[1]) ~= redis.call('HLEN', KEYS[2]) then
    return {}
end
local ids = redis.call('ZRANGE', KEYS[1], -tonumber(ARGV[1]), -1)
local result = {}
for _, id in ipairs(ids) do
    local body = redis.call('HGET', KEYS[2], id)
    if not body then return {} end
    table.insert(result, id)
    table.insert(result, body)
end
return result
`)

// Latest 返回缓存中已有记录的升序快照，不承诺覆盖数据库最新消息或完整区间。
func (c *RecentMessages) Latest(ctx context.Context, roomID int64, limit int) ([]RecentMessage, error) {
	if roomID <= 0 || limit < 1 || limit > RecentMessageLimit {
		return nil, ErrInvalidRecentInput
	}
	rows, err := latestRecentScript.Run(ctx, c.client, recentKeys(roomID), limit).StringSlice()
	if err != nil {
		return nil, fmt.Errorf("读取最近消息缓存: %w", err)
	}
	if len(rows) == 0 || len(rows)%2 != 0 {
		return nil, ErrRecentCacheMiss
	}
	messages := make([]RecentMessage, 0, len(rows)/2)
	var previousID int64
	for i := 0; i < len(rows); i += 2 {
		var message RecentMessage
		if err := json.Unmarshal([]byte(rows[i+1]), &message); err != nil {
			return nil, fmt.Errorf("%w: JSON 错误: %v", ErrRecentCacheMiss, err)
		}
		if !validRecent(message) || message.RoomID != roomID || message.ID <= previousID ||
			strconv.FormatInt(message.ID, 10) != rows[i] {
			return nil, ErrRecentCacheMiss
		}
		previousID = message.ID
		messages = append(messages, message)
	}
	return messages, nil
}

// All-or-Nothing 策略  可优化点
func (c *RecentMessages) GetByIDs(ctx context.Context, roomID int64, ids []int64) ([]RecentMessage, error) {
	if roomID <= 0 || len(ids) == 0 || len(ids) > RecentMessageLimit {
		return nil, ErrInvalidRecentInput
	}
	fields := make([]string, len(ids))
	for i, id := range ids {
		if id <= 0 || id > maxExactID {
			return nil, ErrInvalidRecentInput
		}
		fields[i] = strconv.FormatInt(id, 10)
	}
	values, err := c.client.HMGet(ctx, recentKeys(roomID)[1], fields...).Result()
	if err != nil {
		return nil, fmt.Errorf("按 ID 读取最近消息缓存: %w", err)
	}
	if len(values) != len(ids) {
		return nil, ErrRecentCacheMiss
	}
	messages := make([]RecentMessage, 0, len(ids))
	for i, value := range values {
		body, ok := value.(string)
		if !ok {
			return nil, ErrRecentCacheMiss
		}
		var message RecentMessage
		if err := json.Unmarshal([]byte(body), &message); err != nil {
			return nil, fmt.Errorf("%w: JSON 错误: %v", ErrRecentCacheMiss, err)
		}
		// 缓存不存在 / 类型不对 /json 解析失败 / 数据校验不通过
		if !validRecent(message) || message.RoomID != roomID || message.ID != ids[i] {
			return nil, ErrRecentCacheMiss
		}
		messages = append(messages, message)
	}
	return messages, nil
}
