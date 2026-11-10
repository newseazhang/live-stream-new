package lifecycle

import (
	"context"
	"log"
	"sync"
	"time"

	"example.com/live-stream/internal/service"
	"example.com/live-stream/internal/ws"
	"github.com/beego/beego/v2/server/web"
)

// Web 实现 Beego 的 LifeCycleCallback，由 main 注册。
type Web struct {
	hub        *ws.Hub
	ranking    *service.HotRanking
	pipeline   *service.DanmaPipeline
	bus        *service.RoomBus
	cancelJobs context.CancelFunc
	jobsDone   chan struct{}
}

var _ web.LifeCycleCallback = (*Web)(nil)

func NewWeb(hub *ws.Hub, ranking *service.HotRanking, pipeline *service.DanmaPipeline, bus *service.RoomBus) *Web {
	return &Web{hub: hub, ranking: ranking, pipeline: pipeline, bus: bus}
}

func (w *Web) AfterStart(_ *web.HttpServer) {
	log.Println("lifecycle: HTTP server started")
	ctx, cancel := context.WithCancel(context.Background())
	w.cancelJobs = cancel
	w.jobsDone = make(chan struct{})
	var jobs sync.WaitGroup
	start := func(run func(context.Context)) {
		jobs.Add(1)
		go func() {
			defer jobs.Done()
			run(ctx)
		}()
	}
	start(w.bus.Run)
	start(w.pipeline.Run)
	// 固定由 gw-a 刷榜，避免两个进程交替覆盖榜单。
	if w.ranking != nil {
		start(w.ranking.Run)
	}
	go func() {
		jobs.Wait()
		close(w.jobsDone)
	}()
	log.Println("lifecycle: room bus and kafka consumers started")
}

func (w *Web) BeforeShutdown(_ *web.HttpServer) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	log.Println("lifecycle: stopping websocket clients")
	if err := w.hub.Shutdown(ctx); err != nil {
		log.Printf("lifecycle: websocket shutdown incomplete: %v", err)
	} else {
		log.Println("lifecycle: websocket clients stopped")
	}
	if w.cancelJobs == nil {
		return
	}
	w.cancelJobs()
	select {
	case <-w.jobsDone:
		log.Println("lifecycle: background jobs stopped")
	case <-ctx.Done():
		log.Printf("lifecycle: background shutdown incomplete: %v", ctx.Err())
	}
}
