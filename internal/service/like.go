package service

import (
	"context"
	"errors"
	"fmt"

	"example.com/live-stream/internal/repo"
)

var (
	ErrInvalidLikeIdentity = errors.New("点赞需要有效的房间和登录用户")
	ErrRoomCannotLike      = errors.New("房间不存在或未开播")
)

func LikeRoom(
	ctx context.Context,
	roomID, userID int64,
) (int64, error) {
	if roomID <= 0 || userID <= 0 {
		return 0, ErrInvalidLikeIdentity
	}

	total, err := repo.IncrementRoomLike(ctx, roomID)

	if errors.Is(err, repo.ErrRoomCannotLike) {
		return 0, ErrRoomCannotLike
	}
	if err != nil {
		return 0, fmt.Errorf("点赞失败: %w", err)
	}

	return total, nil
}
