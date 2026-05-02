package collectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

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
	client *http.Client
}

const maxConcurrentStatsRequests = 4

func NewDockerCollector(socketPath string) *DockerCollector {
	return &DockerCollector{
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
			Timeout: 10 * time.Second,
		},
	}
}

func (d *DockerCollector) ListContainers() ([]Container, error) {
	resp, err := d.client.Get("http://docker/v1.43/containers/json?all=true")
	if err != nil {
		return nil, fmt.Errorf("docker list: %w", err)
	}
	defer resp.Body.Close()

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

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
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

func (d *DockerCollector) GetContainerStats(containerID string) (*ContainerStats, error) {
	resp, err := d.client.Get(fmt.Sprintf("http://docker/v1.43/containers/%s/stats?stream=false", containerID))
	if err != nil {
		return nil, fmt.Errorf("docker stats %s: %w", containerID, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read stats body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("docker stats %s: unexpected status %d", containerID, resp.StatusCode)
	}

	var raw struct {
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

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode stats: %w", err)
	}

	cpuDelta := float64(raw.CPUStats.CPUUsage.TotalUsage - raw.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(raw.CPUStats.SystemCPUUsage - raw.PreCPUStats.SystemCPUUsage)
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

	return &ContainerStats{
		ID:         containerID,
		Name:       name,
		CPUPercent: cpuPercent,
		MemUsage:   raw.MemoryStats.Usage,
		MemLimit:   raw.MemoryStats.Limit,
		MemPercent: memPercent,
		NetRx:      netRx,
		NetTx:      netTx,
	}, nil
}

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

	stats := make([]ContainerStats, len(running))
	filled := make([]bool, len(running))
	sem := make(chan struct{}, maxConcurrentStatsRequests)

	var mu sync.Mutex
	var wg sync.WaitGroup
	for i, c := range running {
		wg.Add(1)
		go func(i int, c Container) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			s, err := d.GetContainerStats(c.ID)
			if err != nil {
				return
			}

			mu.Lock()
			stats[i] = *s
			filled[i] = true
			mu.Unlock()
		}(i, c)
	}
	wg.Wait()

	result := make([]ContainerStats, 0, len(running))
	for i, ok := range filled {
		if ok {
			result = append(result, stats[i])
		}
	}

	return result, nil
}
