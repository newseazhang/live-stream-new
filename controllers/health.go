package controllers

import "github.com/beego/beego/v2/server/web"

type HealthController struct {
	web.Controller //  获得beego的请求以及响应能力
}

// 处理 GET 请求；先准备响应数据，再通过 ServeJSON() 写回客户端。
func (c *HealthController) Get() {
	c.Data["json"] = map[string]string{
		"status":  "ok",
		"service": "live-stream",
	}
	c.ServeJSON()
}
