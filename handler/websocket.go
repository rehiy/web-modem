package handler

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/rehiy/web-modem/service"
)

// WebSocketHandler WebSocket处理器
type WebSocketHandler struct {
	upgrader websocket.Upgrader
}

// NewWebSocketHandler 创建新的WebSocket处理器
func NewWebSocketHandler() *WebSocketHandler {
	return &WebSocketHandler{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// HandleWebSocket 处理WebSocket连接
func (h *WebSocketHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("WebSocket client connected: %s", r.RemoteAddr)

	for {
		select {
		case event, ok := <-service.ModemEvent:
			if !ok {
				log.Printf("WebSocket: ModemEvent channel closed")
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(event)); err != nil {
				log.Printf("WebSocket client disconnected: %v(%s)", r.RemoteAddr, err)
				return
			}
		case <-time.After(30 * time.Second):
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("WebSocket client disconnected: %v(%s)", r.RemoteAddr, err)
				return
			}
		}
	}
}
