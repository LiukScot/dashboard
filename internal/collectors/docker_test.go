package collectors

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// statsFrame is the minimal Docker stats object the streaming reader decodes.
func statsFrame(name string, totalUsage, preTotal, sysUsage, preSys uint64) map[string]any {
	return map[string]any{
		"name": name,
		"cpu_stats": map[string]any{
			"cpu_usage":        map[string]any{"total_usage": totalUsage},
			"system_cpu_usage": sysUsage,
			"online_cpus":      2,
		},
		"precpu_stats": map[string]any{
			"cpu_usage":        map[string]any{"total_usage": preTotal},
			"system_cpu_usage": preSys,
		},
		"memory_stats": map[string]any{"usage": 100, "limit": 400},
		"networks":     map[string]any{"eth0": map[string]any{"rx_bytes": 10, "tx_bytes": 20}},
	}
}

func testCollector(t *testing.T, mux *http.ServeMux) *DockerCollector {
	t.Helper()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	addr := strings.TrimPrefix(server.URL, "http://")
	dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", addr)
	}
	streamClient := &http.Client{Transport: &http.Transport{DialContext: dial}}
	c := &DockerCollector{
		client:   &http.Client{Transport: &http.Transport{DialContext: dial}, Timeout: time.Second},
		streamer: NewStatsStreamer(streamClient),
	}
	t.Cleanup(c.Close)
	return c
}

func eventuallyStats(t *testing.T, c *DockerCollector, want int) []ContainerStats {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		stats, err := c.GetAllStats()
		if err != nil {
			t.Fatalf("GetAllStats: %v", err)
		}
		if len(stats) >= want {
			return stats
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("stats cache never reached %d entries", want)
	return nil
}

func TestGetAllStatsStreamsRunningContainers(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1.43/containers/json", func(w http.ResponseWriter, r *http.Request) {
		containers := []map[string]any{
			{"Id": "aaaaaaaaaaaa1111", "Names": []string{"/alpha"}, "State": "running", "Status": "Up"},
			{"Id": "bbbbbbbbbbbb2222", "Names": []string{"/beta"}, "State": "running", "Status": "Up"},
			{"Id": "cccccccccccc3333", "Names": []string{"/gamma"}, "State": "running", "Status": "Up"},
			{"Id": "dddddddddddd4444", "Names": []string{"/stopped"}, "State": "exited", "Status": "Exited"},
		}
		if err := json.NewEncoder(w).Encode(containers); err != nil {
			t.Fatalf("encode containers: %v", err)
		}
	})
	// Streaming stats endpoint: emit a couple of frames then keep the
	// connection open (the reader caches the latest frame).
	mux.HandleFunc("/v1.43/containers/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/stats") {
			http.NotFound(w, r)
			return
		}
		flusher, _ := w.(http.Flusher)
		enc := json.NewEncoder(w)
		for i := 0; i < 2; i++ {
			if err := enc.Encode(statsFrame("test", 200, 100, 400, 200)); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		<-r.Context().Done()
	})

	collector := testCollector(t, mux)

	// First call opens streams; data arrives asynchronously, so poll until the
	// cache fills. Only the 3 running containers should ever appear.
	stats := eventuallyStats(t, collector, 3)
	if len(stats) != 3 {
		t.Fatalf("expected stats for 3 running containers, got %d", len(stats))
	}
	for _, s := range stats {
		if s.CPUPercent <= 0 {
			t.Fatalf("expected computed cpu%% for %s, got %v", s.ID, s.CPUPercent)
		}
		if s.MemPercent != 25 {
			t.Fatalf("expected mem%% 25, got %v", s.MemPercent)
		}
	}
}

func TestStatsStreamerReconcileStopsRemovedContainers(t *testing.T) {
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

	collector := testCollector(t, mux)
	streamer := collector.streamer

	alpha := Container{ID: "aaaaaaaaaaaa", Name: "alpha"}
	beta := Container{ID: "bbbbbbbbbbbb", Name: "beta"}

	streamer.Snapshot([]Container{alpha, beta})
	streamer.mu.Lock()
	openAfterStart := len(streamer.streams)
	streamer.mu.Unlock()
	if openAfterStart != 2 {
		t.Fatalf("expected 2 streams, got %d", openAfterStart)
	}

	// beta disappears; its stream must be torn down.
	streamer.Snapshot([]Container{alpha})
	streamer.mu.Lock()
	_, betaStillOpen := streamer.streams[beta.ID]
	openAfterRemove := len(streamer.streams)
	streamer.mu.Unlock()
	if betaStillOpen {
		t.Fatal("expected beta stream to be removed after reconcile")
	}
	if openAfterRemove != 1 {
		t.Fatalf("expected 1 stream after removal, got %d", openAfterRemove)
	}
}

func TestListContainersReturnsErrorOnHTTPFailure(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1.43/containers/json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(map[string]string{"message": "boom"}); err != nil {
			t.Fatalf("encode error payload: %v", err)
		}
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	addr := strings.TrimPrefix(server.URL, "http://")
	collector := &DockerCollector{
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "tcp", addr)
				},
			},
			Timeout: time.Second,
		},
	}

	_, err := collector.ListContainers()
	if err == nil {
		t.Fatal("expected HTTP failure to return error")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected error to include status and body, got: %v", err)
	}
}
