package controllers

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"

	"example.com/live-stream/internal/service"

	"github.com/beego/beego/v2/server/web"
)

type RegisterController struct {
	web.Controller
}

func (c *RegisterController) Post() {
	request := c.Ctx.Request

	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		c.writeJSON(http.StatusUnsupportedMediaType, map[string]string{
			"message": "请发送 JSON 请求",
		})
		return
	}

	// 解析 JSON 请求体
	var input struct {
		UserName string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	body := http.MaxBytesReader(c.Ctx.ResponseWriter, request.Body, 4096) //限制请求体最大只能读取 **4096 字节** 防止超大body耗尽服务器内存
	data, err := io.ReadAll(body) // data
	if err != nil {
		c.writeJSON(http.StatusBadRequest, map[string]string{
			"message": "无法读取请求体",
		})
		return
	}

	if err := json.Unmarshal(data, &input); err != nil {
		c.writeJSON(http.StatusBadRequest, map[string]string{
			"message": "请求体格式错误",   // http.StatusBadRequest：400
		}) //json 字节切片反序列化 填充到input中 传指针
		return
	}

	user, err := service.RegisterUser(request.Context(), service.RegisterInput{
		UserName: input.UserName,
		Email:    input.Email,
		Password: input.Password,
	})

	switch {
	case errors.Is(err, service.ErrInvalidRegistration):
		c.writeJSON(http.StatusBadRequest, map[string]string{
			"message": err.Error(),
		})
	case errors.Is(err, service.ErrUserAlreadyExists):
		c.writeJSON(http.StatusConflict, map[string]string{
			"message": "用户名或邮箱已注册",
		})
	case err != nil:
		log.Printf("注册失败: %v", err)
		c.writeJSON(http.StatusInternalServerError, map[string]string{
			"message": "注册失败",
		})
	default:
		c.writeJSON(http.StatusCreated, map[string]any{
			"user_id":   user.UserID,
			"user_name": user.UserName,
			"email":     user.Email,
		})
	}
}

func (c *RegisterController) writeJSON(status int, body any) {
	c.Ctx.Output.SetStatus(status)
	c.Data["json"] = body
	_ = c.ServeJSON()
}