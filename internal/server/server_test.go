package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/LiukScot/dashboard/internal/auth"
	"github.com/LiukScot/dashboard/internal/config"
	"github.com/LiukScot/dashboard/internal/db"
)

func TestHandleLoginRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "dashboard.sqlite")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if err := db.RunMigrations(database); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	srv := &Server{
		cfg:     &config.Config{SessionTTL: 3600},
		authSvc: auth.NewService(database, 3600),
	}

	password := strings.Repeat("x", 5000)
	body := fmt.Sprintf(`{"email":"person@example.com","password":"%s"}`, password)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	res := httptest.NewRecorder()

	srv.handleLogin(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for oversized body, got %d", res.Code)
	}
}

func TestHandleSessionReturnsAuthenticatedFalseWithoutCookie(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "dashboard.sqlite")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if err := db.RunMigrations(database); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	srv := &Server{
		cfg:     &config.Config{SessionTTL: 3600},
		authSvc: auth.NewService(database, 3600),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	res := httptest.NewRecorder()

	srv.handleSession(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var body struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body.Authenticated {
		t.Fatal("expected unauthenticated response without cookie")
	}
}

func TestHubBroadcastConcurrentDoesNotPanic(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{}
	serverConnCh := make(chan *websocket.Conn, 1)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		serverConnCh <- conn
	}))
	t.Cleanup(httpServer.Close)

	clientConn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() { _ = clientConn.Close() })

	serverConn := <-serverConnCh
	t.Cleanup(func() { _ = serverConn.Close() })

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := clientConn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	hub := NewHub()
	hub.Register(serverConn)

	panicCh := make(chan any, 1)
	start := make(chan struct{})

	var wg sync.WaitGroup
	writer := func() {
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
	}

	for range 4 {
		wg.Add(1)
		go writer()
	}
	close(start)
	wg.Wait()

	select {
	case p := <-panicCh:
		t.Fatalf("broadcast panicked: %v", p)
	default:
	}

	_ = clientConn.Close()
	<-done
}

func TestRequestOriginAllowedAllowsConfiguredOrigin(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4200/ws", nil)
	req.Host = "127.0.0.1:4200"
	req.Header.Set("Origin", "http://localhost:5173")

	allowed := parseAllowedOrigins("http://localhost:4200,http://localhost:5173")
	if !requestOriginAllowed(req, allowed) {
		t.Fatal("expected configured origin to be allowed")
	}
}

func TestRequestOriginAllowedAllowsSameHostOrigin(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "http://100.75.217.127:4200/ws", nil)
	req.Host = "100.75.217.127:4200"
	req.Header.Set("Origin", "http://100.75.217.127:4200")

	allowed := parseAllowedOrigins("http://localhost:4200,http://127.0.0.1:4200")
	if !requestOriginAllowed(req, allowed) {
		t.Fatal("expected same-host origin to be allowed")
	}
}

func TestRequestOriginAllowedRejectsOtherOrigin(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4200/ws", nil)
	req.Host = "127.0.0.1:4200"
	req.Header.Set("Origin", "http://evil.example")

	allowed := parseAllowedOrigins("http://localhost:4200,http://localhost:5173")
	if requestOriginAllowed(req, allowed) {
		t.Fatal("expected unrelated origin to be rejected")
	}
}
