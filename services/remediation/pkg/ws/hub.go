package ws

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Hub broadcasts drift events to all connected dashboard clients
type Hub struct {
	clients map[*websocket.Conn]bool
	mu      sync.Mutex
	log     *zap.Logger
}

func NewHub(log *zap.Logger) *Hub {
	return &Hub{clients: make(map[*websocket.Conn]bool), log: log}
}

// HandleWS upgrades an HTTP connection to a WebSocket and registers the client
func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Error("websocket upgrade failed", zap.Error(err))
		return
	}
	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()

	h.log.Info("dashboard client connected", zap.Int("total_clients", len(h.clients)))

	// Keep connection open; remove on close
	go func() {
		defer func() {
			h.mu.Lock()
			delete(h.clients, conn)
			h.mu.Unlock()
			conn.Close()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}()
}

// Broadcast sends a drift event to all connected clients
func (h *Hub) Broadcast(event interface{}) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for conn := range h.clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			conn.Close()
			delete(h.clients, conn)
		}
	}
}

// ClientCount returns the number of currently connected dashboard clients
func (h *Hub) ClientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}
