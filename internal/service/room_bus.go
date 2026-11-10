package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"example.com/live-stream/internal/ws"
	"github.com/redis/go-redis/v9"
)

const roomBusChannel = "live:room-events:v1"

// RoomBus 负责跨实例分发，以及固定演示实例的在线快照。
// 本地 Hub 不再把接收到的事件重新发布到 Redis。
type RoomBus struct {
	client      *redis.Client
	hub         *ws.Hub
	instanceID  string
	instanceIDs []string
	sub         *redis.PubSub
	events      <-chan *redis.Message
}

func NewRoomBus(client *redis.Client, hub *ws.Hub, instanceID string, instanceIDs []string) *RoomBus {
	return &RoomBus{client: client, hub: hub, instanceID: instanceID, instanceIDs: instanceIDs}
}

// Open 在启动消费者之前完成订阅确认，避免启动阶段先发布、后订阅。
func (b *RoomBus) Open(ctx context.Context) error {
	b.sub = b.client.Subscribe(ctx, roomBusChannel)
	if _, err := b.sub.Receive(ctx); err != nil {
		_ = b.sub.Close()
		return fmt.Errorf("订阅房间事件: %w", err)
	}
	b.events = b.sub.Channel()
	return nil
}

func (b *RoomBus) Close() {
	if b.sub != nil {
		_ = b.sub.Close()
	}
}

func (b *RoomBus) Publish(ctx context.Context, message ws.ServerMessage) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	n, err := b.client.Publish(ctx, roomBusChannel, data).Result()
	if err != nil {
		return fmt.Errorf("发布房间事件: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("房间事件没有订阅者")
	}
	return nil
}

func (b *RoomBus) Run(ctx context.Context) {
	defer b.Close()
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := b.client.Del(cleanup, b.onlineKey(b.instanceID)).Err(); err != nil {
			log.Printf("instance=%s remove online snapshot: %v", b.instanceID, err)
		}
	}()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	b.refreshOnline(ctx)
	log.Printf("instance=%s room bus subscribed", b.instanceID)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.refreshOnline(ctx)
		case event, ok := <-b.events:
			if !ok {
				log.Printf("instance=%s room bus channel closed", b.instanceID)
				return
			}
			var message ws.ServerMessage
			if err := json.Unmarshal([]byte(event.Payload), &message); err != nil || message.RoomID <= 0 {
				log.Printf("instance=%s invalid room bus event", b.instanceID)
				continue
			}
			switch message.Type {
			case ws.TypeAck, ws.TypeError:
				if message.UserID > 0 {
					b.hub.SendToUser(message.RoomID, message.UserID, message)
				}
			case ws.TypeDanma, ws.TypeRoomState:
				b.hub.Broadcast(message)
			default:
				continue
			}
			log.Printf("instance=%s bus received type=%s room=%d message_id=%s",
				b.instanceID, message.Type, message.RoomID, message.MessageID)
		}
	}
}

func (b *RoomBus) onlineKey(instanceID string) string {
	return "live:online:" + instanceID
}

// SnapshotOnlineCounts 汇总固定实例列表中尚未过期的快照；连接包含游客。
func (b *RoomBus) SnapshotOnlineCounts(ctx context.Context) (map[int64]int, error) {
	keys := make([]string, len(b.instanceIDs))
	for i, instanceID := range b.instanceIDs {
		keys[i] = b.onlineKey(instanceID)
	}
	values, err := b.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	total := make(map[int64]int)
	for _, value := range values {
		if value == nil {
			continue
		}
		payload, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("在线快照类型错误")
		}
		var counts map[int64]int
		if err := json.Unmarshal([]byte(payload), &counts); err != nil {
			return nil, fmt.Errorf("解析在线快照: %w", err)
		}
		for roomID, count := range counts {
			if roomID <= 0 || count < 0 {
				return nil, fmt.Errorf("在线快照计数无效")
			}
			total[roomID] += count
		}
	}
	return total, nil
}

func (b *RoomBus) refreshOnline(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, time.Second)
	defer cancel()
	local := b.hub.SnapshotOnlineCounts()
	data, err := json.Marshal(local)
	if err == nil {
		err = b.client.Set(ctx, b.onlineKey(b.instanceID), data, 12*time.Second).Err()
	}
	if err != nil {
		log.Printf("instance=%s write online snapshot: %v", b.instanceID, err)
		return
	}
	total, err := b.SnapshotOnlineCounts(ctx)
	if err != nil {
		log.Printf("instance=%s read online snapshots: %v", b.instanceID, err)
		return
	}
	for roomID := range local {
		count := total[roomID]
		b.hub.Broadcast(ws.ServerMessage{
			Type: ws.TypeRoomState, RoomID: roomID,
			OnlineCount: &count, SentAt: time.Now().UTC(),
		})
	}
}
