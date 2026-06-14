package collectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
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

	// staleAfter drops a cached frame from Snapshot once it is older than this,
	// so a wedged stream surfaces as "no data" instead of frozen numbers.
	staleAfter time.Duration
	// reconnectBackoff is the minimum gap before a dropped/errored stream is
	// re-opened, so a container whose /stats keeps failing doesn't hot-loop.
	reconnectBackoff time.Duration
	// now is overridable in tests to control staleness/backoff timing.
	now func() time.Time

	mu      sync.Mutex
	streams map[string]*statsStream
}

type statsStream struct {
	cancel context.CancelFunc

	mu        sync.Mutex
	latest    ContainerStats
	hasData   bool
	updatedAt time.Time
	// done is set when run() exits (drop, EOF, non-200). A done-but-wanted
	// stream is re-opened by the next reconcile.
	done bool
	// notBefore gates re-opening after an exit, implementing backoff.
	notBefore time.Time
}

func (s *statsStream) store(stats ContainerStats, at time.Time) {
	s.mu.Lock()
	s.latest = stats
	s.hasData = true
	s.updatedAt = at
	s.mu.Unlock()
}

func (s *statsStream) read() (ContainerStats, bool, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latest, s.hasData, s.updatedAt
}

func (s *statsStream) markDone(notBefore time.Time) {
	s.mu.Lock()
	s.done = true
	s.notBefore = notBefore
	s.mu.Unlock()
}

func (s *statsStream) canReopen(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.done && !now.Before(s.notBefore)
}

const (
	defaultStatsStaleAfter       = 15 * time.Second
	defaultStatsReconnectBackoff = 5 * time.Second
)

func NewStatsStreamer(client *http.Client) *StatsStreamer {
	return &StatsStreamer{
		client:           client,
		staleAfter:       defaultStatsStaleAfter,
		reconnectBackoff: defaultStatsReconnectBackoff,
		now:              time.Now,
		streams:          make(map[string]*statsStream),
	}
}

// Snapshot reconciles the set of live streams against the currently running
// containers, then returns the latest cached stats for those that have produced
// at least one frame within staleAfter. A container that just started (no frame
// yet) or whose stream has wedged (frame older than staleAfter) is omitted,
// matching the prior behaviour where a failed stats fetch dropped the container
// from the result.
func (st *StatsStreamer) Snapshot(running []Container) []ContainerStats {
	st.reconcile(running)

	now := st.now()

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
		stats, hasData, updatedAt := s.read()
		if !hasData || now.Sub(updatedAt) > st.staleAfter {
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

	now := st.now()

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
		if existing, ok := st.streams[c.ID]; ok {
			// Still-running container whose stream died: re-open it once its
			// backoff window has elapsed. A live stream is left alone.
			if !existing.canReopen(now) {
				continue
			}
			existing.cancel()
			delete(st.streams, c.ID)
		}
		st.startStream(c.ID)
	}
}

// startStream creates a stream entry and launches its reader. Caller holds st.mu.
func (st *StatsStreamer) startStream(containerID string) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &statsStream{cancel: cancel}
	st.streams[containerID] = s
	go st.run(ctx, containerID, s)
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
// cancelled (container stopped or shutdown) or the connection drops/errors. On
// any non-cancelled exit it marks the stream done with a backoff deadline, so
// the next reconcile re-opens it for a still-running container without
// hot-looping on a persistently failing endpoint.
func (st *StatsStreamer) run(ctx context.Context, containerID string, s *statsStream) {
	defer func() {
		if ctx.Err() == nil {
			s.markDone(st.now().Add(st.reconnectBackoff))
		}
	}()

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
		// Drain so the transport can reuse the connection.
		_, _ = io.Copy(io.Discard, resp.Body)
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
		s.store(raw.toContainerStats(containerID), st.now())
	}
}
