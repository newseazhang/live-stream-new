package ws

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 20 * time.Second
	maxMessageSize = 4096
	sendQueueSize  = 64
)

type Client struct {
	RoomID int64
	UserID int64

	conn      *websocket.Conn
	send      chan ServerMessage //发送消息队列：WriteLoop协程消费这个channel
	done      chan struct{}
	closeOnce sync.Once
}

// 绑定房间和用户 在查到的房间和用户之间建立连接
func NewClient(conn *websocket.Conn, roomID, userID int64) *Client {
	return &Client{
		RoomID: roomID,
		UserID: userID,
		conn:   conn,
		send:   make(chan ServerMessage, sendQueueSize),
		done:   make(chan struct{}), //无缓存 仅作信号标记
	}
}

func (c *Client) Close() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

func (c *Client) Send(message ServerMessage) bool {

	select {
	case <-c.done:
		return false // 连接已经关闭，发送失败
	case c.send <- message:
		return true // // 消息成功进入发送队列，等待 WriteLoop 发送
	default:
		c.Close() // send队列满了（64条塞满），客户端读消息太慢，判定卡死，直接踢掉
		return false
	}
}

// // ReadLoop 持续读取客户端消息；当前由控制器所在的 goroutine 阻塞执行
func (c *Client) ReadLoop(handle func(ClientMessage)) error {
	defer c.Close() // 无论什么原因退出读循环，都执行关闭
	c.conn.SetReadLimit(maxMessageSize)

	if err := c.conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		return err
	}

	//收到客户端pong包时的回调 刷新读超时时间 说明连接还存活
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		var message ClientMessage
		if err := c.conn.ReadJSON(&message); err != nil {
			return err
		}
		handle(message)

	}
}

func (c *Client) WriteLoop() error {
	defer c.Close()

	ticker := time.NewTicker(pingPeriod) //定时器
	defer ticker.Stop()

	for {
		select {

		case <-c.done: //关闭信号遇到
			return nil
		case message := <-c.send:
			//往连接中写信息时 先判断连接是否存活
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				return err
			}
			if err := c.conn.WriteJSON(message); err != nil {
				return err
			}
		case <-ticker.C:
			err := c.conn.WriteControl(
				websocket.PingMessage,
				nil,
				time.Now().Add(writeWait),
			)
			if err != nil {
				return err
			}
		}
	}
}
