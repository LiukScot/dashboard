package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LiukScot/dashboard/internal/auth"
	"github.com/LiukScot/dashboard/internal/db"
)

// upgradeServer spins up an httptest server that just upgrades incoming
// requests to WebSocket and returns the server-side conn on a channel.
// Tests use it to exercise Hub register/unregister/broadcast against real
// gorilla/websocket connections.
func upgradeServer(t *testing.T) (*httptest.Server, <-chan *websocket.Conn) {
	t.Helper()
	upgrader := websocket.Upgrader{}
	ch := make(chan *websocket.Conn, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		ch <- conn
	}))
	t.Cleanup(srv.Close)
	return srv, ch
}

func dialWS(t *testing.T, httpURL string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(httpURL, "http")
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestHubRegisterIncrementsClientCount(t *testing.T) {
	t.Parallel()
	srv, serverConns := upgradeServer(t)

	client := dialWS(t, srv.URL)
	defer client.Close()

	hub := NewHub()
	hub.Register(<-serverConns)
	assert.Equal(t, 1, hub.ClientCount())

	client2 := dialWS(t, srv.URL)
	defer client2.Close()
	hub.Register(<-serverConns)
	assert.Equal(t, 2, hub.ClientCount())
}

func TestHubUnregisterRemovesClient(t *testing.T) {
	t.Parallel()
	srv, serverConns := upgradeServer(t)

	client := dialWS(t, srv.URL)
	defer client.Close()

	hub := NewHub()
	c := hub.Register(<-serverConns)
	require.Equal(t, 1, hub.ClientCount())

	hub.Unregister(c)
	assert.Equal(t, 0, hub.ClientCount())
}

func TestHubBroadcastDeliversToClients(t *testing.T) {
	t.Parallel()
	srv, serverConns := upgradeServer(t)

	client := dialWS(t, srv.URL)
	hub := NewHub()
	hub.Register(<-serverConns)

	payload := []byte(`{"type":"metrics","ok":true}`)
	hub.Broadcast(payload)

	require.NoError(t, client.SetReadDeadline(time.Now().Add(2*time.Second)))
	mt, got, err := client.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, websocket.TextMessage, mt)
	assert.Equal(t, payload, got)
}

func TestHubBroadcastRemovesDeadClient(t *testing.T) {
	t.Parallel()
	srv, serverConns := upgradeServer(t)

	client := dialWS(t, srv.URL)
	hub := NewHub()
	hub.Register(<-serverConns)

	// Kill the client side first. Broadcast must detect the write
	// failure and unregister the dead conn so ClientCount drops to 0.
	require.NoError(t, client.Close())

	// Broadcast a couple of times so the write deadline trips even on
	// kernels that don't surface the EOF on the first send.
	for i := 0; i < 3; i++ {
		hub.Broadcast([]byte("x"))
	}

	assert.Eventually(t, func() bool {
		return hub.ClientCount() == 0
	}, 2*time.Second, 50*time.Millisecond, "dead client must be unregistered")
}

func TestHubBroadcastConcurrentWriteSafe(t *testing.T) {
	t.Parallel()
	// 4 goroutines hammering Broadcast on a single registered conn must
	// never trip gorilla/websocket's "concurrent write" panic. The
	// connWithMu wrapper serialises writes per-conn.
	srv, serverConns := upgradeServer(t)
	client := dialWS(t, srv.URL)

	// Drain the client side so the kernel buffer doesn't backpressure.
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for {
			if _, _, err := client.ReadMessage(); err != nil {
				return
			}
		}
	}()

	hub := NewHub()
	hub.Register(<-serverConns)

	panicCh := make(chan any, 1)
	start := make(chan struct{})

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					select {
					case panicCh <- r:
					default:
					}
				}
			}()
			<-start
			for i := 0; i < 100; i++ {
				hub.Broadcast([]byte("payload"))
			}
		}()
	}
	close(start)
	wg.Wait()

	select {
	case p := <-panicCh:
		t.Fatalf("broadcast panicked: %v", p)
	default:
	}

	_ = client.Close()
	<-drained
}

func TestHandleUpgradeRejectsUnauthenticated(t *testing.T) {
	t.Parallel()
	// No cookie → 401 without upgrade attempt.
	dbPath := filepath.Join(t.TempDir(), "dashboard.sqlite")
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	require.NoError(t, db.RunMigrations(database))

	authSvc := auth.NewService(database, 3600)
	ws := &WSHandler{
		hub:      NewHub(),
		authSvc:  authSvc,
		upgrader: websocket.Upgrader{},
	}

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	res := httptest.NewRecorder()
	ws.HandleUpgrade(res, req)

	assert.Equal(t, http.StatusUnauthorized, res.Code)
	assert.Contains(t, res.Body.String(), "unauthorized")
}

func TestHandleUpgradeRejectsInvalidSession(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "dashboard.sqlite")
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	require.NoError(t, db.RunMigrations(database))

	authSvc := auth.NewService(database, 3600)
	ws := &WSHandler{
		hub:      NewHub(),
		authSvc:  authSvc,
		upgrader: websocket.Upgrader{},
	}

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.AddCookie(&http.Cookie{Name: "DASHBOARD_SESSID", Value: "garbage"})
	res := httptest.NewRecorder()
	ws.HandleUpgrade(res, req)

	assert.Equal(t, http.StatusUnauthorized, res.Code)
}
