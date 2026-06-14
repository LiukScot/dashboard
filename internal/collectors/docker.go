package collectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const dockerAPIVersion = "v1.43"

type Container struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Image   string            `json:"image"`
	State   string            `json:"state"`
	Status  string            `json:"status"`
	Created int64             `json:"created"`
	Ports   []ContainerPort   `json:"ports"`
	Labels  map[string]string `json:"labels,omitempty"`
}

type ContainerPort struct {
	IP          string `json:"ip,omitempty"`
	PrivatePort uint16 `json:"privatePort"`
	PublicPort  uint16 `json:"publicPort,omitempty"`
	Type        string `json:"type"`
}

type ContainerStats struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	CPUPercent float64 `json:"cpuPercent"`
	MemUsage   uint64  `json:"memUsage"`
	MemLimit   uint64  `json:"memLimit"`
	MemPercent float64 `json:"memPercent"`
	NetRx      uint64  `json:"netRx"`
	NetTx      uint64  `json:"netTx"`
}

type DockerCollector struct {
	client   *http.Client
	streamer *StatsStreamer
}

func NewDockerCollector(socketPath string) *DockerCollector {
	// The list/one-shot client keeps a request timeout. The streamer needs a
	// client WITHOUT a timeout: a streaming /stats connection stays open
	// indefinitely, so a per-request deadline would kill it. Both dial the
	// same unix socket.
	dial := func(ctx context.Context, _, _ string) (net.Conn, error) {
		return net.Dial("unix", socketPath)
	}
	streamClient := &http.Client{
		Transport: &http.Transport{DialContext: dial},
	}
	return &DockerCollector{
		client: &http.Client{
			Transport: &http.Transport{DialContext: dial},
			Timeout:   10 * time.Second,
		},
		streamer: NewStatsStreamer(streamClient),
	}
}

func (d *DockerCollector) ListContainers() ([]Container, error) {
	resp, err := d.client.Get("http://docker/" + dockerAPIVersion + "/containers/json?all=true")
	if err != nil {
		return nil, fmt.Errorf("docker list: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read containers body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("docker list: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var raw []struct {
		Id      string            `json:"Id"`
		Names   []string          `json:"Names"`
		Image   string            `json:"Image"`
		State   string            `json:"State"`
		Status  string            `json:"Status"`
		Created int64             `json:"Created"`
		Labels  map[string]string `json:"Labels"`
		Ports   []struct {
			IP          string `json:"IP"`
			PrivatePort uint16 `json:"PrivatePort"`
			PublicPort  uint16 `json:"PublicPort"`
			Type        string `json:"Type"`
		} `json:"Ports"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode containers: %w", err)
	}

	containers := make([]Container, len(raw))
	for i, c := range raw {
		name := ""
		if len(c.Names) > 0 {
			name = c.Names[0]
			if len(name) > 0 && name[0] == '/' {
				name = name[1:]
			}
		}

		ports := make([]ContainerPort, len(c.Ports))
		for j, p := range c.Ports {
			ports[j] = ContainerPort{
				IP:          p.IP,
				PrivatePort: p.PrivatePort,
				PublicPort:  p.PublicPort,
				Type:        p.Type,
			}
		}

		id := c.Id
		if len(id) > 12 {
			id = id[:12]
		}

		containers[i] = Container{
			ID:      id,
			Name:    name,
			Image:   c.Image,
			State:   c.State,
			Status:  c.Status,
			Created: c.Created,
			Ports:   ports,
			Labels:  c.Labels,
		}
	}

	return containers, nil
}

func isValidContainerID(id string) bool {
	if len(id) != 12 {
		return false
	}
	for _, c := range id {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	return true
}

// rawContainerStats is the subset of the Docker stats JSON we consume. The
// streaming reader decodes this shape once per pushed frame.
type rawContainerStats struct {
	Name     string `json:"name"`
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     int    `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
}

func (raw rawContainerStats) toContainerStats(containerID string) ContainerStats {
	// Guard uint64 subtraction; counters can drop on container restart or
	// stats reset, and wrap-around would surface as nonsense CPU%.
	var cpuDelta, systemDelta float64
	if raw.CPUStats.CPUUsage.TotalUsage >= raw.PreCPUStats.CPUUsage.TotalUsage {
		cpuDelta = float64(raw.CPUStats.CPUUsage.TotalUsage - raw.PreCPUStats.CPUUsage.TotalUsage)
	}
	if raw.CPUStats.SystemCPUUsage >= raw.PreCPUStats.SystemCPUUsage {
		systemDelta = float64(raw.CPUStats.SystemCPUUsage - raw.PreCPUStats.SystemCPUUsage)
	}
	cpuPercent := 0.0
	if systemDelta > 0 && cpuDelta > 0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(raw.CPUStats.OnlineCPUs) * 100.0
	}

	var netRx, netTx uint64
	for _, n := range raw.Networks {
		netRx += n.RxBytes
		netTx += n.TxBytes
	}

	name := raw.Name
	if len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}

	memPercent := 0.0
	if raw.MemoryStats.Limit > 0 {
		memPercent = 100.0 * float64(raw.MemoryStats.Usage) / float64(raw.MemoryStats.Limit)
	}

	return ContainerStats{
		ID:         containerID,
		Name:       name,
		CPUPercent: cpuPercent,
		MemUsage:   raw.MemoryStats.Usage,
		MemLimit:   raw.MemoryStats.Limit,
		MemPercent: memPercent,
		NetRx:      netRx,
		NetTx:      netTx,
	}
}

// GetAllStats lists running containers and returns their latest stats from the
// streaming cache. The first call for a freshly seen container opens its stream
// and may return it without stats until the first frame arrives (~1s); steady
// state reads are served entirely from memory, with no per-container HTTP call
// per tick.
func (d *DockerCollector) GetAllStats() ([]ContainerStats, error) {
	containers, err := d.ListContainers()
	if err != nil {
		return nil, err
	}

	running := make([]Container, 0, len(containers))
	for _, c := range containers {
		if c.State != "running" {
			continue
		}
		running = append(running, c)
	}

	return d.streamer.Snapshot(running), nil
}

// Close stops all background stats streams. Call on shutdown.
func (d *DockerCollector) Close() {
	d.streamer.Close()
}
