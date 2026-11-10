package filters

import (
    "errors"
    "log"
    "net/http"

    "example.com/live-stream/internal/auth"

    "github.com/beego/beego/v2/server/web"
    beecontext "github.com/beego/beego/v2/server/web/context"
    "github.com/redis/go-redis/v9"
)

const currentUserIDKey = "auth.current_user_id"

func RequireSession(client *redis.Client) web.FilterFunc {
    return func(ctx *beecontext.Context) {
        sid := ctx.GetCookie("sid")
        userID, err := auth.GetSessionUserID(
            ctx.Request.Context(),
            client,
            sid,
        )

        if errors.Is(err, auth.ErrInvalidSession) {
            ctx.Output.SetStatus(http.StatusUnauthorized)
            _ = ctx.JSONResp(map[string]string{
                "message": "请先登录",
            })
            return
        }

        if err != nil {
            log.Printf("鉴权失败: %v", err)
            ctx.Output.SetStatus(http.StatusInternalServerError)
            _ = ctx.JSONResp(map[string]string{
                "message": "无法验证登录状态",
            })
            return
        }

        ctx.Input.SetData(currentUserIDKey, userID)
    }
}
//
func CurrentUserID(ctx *beecontext.Context) (int64, bool) {
    userID, ok := ctx.Input.GetData(currentUserIDKey).(int64)
    return userID, ok && userID > 0
}