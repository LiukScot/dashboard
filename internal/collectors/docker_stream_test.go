package collectors

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock is a manually advanced clock for deterministic staleness/backoff.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Unix(1_700_000_000, 0)} }

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func streamerForTest(t *testing.T, mux *http.ServeMux) *StatsStreamer {
	t.Helper()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	addr := strings.TrimPrefix(server.URL, "http://")
	dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", addr)
	}
	st := NewStatsStreamer(&http.Client{Transport: &http.Transport{DialContext: dial}})
	t.Cleanup(st.Close)
	return st
}

func waitForStreamDone(t *testing.T, st *StatsStreamer, id string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st.mu.Lock()
		s, ok := st.streams[id]
		st.mu.Unlock()
		if ok {
			s.mu.Lock()
			done := s.done
			s.mu.Unlock()
			if done {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("stream %s never marked done", id)
}

func TestStreamerReopensDroppedStreamOnReconcile(t *testing.T) {
	t.Parallel()

	var hits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.43/containers/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/stats") {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&hits, 1)
		// Serve exactly one frame, then return so the connection drops (EOF).
		enc := json.NewEncoder(w)
		_ = enc.Encode(statsFrame("test", 200, 100, 400, 200))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Returning here closes the response body → reader sees EOF.
	})

	st := streamerForTest(t, mux)
	clock := newFakeClock()
	st.now = clock.now
	st.reconnectBackoff = 0 // re-open immediately once done

	id := "aaaaaaaaaaaa"
	container := []Container{{ID: id, Name: "alpha"}}

	st.Snapshot(container)
	waitForStreamDone(t, st, id)
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected 1 connection before reconnect, got %d", got)
	}

	// The container is still running; the next reconcile must re-open the
	// dropped stream rather than serving a frozen frame forever.
	st.Snapshot(container)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&hits) >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&hits); got < 2 {
		t.Fatalf("dropped stream was not re-opened on reconcile; hits=%d", got)
	}
}

func TestStreamerOmitsStaleFrame(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1.43/containers/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/stats") {
			http.NotFound(w, r)
			return
		}
		enc := json.NewEncoder(w)
		_ = enc.Encode(statsFrame("test", 200, 100, 400, 200))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	})

	st := streamerForTest(t, mux)
	clock := newFakeClock()
	st.now = clock.now
	st.staleAfter = 10 * time.Second

	id := "aaaaaaaaaaaa"
	container := []Container{{ID: id, Name: "alpha"}}

	// Wait until a frame is cached (stored with clock's current time).
	deadline := time.Now().Add(2 * time.Second)
	var fresh []ContainerStats
	for time.Now().Before(deadline) {
		fresh = st.Snapshot(container)
		if len(fresh) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(fresh) != 1 {
		t.Fatalf("expected fresh frame to be served, got %d", len(fresh))
	}

	// Advance past the staleness window without any new frame: the wedged
	// stream must surface as "no data".
	clock.advance(11 * time.Second)
	if stale := st.Snapshot(container); len(stale) != 0 {
		t.Fatalf("expected stale frame to be omitted, got %d", len(stale))
	}
}

func TestStreamerBacksOffFailingEndpoint(t *testing.T) {
	t.Parallel()

	var hits int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1.43/containers/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/stats") {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusNotFound) // persistent failure
	})

	st := streamerForTest(t, mux)
	clock := newFakeClock()
	st.now = clock.now
	st.reconnectBackoff = 30 * time.Second

	id := "aaaaaaaaaaaa"
	container := []Container{{ID: id, Name: "alpha"}}

	st.Snapshot(container)
	waitForStreamDone(t, st, id)
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected 1 attempt, got %d", got)
	}

	// Within the backoff window, reconcile must NOT hammer the endpoint.
	st.Snapshot(container)
	st.Snapshot(container)
	time.Sleep(100 * time.Millisecond)
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected no reconnect during backoff, got %d hits", got)
	}

	// Past the backoff window, reconcile re-attempts.
	clock.advance(31 * time.Second)
	st.Snapshot(container)
	waitForStreamDone(t, st, id)
	if got := atomic.LoadInt32(&hits); got < 2 {
		t.Fatalf("expected reconnect after backoff elapsed, got %d hits", got)
	}
}
