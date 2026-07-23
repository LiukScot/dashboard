package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/LiukScot/dashboard/internal/auth"
	"github.com/LiukScot/dashboard/internal/collectors"
	"github.com/LiukScot/dashboard/internal/config"
)

func newUpgrader(allowedOrigins string, requireHTTPS bool) websocket.Upgrader {
	allowed := parseAllowedOrigins(allowedOrigins)
	return websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return requestOriginAllowed(r, allowed, requireHTTPS)
		},
	}
}

// connWithMu wraps a websocket.Conn with a write mutex to prevent
// concurrent WriteMessage calls (gorilla/websocket requires this).
type connWithMu struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*connWithMu]bool
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[*connWithMu]bool),
	}
}

func (h *Hub) Register(conn *websocket.Conn) *connWithMu {
	c := &connWithMu{conn: conn}
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
	return c
}

func (h *Hub) Unregister(c *connWithMu) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	c.conn.Close()
}

func (h *Hub) Broadcast(data []byte) {
	h.mu.RLock()
	var failed []*connWithMu
	for c := range h.clients {
		c.mu.Lock()
		// 10s write deadline so a stuck client doesn't pile up goroutines.
		_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		err := c.conn.WriteMessage(websocket.TextMessage, data)
		c.mu.Unlock()
		if err != nil {
			failed = append(failed, c)
		}
	}
	h.mu.RUnlock()

	for _, c := range failed {
		h.Unregister(c)
	}
}

func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

type WSHandler struct {
	hub        *Hub
	authSvc    *auth.Service
	sysColl    *collectors.SystemCollector
	dockerColl *collectors.DockerCollector
	upgrader   websocket.Upgrader
}

func NewWSHandler(hub *Hub, authSvc *auth.Service, sysColl *collectors.SystemCollector, dockerColl *collectors.DockerCollector, cfg *config.Config) *WSHandler {
	return &WSHandler{
		hub:        hub,
		authSvc:    authSvc,
		sysColl:    sysColl,
		dockerColl: dockerColl,
		upgrader:   newUpgrader(cfg.AllowedOrigins, cfg.CookieSecure),
	}
}

// wsMaxMessageBytes caps inbound frame size to prevent a single client from
// asking us to allocate arbitrarily large buffers (gorilla/websocket reads
// the whole frame before returning).
const wsMaxMessageBytes = 1 << 16 // 64 KiB

func (ws *WSHandler) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	_, err := auth.ValidateSessionFromCookie(ws.authSvc, r)
	if err != nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	conn, err := ws.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}

	conn.SetReadLimit(wsMaxMessageBytes)

	c := ws.hub.Register(conn)
	log.Printf("ws client connected (%d total)", ws.hub.ClientCount())

	go func() {
		defer ws.hub.Unregister(c)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}()
}

type MetricsBroadcast struct {
	Type    string                      `json:"type"`
	System  *collectors.SystemMetrics   `json:"system,omitempty"`
	Network []collectors.NetworkMetrics `json:"network,omitempty"`
	Docker  []collectors.ContainerStats `json:"docker,omitempty"`
}

// StartBroadcastLoop runs the metrics broadcast goroutine until ctx is
// cancelled. The returned channel closes once the goroutine has exited and the
// ticker is stopped, letting a caller wait for a clean shutdown.
func (ws *WSHandler) StartBroadcastLoop(ctx context.Context, interval time.Duration) <-chan struct{} {
	ticker := time.NewTicker(interval)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

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
	return done
}
