package ws

import (
	"log"
	"sync"
)

const (
	fanoutShardCount = 4
	fanoutQueueSize  = 256
)

type clientShard struct {
	mu      sync.RWMutex
	clients map[*Client]uint64
}

type fanoutJob struct {
	shard       *clientShard
	message     ServerMessage
	maxClientID uint64
}

func clientShardIndex(clientID uint64) int {
	return int((clientID - 1) % fanoutShardCount)
}

// 每个 Hub 只启动四个 worker，所有房间共用，不随消息数或房间数创建 goroutine。
func (h *Hub) startFanout() {
	for index := range h.fanoutQueues {
		queue := make(chan fanoutJob, fanoutQueueSize)
		h.fanoutQueues[index] = queue
		h.fanoutWG.Add(1)
		go h.fanoutWorker(index, queue)
	}
	log.Printf("fanout started workers=%d queue_capacity=%d", fanoutShardCount, fanoutQueueSize)
}

func (h *Hub) fanoutWorker(index int, queue <-chan fanoutJob) {
	defer h.fanoutWG.Done()
	var jobs, sendAttempts uint64
	firstDanma := true
	defer func() {
		log.Printf("fanout shard=%d stopped jobs=%d send_attempts=%d pending=%d",
			index, jobs, sendAttempts, len(queue))
	}()
	for {
		// 退出信号优先，避免停止后持续消耗积压任务。
		select {
		case <-h.fanoutStop:
			return
		default:
		}
		select {
		case <-h.fanoutStop:
			return
		case job := <-queue:
			clients := snapshotShard(job.shard, job.maxClientID)
			if firstDanma && job.message.Type == TypeDanma && len(clients) > 0 {
				log.Printf("fanout shard=%d first_danma room=%d clients=%d message_id=%s",
					index, job.message.RoomID, len(clients), job.message.MessageID)
				firstDanma = false
			}
			for _, client := range clients {
				select {
				case <-h.fanoutStop:
					return
				default:
				}
				// Send 只投递到连接原有的有界发送队列；网络写仍由 WriteLoop 执行。
				client.Send(job.message)
				sendAttempts++
			}
			jobs++
		}
	}
}

func snapshotShard(shard *clientShard, maxClientID uint64) []*Client {
	shard.mu.RLock()
	clients := make([]*Client, 0, len(shard.clients))
	for client, clientID := range shard.clients {
		// 排除广播入队后才加入的连接，避免异步处理改变入房边界。
		if clientID <= maxClientID {
			clients = append(clients, client)
		}
	}
	shard.mu.RUnlock()
	return clients
}
