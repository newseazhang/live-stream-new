package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

type Producer struct {
	writer *kafkago.Writer
}

func NewProducer(brokers []string, topic string) *Producer {
	return &Producer{writer: &kafkago.Writer{
		Addr:         kafkago.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafkago.Hash{},
		RequiredAcks: kafkago.RequireAll,
		Async:        false,
		BatchSize:    1,
		MaxAttempts:  3,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}}
}

func (p *Producer) Publish(
	ctx context.Context,
	event DanmaEvent,
) error {
	value, err := json.Marshal(event)

	if err != nil {
		return fmt.Errorf("序列化弹幕事件: %w", err)
	}

	err = p.writer.WriteMessages(ctx, kafkago.Message{
		Key:   []byte(strconv.FormatInt(event.RoomID, 10)),
		Value: value,
		Time:  event.OccurredAt,
	})
	if err != nil {
		return fmt.Errorf("发布弹幕事件: %w", err)
	}
	return nil
}

func (p *Producer) Close() error {
	return p.writer.Close()
}
