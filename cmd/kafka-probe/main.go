package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"

	"example.com/live-stream/internal/kafka"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt,
	)
	defer stop()

	consumer := kafka.NewConsumer(
		[]string{"127.0.0.1:19092"},
		"live-danma-probe",
		"live-consumer-probe-v1",
	)

	first := true
	log.Println("消费循环已启动，等待事件")

	err := consumer.Run(ctx, func(
		_ context.Context,
		event kafka.DanmaEvent,
	) error {
		if first {
			first = false
			return fmt.Errorf(
				"模拟首次处理失败 message_id=%s",
				event.MessageID,
			)
		}
		log.Printf(
			"消费成功 message_id=%s content=%s",
			event.MessageID, event.Content,
		)
		return nil
	})

	if err != nil && ctx.Err() == nil {
		log.Printf("消费者异常退出: %v", err)
		return
	}
	log.Println("消费者已关闭")
}