package service

import (
	"context"
	"errors"

	"example.com/live-stream/internal/model"
    "example.com/live-stream/internal/repo"
)
var ErrCurrentUserNotFound = errors.New("当前用户不存在")

func GetCurrentUser(ctx context.Context, userid int64)(*model.User,error){
	user, err := repo.FindUserByID(ctx, userid)

    if err != nil {
        return nil, err
    }
    if user == nil {
        return nil, ErrCurrentUserNotFound
    }

    return user, nil

}