package routers

import (
	"net/http"

	"example.com/live-stream/controllers"
	"example.com/live-stream/filters"

	"github.com/beego/beego/v2/server/web"
	"github.com/redis/go-redis/v9"

	beecontext "github.com/beego/beego/v2/server/web/context"

	"example.com/live-stream/internal/kafka"
	"example.com/live-stream/internal/service"
	"example.com/live-stream/internal/ws"
)

func Register(redisClient *redis.Client, secureCookie bool, hub *ws.Hub, producer *kafka.Producer, bus *service.RoomBus) {
	web.Router(
		"/health",
		&controllers.HealthController{},
		"get:Get",
	)

	//注册
	web.Router("/api/v1/auth/register",
		&controllers.RegisterController{},
		"post:Post")

	// 登录
	web.Post(
		"/api/v1/auth/login",
		controllers.NewLoginHandler(redisClient, secureCookie),
	)
	//刷新页面后不需要重新登录
	const mePath = "/api/v1/auth/me"

	web.InsertFilter(
		mePath,
		web.BeforeExec,
		filters.RequireSession(redisClient),
		web.WithReturnOnOutput(true),
	)

	web.Router(mePath, &controllers.MeController{}, "get:Get")

	//登出

	web.Post("/api/v1/auth/logout", controllers.NewLogoutHandler(redisClient, secureCookie))

	//创建房间 ：绑定路由与鉴权

	const roomsPath = "/api/v1/rooms"
	requireRoomSession := filters.RequireSession(redisClient)

	web.InsertFilter(roomsPath, web.BeforeExec, func(ctx *beecontext.Context) {
		if ctx.Request.Method == http.MethodPost {
			requireRoomSession(ctx)
		}
	}, web.WithReturnOnOutput(true))

	web.Post(roomsPath, controllers.CreateRoom)

	// 查询房间
	const myRoomPath = "/api/v1/rooms/mine"

	web.InsertFilter(myRoomPath, web.BeforeExec, requireRoomSession,
		web.WithReturnOnOutput(true))
	web.Get(myRoomPath, controllers.GetMyRoom)

	web.Get(roomsPath+"/:room_id", controllers.GetRoomDetail)

	//查看开播中的房间

	web.Get(roomsPath, controllers.ListRooms)

	web.Get("/api/v1/rooms/:room_id/messages/sync", controllers.NewMessageSyncHandler(redisClient))

	//开播
	const startRoomPath = roomsPath + "/:room_id/start"

	web.InsertFilter(startRoomPath, web.BeforeExec, requireRoomSession,
		web.WithReturnOnOutput(true))
	web.Post(startRoomPath, controllers.StartRoom)

	// 使用 websocket
	web.Get("/api/v1/rooms/:room_id/ws", controllers.NewWebSocketHandler(redisClient, hub, producer, bus))
	/*
			routers.Register 绑定 GET 路由
		  → controllers.GetRoomMessages
		      解析 room_id、before_id、limit
		  → service.GetRoomMessages
		      确认房间存在、处理分页与排序
		  → repo.ListRoomMessagesBefore
		      按房间和 ID 查询 SQLite
		  → Controller 返回 JSON
	*/
	web.Get(
		"/api/v1/rooms/hot",
		controllers.NewHotRankingHandler(redisClient),
	)
	web.Get(
		"/api/v1/rooms/:room_id/messages",
		controllers.NewMessageHistoryHandler(redisClient),
	)

}
