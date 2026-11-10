package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"time"

	"example.com/live-stream/internal/auth"
	"example.com/live-stream/internal/service"

	beecontext "github.com/beego/beego/v2/server/web/context"
	"github.com/redis/go-redis/v9"
)

func NewLoginHandler(
	client *redis.Client,
	secureCookie bool,
) func(*beecontext.Context) {
	return func(ctx *beecontext.Context) {
		mediaType, _, err := mime.ParseMediaType(ctx.Request.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			writeLoginError(ctx, http.StatusUnsupportedMediaType, "请发送 JSON 请求")
			return
		}

		var input struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}

		body := http.MaxBytesReader(ctx.ResponseWriter, ctx.Request.Body, 4096)
		decoder := json.NewDecoder(body)

		if err := decoder.Decode(&input); err != nil {
			writeLoginError(ctx, http.StatusBadRequest, "请求体格式错误")
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			writeLoginError(ctx, http.StatusBadRequest, "请求体格式错误")
			return
		}

		requestCtx := ctx.Request.Context()
		user, err := service.LoginUser(requestCtx, input.Email, input.Password)
		if errors.Is(err, service.ErrInvalidCredentials) {
			writeLoginError(ctx, http.StatusUnauthorized, "邮箱或密码错误")
			return
		}
		if err != nil {
			log.Printf("登录失败: %v", err)
			writeLoginError(ctx, http.StatusInternalServerError, "登录失败")
			return
		}

		sessionID, err := auth.CreateSession(requestCtx, client, user.UserID)
		if err != nil {
			log.Printf("创建会话失败: %v", err)
			writeLoginError(ctx, http.StatusInternalServerError, "创建会话失败")
			return
		}

		http.SetCookie(ctx.ResponseWriter, &http.Cookie{
			Name:     "sid",
			Value:    sessionID,
			Path:     "/",
			MaxAge:   int(auth.SessionTTL / time.Second),
			HttpOnly: true,
			Secure:   secureCookie,
			SameSite: http.SameSiteLaxMode,
		})

		ctx.Output.Header("Cache-Control", "no-store")
		_ = ctx.JSONResp(map[string]any{
			"user_id":   user.UserID,
			"user_name": user.UserName,
			"email":     user.Email,
		})
	}
}

func writeLoginError(ctx *beecontext.Context, status int, message string) {
	ctx.Output.SetStatus(status)
	_ = ctx.JSONResp(map[string]string{"message": message})
}
