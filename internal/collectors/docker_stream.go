package collectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
)

// StatsStreamer removes the per-tick N+1 in docker stats collection. Instead of
// issuing one one-shot `/stats?stream=false` request per container on every
// broadcast tick — each of which makes the Docker daemon collect twice with a
// ~1s gap to compute CPU deltas — it keeps a single long-lived streaming
// connection per running container. Docker pushes a fresh stats frame roughly
// once per second on that connection; the latest frame is cached, so a
// broadcast tick reads memory instead of fanning out N blocking HTTP calls.
//
// The Docker Engine API has no aggregate stats endpoint, so one connection per
// container is the minimum; the win is that connections are reused across ticks
// rather than re-opened every tick.
type StatsStreamer struct {
	client *http.Client

	mu      sync.Mutex
	streams map[string]*statsStream
}

type statsStream struct {
	cancel context.CancelFunc

	mu      sync.Mutex
	latest  ContainerStats
	hasData bool
}

func (s *statsStream) store(stats ContainerStats) {
	s.mu.Lock()
	s.latest = stats
	s.hasData = true
	s.mu.Unlock()
}

func (s *statsStream) read() (ContainerStats, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latest, s.hasData
}

func NewStatsStreamer(client *http.Client) *StatsStreamer {
	return &StatsStreamer{
		client:  client,
		streams: make(map[string]*statsStream),
	}
}

// Snapshot reconciles the set of live streams against the currently running
// containers, then returns the latest cached stats for those that have produced
// at least one frame. A container that just started (no frame yet) is simply
// omitted until its first frame arrives, matching the prior behaviour where a
// failed stats fetch dropped the container from the result.
func (st *StatsStreamer) Snapshot(running []Container) []ContainerStats {
	st.reconcile(running)

	st.mu.Lock()
	streamsByID := make(map[string]*statsStream, len(st.streams))
	for id, s := range st.streams {
		streamsByID[id] = s
	}
	st.mu.Unlock()

	result := make([]ContainerStats, 0, len(running))
	for _, c := range running {
		s, ok := streamsByID[c.ID]
		if !ok {
			continue
		}
		stats, hasData := s.read()
		if !hasData {
			continue
		}
		// Names can change; the list endpoint is authoritative for them.
		stats.ID = c.ID
		if c.Name != "" {
			stats.Name = c.Name
		}
		result = append(result, stats)
	}
	return result
}

func (st *StatsStreamer) reconcile(running []Container) {
	wanted := make(map[string]bool, len(running))
	for _, c := range running {
		wanted[c.ID] = true
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	for id := range st.streams {
		if !wanted[id] {
			st.streams[id].cancel()
			delete(st.streams, id)
		}
	}

	for _, c := range running {
		if !isValidContainerID(c.ID) {
			continue
		}
		if _, ok := st.streams[c.ID]; ok {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		s := &statsStream{cancel: cancel}
		st.streams[c.ID] = s
		go st.run(ctx, c.ID, s)
	}
}

// Close tears down every active stream. Safe to call on shutdown.
func (st *StatsStreamer) Close() {
	st.mu.Lock()
	defer st.mu.Unlock()
	for id, s := range st.streams {
		s.cancel()
		delete(st.streams, id)
	}
}

// run holds one streaming /stats connection open and decodes frames as Docker
// pushes them, storing the latest into the shared cache. It returns when ctx is
// cancelled (container stopped or shutdown) or the connection drops; in the
// drop case the next reconcile re-opens it.
func (st *StatsStreamer) run(ctx context.Context, containerID string, s *statsStream) {
	url := fmt.Sprintf("http://docker/%s/containers/%s/stats?stream=true", dockerAPIVersion, containerID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		log.Printf("docker stats stream %s: build request: %v", containerID, err)
		return
	}

	resp, err := st.client.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("docker stats stream %s: %v", containerID, err)
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("docker stats stream %s: unexpected status %d", containerID, resp.StatusCode)
		return
	}

	decoder := json.NewDecoder(resp.Body)
	for {
		if ctx.Err() != nil {
			return
		}
		var raw rawContainerStats
		if err := decoder.Decode(&raw); err != nil {
			if ctx.Err() == nil && err != io.EOF {
				log.Printf("docker stats stream %s: decode: %v", containerID, err)
			}
			return
		}
		s.store(raw.toContainerStats(containerID))
	}
}
