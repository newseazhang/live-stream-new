package ws

import (
	"context"
	"sync"
	"time"
)

type Hub struct {
	mu           sync.Mutex
	rooms        map[int64]*roomGroup
	closing      bool
	active       sync.WaitGroup
	shutdownOnce sync.Once
	stopped      chan struct{}
	fanoutQueues [fanoutShardCount]chan fanoutJob
	fanoutStop   chan struct{}
	fanoutWG     sync.WaitGroup
}

type roomGroup struct {
	// clients / shardCounts 由 Hub.mu 保护，供注册、计数和私有发送使用。
	clients      map[*Client]uint64
	nextClientID uint64
	shardCounts  [fanoutShardCount]int
	shards       [fanoutShardCount]clientShard
	broadcastMu  sync.Mutex
}

func NewHub() *Hub {
	h := &Hub{
		rooms:      make(map[int64]*roomGroup),
		stopped:    make(chan struct{}),
		fanoutStop: make(chan struct{}),
	}
	h.startFanout()
	return h
}

func newRoomGroup() *roomGroup {
	room := &roomGroup{clients: make(map[*Client]uint64)}
	for i := range room.shards {
		room.shards[i].clients = make(map[*Client]uint64)
	}
	return room
}

// SnapshotOnlineCounts 返回独立快照；同账号多页面和游客都按连接计数。
func (h *Hub) SnapshotOnlineCounts() map[int64]int {
	h.mu.Lock()
	defer h.mu.Unlock()
	counts := make(map[int64]int, len(h.rooms))
	if h.closing {
		return counts
	}
	for roomID, room := range h.rooms {
		counts[roomID] = len(room.clients)
	}
	return counts
}

func (h *Hub) Add(client *Client) bool {
	h.mu.Lock()
	if h.closing {
		h.mu.Unlock()
		client.Close()
		return false
	}
	room := h.rooms[client.RoomID]
	if room == nil {
		room = newRoomGroup()
		h.rooms[client.RoomID] = room
	}
	if _, exists := room.clients[client]; exists {
		h.mu.Unlock()
		return true
	}
	// 使用连接序号分片，同账号和游客的多个连接也能分散到不同 worker。
	room.nextClientID++
	clientID := room.nextClientID
	index := clientShardIndex(clientID)
	shard := &room.shards[index]
	shard.mu.Lock()
	room.clients[client] = clientID
	room.shardCounts[index]++
	shard.clients[client] = clientID
	shard.mu.Unlock()
	// 与 closing 的检查共用同一把锁；停止后不能再增加等待计数。
	h.active.Add(1)
	h.mu.Unlock()

	// 跨实例在线数由 RoomBus 定期汇总并发送，本地成员变更不发布局部总数。
	return true
}

func (h *Hub) Remove(client *Client) {
	removed := false
	h.mu.Lock()
	if room := h.rooms[client.RoomID]; room != nil {
		if clientID, exists := room.clients[client]; exists {
			index := clientShardIndex(clientID)
			shard := &room.shards[index]
			shard.mu.Lock()
			delete(room.clients, client)
			delete(shard.clients, client)
			room.shardCounts[index]--
			shard.mu.Unlock()
			removed = true
		}
		if len(room.clients) == 0 {
			delete(h.rooms, client.RoomID)
		}
	}
	h.mu.Unlock()
	client.Close()

	if removed {
		h.active.Done()
	}
}

// Broadcast 按房间串行入队，由固定 worker 异步执行各连接分片的投递。
// 返回表示完成入队，不表示所有 WebSocket 已写出。
func (h *Hub) Broadcast(message ServerMessage) {
	for {
		h.mu.Lock()
		if h.closing {
			h.mu.Unlock()
			return
		}
		room := h.rooms[message.RoomID]
		h.mu.Unlock()
		if room == nil {
			return
		}

		room.broadcastMu.Lock()
		h.mu.Lock()
		if h.closing {
			h.mu.Unlock()
			room.broadcastMu.Unlock()
			return
		}
		// 等待广播锁期间，空房可能已删除并重建；重新取当前分组。
		if h.rooms[message.RoomID] != room {
			h.mu.Unlock()
			room.broadcastMu.Unlock()
			continue
		}
		maxClientID := room.nextClientID
		counts := room.shardCounts
		h.mu.Unlock()

		if message.Type == TypeRoomState {
			// 保留 RoomBus 传入的跨实例计数；点赞状态可以不携带在线数。
			message.SentAt = time.Now().UTC()
		}
		for index, count := range counts {
			if count == 0 {
				continue
			}
			job := fanoutJob{
				shard: &room.shards[index], message: message,
				maxClientID: maxClientID,
			}
			select {
			case h.fanoutQueues[index] <- job:
			case <-h.fanoutStop:
				room.broadcastMu.Unlock()
				return
			}
		}
		room.broadcastMu.Unlock()
		return
	}
}

// SendToUser 给本房间该用户的连接发送私有消息，不执行房间广播。
func (h *Hub) SendToUser(roomID, userID int64, message ServerMessage) {
	h.mu.Lock()
	if h.closing {
		h.mu.Unlock()
		return
	}
	clients := make([]*Client, 0)
	if room := h.rooms[roomID]; room != nil {
		for client := range room.clients {
			if client.UserID == userID {
				clients = append(clients, client)
			}
		}
	}
	h.mu.Unlock()
	for _, client := range clients {
		client.Send(message)
	}
}

// Shutdown 停止接纳新连接，关闭当前连接，等待它们完成清理。
// ctx 仅限制本次等待，超时后 Hub 仍保持关闭状态。
func (h *Hub) Shutdown(ctx context.Context) error {
	h.shutdownOnce.Do(func() {
		h.mu.Lock()
		h.closing = true
		// 队列不关闭；唤醒等待入队的 Broadcast，避免并发发送到已关闭 channel。
		close(h.fanoutStop)
		clients := make([]*Client, 0)
		for _, room := range h.rooms {
			for client := range room.clients {
				clients = append(clients, client)
			}
		}
		h.mu.Unlock()

		go func() {
			for _, client := range clients {
				client.Close()
			}
			h.fanoutWG.Wait()
			h.active.Wait()
			close(h.stopped)
		}()
	})

	select {
	case <-h.stopped:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
