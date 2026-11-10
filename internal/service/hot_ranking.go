package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"example.com/live-stream/internal/cache"
	"example.com/live-stream/internal/repo"
	"github.com/redis/go-redis/v9"
)

type HotRanking struct {
	activity *cache.RoomActivity
	ranking  *cache.HotRanking
	snapshot func(context.Context) (map[int64]int, error)
}

func NewHotRanking(client *redis.Client, snapshot func(context.Context) (map[int64]int, error)) *HotRanking {
	return &HotRanking{
		activity: cache.NewRoomActivity(client),
		ranking:  cache.NewHotRanking(client),
		snapshot: snapshot,
	}
}

/*
`Refresh` 执行一轮业务，`Run` 编排周期与取消。
先读完本轮所有分数，再调用 Replace；某个分钟桶损坏时返回错误，本轮不发布残缺榜。
旧榜保持到下一轮成功刷新或 TTL 到期，错误会带上房间 ID。*/

// Refresh 先算完整轮分数，再发布；读取失败时不写入残缺榜单。
func (r *HotRanking) Refresh(ctx context.Context) error {
	roomIDs, err := repo.ListLiveRoomIDs(ctx)
	if err != nil {
		return err
	}

	online, err := r.snapshot(ctx)
	if err != nil {
		return fmt.Errorf("读取跨实例在线数: %w", err)
	}
	now := time.Now().UTC()
	scores := make(map[int64]float64, len(roomIDs))

	for _, roomID := range roomIDs {
		counts, err := r.activity.ReadRecent(ctx, roomID, now)
		if err != nil {
			return fmt.Errorf("读取房间%d热度: %w", roomID, err)
		}
		// 评分规则 在线人数 + 3 *弹幕数 + 点赞数   在五分钟之内的
		scores[roomID] = float64(online[roomID]) +
			3*float64(counts.Danmas) + float64(counts.Likes)
	}

	return r.ranking.Replace(ctx, scores)
}

// Run 由生命周期启动一次；每轮串行执行，共用可取消的父 context。
func (r *HotRanking) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		if ctx.Err() != nil {
			return
		}

		refreshCtx, cancel := context.WithTimeout(
			ctx, 3*time.Second,
		)
		// 每个更新只给3秒 超时直接取消
		err := r.Refresh(refreshCtx)
		cancel()

		if err != nil && ctx.Err() == nil {
			log.Printf("hot ranking refresh: %v", err)
		}

		select {
		case <-ctx.Done(): // 如果收到取消信号就结束run
			return
		case <-ticker.C: //
		}
	}
}

var ErrInvalidHotLimit = errors.New("limit 必须是 1–20 的整数")

func GetHotRooms(
	ctx context.Context,
	ranking *cache.HotRanking,
	limit int,
) ([]cache.HotRoom, error) {
	if limit < 1 || limit > 20 {
		return nil, ErrInvalidHotLimit
	}
	return ranking.Top(ctx, limit)
}
