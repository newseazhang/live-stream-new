package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

type Consumer struct {
	reader *kafkago.Reader
	group  string
}

// 构造消费对象
func NewConsumer(brokers []string, topic, group string) *Consumer {
	return &Consumer{
		group: group, // 自己使用，方便定位日志
		reader: kafkago.NewReader(kafkago.ReaderConfig{
			Brokers: brokers, Topic: topic, GroupID: group, //// 交给 Kafka，标识消费组
			MinBytes: 1, MaxBytes: 1 << 20,
			MaxWait: time.Second, CommitInterval: 0,
			StartOffset: kafkago.FirstOffset,
		}),
	}
}

// Run 先处理成功，再提交；失败时停留在当前消息，不跳到下一条。
func (c *Consumer) Run(ctx context.Context, handle func(context.Context, DanmaEvent) error) error {
	defer func() {
		if err := c.reader.Close(); err != nil {
			log.Printf("kafka group=%s close: %v", c.group, err)
		}
	}()
	for {
		message, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("kafka group=%s fetch: %v", c.group, err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
				continue
			}
		}
		// JSON 反序列化，把消息字节转回弹幕事件结构体。 一旦解析失败，直接 return err，消费循环退出！ 可优化点
		var event DanmaEvent
		if err := json.Unmarshal(message.Value, &event); err != nil {
			return fmt.Errorf("group=%s partition=%d offset=%d JSON无效: %w",
				c.group, message.Partition, message.Offset, err)
		}
		if event.MessageID == "" || event.RoomID <= 0 || event.UserID <= 0 ||
			event.Content == "" || event.OccurredAt.IsZero() {
			return fmt.Errorf("group=%s partition=%d offset=%d 事件字段无效",
				c.group, message.Partition, message.Offset)
		}
		if err := c.retry(ctx, "handle", func(attempt context.Context) error {
			return handle(attempt, event)
		}); err != nil {
			return err
		}
		if err := c.retry(ctx, "commit", func(attempt context.Context) error {
			return c.reader.CommitMessages(attempt, message)
		}); err != nil {
			return err
		}
	}
}

func (c *Consumer) retry(ctx context.Context, step string, work func(context.Context) error) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		attempt, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := work(attempt)
		cancel()
		if err == nil {
			return nil
		}
		log.Printf("kafka group=%s %s retry: %v", c.group, step, err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
