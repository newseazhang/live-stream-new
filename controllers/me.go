package controllers

import (

    "errors"
    "log"
    "net/http"

    "example.com/live-stream/filters"
    "example.com/live-stream/internal/service"

    "github.com/beego/beego/v2/server/web"
	
)

type MeController struct {
	web.Controller
}

func (c *MeController) Get(){
	userID, ok := filters.CurrentUserID(c.Ctx)

	if !ok {
		c.writeJSON(http.StatusUnauthorized, map[string]string{
            "message": "请先登录",
        })
        return
	}

	user ,err := service.GetCurrentUser(c.Ctx.Request.Context(), userID)
	if errors.Is(err, service.ErrCurrentUserNotFound) {
        c.writeJSON(http.StatusUnauthorized, map[string]string{
            "message": "请重新登录",
        })
        return
    }
    if err != nil {
        log.Printf("查询当前用户失败: %v", err)
        c.writeJSON(http.StatusInternalServerError, map[string]string{
            "message": "查询用户失败",
        })
        return
    }

	    c.Ctx.Output.Header("Cache-Control", "no-store")
    c.writeJSON(http.StatusOK, map[string]any{
        "user_id":   user.UserID,
        "user_name": user.UserName,
        "email":     user.Email,
    })
}

func (c *MeController) writeJSON(status int, body any) {
    c.Ctx.Output.SetStatus(status)
    c.Data["json"] = body
    _ = c.ServeJSON()
}