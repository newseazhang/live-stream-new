package controllers

import (
	"log"
	"net/http"
	"time"

	"example.com/live-stream/internal/auth"
	beecontext "github.com/beego/beego/v2/server/web/context"
	"github.com/redis/go-redis/v9"
)

// 删除session会话连接 相当于登出
func NewLogoutHandler(
	client *redis.Client,
	secureCookie bool,
) func(*beecontext.Context) {
	return func(ctx *beecontext.Context) {
		sid := ctx.GetCookie("sid")

		if err := auth.DeleteSession(ctx.Request.Context(), client, sid); err != nil {
			log.Printf("退出登录失败: %v", err)
			ctx.Output.SetStatus(http.StatusInternalServerError)
			_ = ctx.JSONResp(map[string]string{
				"message": "退出登录失败，请重试",
			})
			return
		}

		http.SetCookie(ctx.ResponseWriter, &http.Cookie{
			Name:     "sid",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			Expires:  time.Unix(0, 0).UTC(),
			HttpOnly: true,
			Secure:   secureCookie,
			SameSite: http.SameSiteLaxMode,
		})
		ctx.ResponseWriter.WriteHeader(http.StatusNoContent)
	}
}
