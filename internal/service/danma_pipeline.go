package service

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"example.com/live-stream/internal/cache"
	"example.com/live-stream/internal/kafka"
	"example.com/live-stream/internal/ws"
	"github.com/redis/go-redis/v9"
)

type DanmaPipeline struct {
	realtime *kafka.Consumer
	persist  *kafka.Consumer
	bus      *RoomBus
	recent   *cache.RecentMessages
	activity *cache.RoomActivity
}

func NewDanmaPipeline(client *redis.Client, bus *RoomBus, brokers []string, topic string) *DanmaPipeline {
	return &DanmaPipeline{
		realtime: kafka.NewConsumer(brokers, topic, "live-realtime-v1"),
		persist:  kafka.NewConsumer(brokers, topic, "live-persist-v1"),
		bus:      bus, recent: cache.NewRecentMessages(client),
		activity: cache.NewRoomActivity(client),
	}
}

func (p *DanmaPipeline) Run(ctx context.Context) {
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var workers sync.WaitGroup
	start := func(name string, consumer *kafka.Consumer, handle func(context.Context, kafka.DanmaEvent) error) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := consumer.Run(taskCtx, handle); err != nil && taskCtx.Err() == nil {
				log.Printf("kafka pipeline %s stopped: %v", name, err)
				cancel()
			}
		}()
	}
	start("realtime", p.realtime, p.broadcast)
	start("persist", p.persist, p.save)
	workers.Wait()
}

func (p *DanmaPipeline) broadcast(ctx context.Context, event kafka.DanmaEvent) error {
	input, err := ValidateDanma(DanmaInput{MessageID: event.MessageID, Content: event.Content})
	if err != nil {
		log.Printf("kafka realtime rejected message_id=%s: %v", event.MessageID, err)
		return nil
	}
	if err := p.bus.Publish(ctx, ws.ServerMessage{
		Type: ws.TypeDanma, MessageID: input.MessageID,
		RoomID: event.RoomID, UserID: event.UserID,
		Content: input.Content, SentAt: event.OccurredAt,
	}); err != nil {
		return err
	}
	log.Printf("kafka realtime room=%d message_id=%s", event.RoomID, event.MessageID)
	return nil
}

func (p *DanmaPipeline) save(ctx context.Context, event kafka.DanmaEvent) error {
	saved, created, err := SaveDanmaAt(ctx, event.RoomID, event.UserID, DanmaInput{
		MessageID: event.MessageID, Content: event.Content,
	}, event.OccurredAt)
	if errors.Is(err, ErrDanmaConflict) || errors.Is(err, ErrInvalidDanmaID) ||
		errors.Is(err, ErrInvalidDanmaContent) || errors.Is(err, ErrInvalidDanmaIdentity) {
		log.Printf("kafka persist rejected message_id=%s: %v", event.MessageID, err)
		retryable := false
		return p.bus.Publish(ctx, ws.ServerMessage{
			Type: ws.TypeError, RoomID: event.RoomID, UserID: event.UserID,
			MessageID: event.MessageID, Code: "danma_rejected",
			Content: err.Error(), Retryable: &retryable, SentAt: time.Now().UTC(),
		})
	}
	if err != nil {
		return err
	}
	log.Printf("kafka persist saved message_id=%s id=%d created=%t", saved.MessageID, saved.ID, created)
	if created {
		metricCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		err := p.activity.Record(metricCtx, saved.RoomID, cache.ActivityDanma, saved.SentAt)
		cancel()
		if err != nil {
			log.Printf("room activity room=%d: %v", saved.RoomID, err)
		}
	}
	cacheCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	if err := p.recent.Put(cacheCtx, cache.RecentMessage{
		ID: saved.ID, MessageID: saved.MessageID, RoomID: saved.RoomID,
		UserID: saved.UserID, Content: saved.Content, SentAt: saved.SentAt,
	}); err != nil {
		log.Printf("kafka recent cache room=%d: %v", saved.RoomID, err)
	}
	cancel()
	// ACK 发布失败返回错误，由原消费循环重试；SQLite UUID 唯一约束保持幂等。
	return p.bus.Publish(ctx, ws.ServerMessage{
		Type: ws.TypeAck, ID: saved.ID, MessageID: saved.MessageID,
		RoomID: saved.RoomID, UserID: saved.UserID,
		Content: saved.Content, SentAt: saved.SentAt,
	})
}
