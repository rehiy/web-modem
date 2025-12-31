package handlers

import (
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/rehiy/web-modem/services"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// HandleWebSocket 将 HTTP 连接升级为 WebSocket 连接
// 并将串口事件流式传输到客户端
func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// 订阅事件监听器
	ch, cancel := services.GetEventListener().Subscribe(100)
	defer cancel()

	// 流式传输消息
	for msg := range ch {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			return
		}
	}
}
