package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LiukScot/dashboard/internal/auth"
	"github.com/LiukScot/dashboard/internal/collectors"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]bool
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*websocket.Conn]bool),
	}
}

func (h *Hub) Register(conn *websocket.Conn) {
	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()
}

func (h *Hub) Unregister(conn *websocket.Conn) {
	h.mu.Lock()
	delete(h.clients, conn)
	h.mu.Unlock()
	conn.Close()
}

func (h *Hub) Broadcast(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for conn := range h.clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			go h.Unregister(conn)
		}
	}
}

func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

type WSHandler struct {
	hub       *Hub
	authSvc   *auth.Service
	sysColl   *collectors.SystemCollector
	dockerColl *collectors.DockerCollector
}

func NewWSHandler(hub *Hub, authSvc *auth.Service, sysColl *collectors.SystemCollector, dockerColl *collectors.DockerCollector) *WSHandler {
	return &WSHandler{
		hub:        hub,
		authSvc:    authSvc,
		sysColl:    sysColl,
		dockerColl: dockerColl,
	}
}

func (ws *WSHandler) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	_, err := auth.ValidateSessionFromCookie(ws.authSvc, r)
	if err != nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}

	ws.hub.Register(conn)
	log.Printf("ws client connected (%d total)", ws.hub.ClientCount())

	go func() {
		defer ws.hub.Unregister(conn)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}()
}

type MetricsBroadcast struct {
	Type    string                     `json:"type"`
	System  *collectors.SystemMetrics  `json:"system,omitempty"`
	Network []collectors.NetworkMetrics `json:"network,omitempty"`
	Docker  []collectors.ContainerStats `json:"docker,omitempty"`
}

func (ws *WSHandler) StartBroadcastLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			if ws.hub.ClientCount() == 0 {
				continue
			}

			msg := MetricsBroadcast{Type: "metrics"}

			if sys, err := ws.sysColl.Collect(); err == nil {
				msg.System = sys
			}
			if net, err := ws.sysColl.CollectNetwork(); err == nil {
				msg.Network = net
			}
			if stats, err := ws.dockerColl.GetAllStats(); err == nil {
				msg.Docker = stats
			}

			data, err := json.Marshal(msg)
			if err != nil {
				log.Printf("ws marshal error: %v", err)
				continue
			}

			ws.hub.Broadcast(data)
		}
	}()
}
