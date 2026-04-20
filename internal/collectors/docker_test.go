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

func TestGetAllStatsCollectsRunningContainersInParallel(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1.43/containers/json", func(w http.ResponseWriter, r *http.Request) {
		containers := []map[string]any{
			{"Id": "aaaaaaaaaaaa1111", "Names": []string{"/alpha"}, "State": "running", "Status": "Up"},
			{"Id": "bbbbbbbbbbbb2222", "Names": []string{"/beta"}, "State": "running", "Status": "Up"},
			{"Id": "cccccccccccc3333", "Names": []string{"/gamma"}, "State": "running", "Status": "Up"},
		}
		if err := json.NewEncoder(w).Encode(containers); err != nil {
			t.Fatalf("encode containers: %v", err)
		}
	})
	mux.HandleFunc("/v1.43/containers/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/stats") {
			http.NotFound(w, r)
			return
		}

		time.Sleep(150 * time.Millisecond)
		stats := map[string]any{
			"name": "test",
			"cpu_stats": map[string]any{
				"cpu_usage":        map[string]any{"total_usage": 200},
				"system_cpu_usage": 400,
				"online_cpus":      2,
			},
			"precpu_stats": map[string]any{
				"cpu_usage":        map[string]any{"total_usage": 100},
				"system_cpu_usage": 200,
			},
			"memory_stats": map[string]any{
				"usage": 100,
				"limit": 400,
			},
			"networks": map[string]any{
				"eth0": map[string]any{"rx_bytes": 10, "tx_bytes": 20},
			},
		}
		if err := json.NewEncoder(w).Encode(stats); err != nil {
			t.Fatalf("encode stats: %v", err)
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

	start := time.Now()
	stats, err := collector.GetAllStats()
	if err != nil {
		t.Fatalf("GetAllStats: %v", err)
	}
	elapsed := time.Since(start)

	if len(stats) != 3 {
		t.Fatalf("expected stats for 3 containers, got %d", len(stats))
	}
	if elapsed >= 300*time.Millisecond {
		t.Fatalf("expected parallel collection under 300ms, got %s", elapsed)
	}
}
