package main

import (
	"context"
	"log"
	"os"
	"time"

	"example.com/live-stream/internal/kafka"
	"example.com/live-stream/internal/lifecycle"
	"example.com/live-stream/internal/service"
	"example.com/live-stream/internal/storage"
	"example.com/live-stream/internal/ws"
	"example.com/live-stream/routers"

	"github.com/beego/beego/v2/server/web"
	"github.com/beego/beego/v2/server/web/grace"
)

func main() {
	// 1. 选择实例和监听端口。
	instanceID := os.Getenv("LIVE_INSTANCE_ID")
	if instanceID == "" {
		instanceID = "gw-a"
	}
	if instanceID != "gw-a" && instanceID != "gw-b" {
		log.Fatal("LIVE_INSTANCE_ID 只支持 gw-a 或 gw-b")
	}
	log.SetPrefix("[" + instanceID + "] ")

	port := 19000
	if instanceID == "gw-b" {
		port = 19001
	}
	web.BConfig.Listen.HTTPAddr = "127.0.0.1"
	web.BConfig.Listen.HTTPPort = port

	// 2. 初始化 SQLite。两个实例共用同一个数据库文件。
	dbPath := web.AppConfig.DefaultString(
		"sqlite_path",
		"data/live-stream.db",
	)
	if err := storage.InitSQLite(dbPath); err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	log.Printf("SQLite 初始化成功，数据库路径：%s", dbPath)

	if err := storage.InitSchema(); err != nil {
		log.Fatalf("初始化表结构失败: %v", err)
	}
	log.Println("数据库表结构初始化成功")

	// 3. 初始化 Redis。
	redisCfg := storage.RedisConfig{
		Addr: web.AppConfig.DefaultString(
			"redis_addr",
			"127.0.0.1:16379",
		),
		Password: web.AppConfig.DefaultString(
			"redis_password",
			"",
		),
		DB: web.AppConfig.DefaultInt("redis_db", 0),
	}
	redisClient, err := storage.InitRedis(redisCfg)
	if err != nil {
		log.Fatalf("初始化 Redis 失败: %v", err)
	}
	defer redisClient.Close()
	log.Printf("Redis 初始化成功，地址：%s", redisCfg.Addr)

	// 4. 本地 Hub 管连接，RoomBus 负责跨实例事件分发。
	hub := ws.NewHub()
	roomBus := service.NewRoomBus(
		redisClient,
		hub,
		instanceID,
		[]string{"gw-a", "gw-b"},
	)

	// 先确认 Redis 订阅，再启动 HTTP 和 Kafka 消费任务。
	subscribeCtx, subscribeCancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	subscribeErr := roomBus.Open(subscribeCtx)
	subscribeCancel()
	if subscribeErr != nil {
		log.Fatalf("初始化跨实例订阅失败: %v", subscribeErr)
	}
	defer roomBus.Close()

	// 5. 每个实例创建自己的 Producer，使用相同的 Kafka。
	kafkaBrokers := []string{"127.0.0.1:19092"}
	danmaTopic := "live-danma"

	producer := kafka.NewProducer(kafkaBrokers, danmaTopic)
	defer func() {
		if err := producer.Close(); err != nil {
			log.Printf("lifecycle: kafka producer close: %v", err)
		} else {
			log.Println("lifecycle: kafka producer closed")
		}
	}()

	// 消费后的广播和保存 ACK 改走 RoomBus。
	danmaPipeline := service.NewDanmaPipeline(
		redisClient,
		roomBus,
		kafkaBrokers,
		danmaTopic,
	)

	// 6. 仅 A 刷新共享榜单，使用两个实例汇总的在线数。
	var hotRanking *service.HotRanking
	if instanceID == "gw-a" {
		hotRanking = service.NewHotRanking(
			redisClient,
			roomBus.SnapshotOnlineCounts,
		)
	}

	// 7. 注册路由，注入 RoomBus。
	secureCookie := web.AppConfig.DefaultBool("cookie_secure", false)
	routers.Register(
		redisClient,
		secureCookie,
		hub,
		producer,
		roomBus,
	)
	log.Println("路由注册成功")

	// 8. 生命周期启动后台任务，并负责退出清理。
	web.BConfig.Listen.Graceful = true
	grace.DefaultTimeout = 15 * time.Second
	web.BeeApp.LifeCycleCallbacks = append(
		web.BeeApp.LifeCycleCallbacks,
		lifecycle.NewWeb(hub, hotRanking, danmaPipeline, roomBus),
	)

	log.Printf("starting backend on 127.0.0.1:%d", port)
	web.Run()
	log.Println("lifecycle: HTTP server run finished")
}
