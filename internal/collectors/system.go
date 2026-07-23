package collectors

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type SystemMetrics struct {
	Hostname    string    `json:"hostname"`
	Uptime      float64   `json:"uptime"`
	LoadAvg     [3]float64 `json:"loadAvg"`
	CPUPercent  float64   `json:"cpuPercent"`
	CPUCores    int       `json:"cpuCores"`
	MemTotal    uint64    `json:"memTotal"`
	MemUsed     uint64    `json:"memUsed"`
	MemPercent  float64   `json:"memPercent"`
	SwapTotal   uint64    `json:"swapTotal"`
	SwapUsed    uint64    `json:"swapUsed"`
	DiskTotal   uint64    `json:"diskTotal"`
	DiskUsed    uint64    `json:"diskUsed"`
	DiskPercent float64   `json:"diskPercent"`
	Timestamp   time.Time `json:"timestamp"`
}

type NetworkMetrics struct {
	Interface string  `json:"interface"`
	RxBytes   uint64  `json:"rxBytes"`
	TxBytes   uint64  `json:"txBytes"`
	RxRate    float64 `json:"rxRate"`
	TxRate    float64 `json:"txRate"`
}

const defaultMaxSamples = 120

type SystemCollector struct {
	procPath    string
	mu          sync.RWMutex
	history     []SystemMetrics
	netHistory  map[string]netSample
	maxSamples  int

	prevCPUIdle  uint64
	prevCPUTotal uint64
}

type netSample struct {
	rxBytes   uint64
	txBytes   uint64
	timestamp time.Time
}

func NewSystemCollector(procPath string) *SystemCollector {
	return &SystemCollector{
		procPath:   procPath,
		history:    make([]SystemMetrics, 0, defaultMaxSamples),
		netHistory: make(map[string]netSample),
		maxSamples: defaultMaxSamples,
	}
}

func (c *SystemCollector) Collect() (*SystemMetrics, error) {
	m := &SystemMetrics{Timestamp: time.Now()}

	hostname, _ := os.Hostname()
	m.Hostname = hostname

	if err := c.readUptime(m); err != nil {
		return nil, err
	}
	if err := c.readLoadAvg(m); err != nil {
		return nil, err
	}
	if err := c.readCPU(m); err != nil {
		return nil, err
	}
	if err := c.readMemory(m); err != nil {
		return nil, err
	}
	c.readDisk(m)

	c.mu.Lock()
	c.history = append(c.history, *m)
	if len(c.history) > c.maxSamples {
		c.history = c.history[1:]
	}
	c.mu.Unlock()

	return m, nil
}

func (c *SystemCollector) History() []SystemMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]SystemMetrics, len(c.history))
	copy(result, c.history)
	return result
}

func (c *SystemCollector) CollectNetwork() ([]NetworkMetrics, error) {
	data, err := os.ReadFile(filepath.Join(c.procPath, "net/dev"))
	if err != nil {
		return nil, fmt.Errorf("read net/dev: %w", err)
	}

	now := time.Now()
	var metrics []NetworkMetrics

	lines := strings.Split(string(data), "\n")
	if len(lines) < 3 {
		return metrics, nil
	}
	for _, line := range lines[2:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		iface := strings.TrimSpace(parts[0])
		if iface == "lo" {
			continue
		}

		fields := strings.Fields(parts[1])
		if len(fields) < 10 {
			continue
		}

		rxBytes, _ := strconv.ParseUint(fields[0], 10, 64)
		txBytes, _ := strconv.ParseUint(fields[8], 10, 64)

		nm := NetworkMetrics{
			Interface: iface,
			RxBytes:   rxBytes,
			TxBytes:   txBytes,
		}

		c.mu.Lock()
		if prev, ok := c.netHistory[iface]; ok {
			dt := now.Sub(prev.timestamp).Seconds()
			if dt > 0 && rxBytes >= prev.rxBytes && txBytes >= prev.txBytes {
				nm.RxRate = float64(rxBytes-prev.rxBytes) / dt
				nm.TxRate = float64(txBytes-prev.txBytes) / dt
			}
		}
		c.netHistory[iface] = netSample{rxBytes: rxBytes, txBytes: txBytes, timestamp: now}
		c.mu.Unlock()

		metrics = append(metrics, nm)
	}

	return metrics, nil
}

func (c *SystemCollector) readUptime(m *SystemMetrics) error {
	data, err := os.ReadFile(filepath.Join(c.procPath, "uptime"))
	if err != nil {
		return fmt.Errorf("read uptime: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) >= 1 {
		m.Uptime, _ = strconv.ParseFloat(fields[0], 64)
	}
	return nil
}

func (c *SystemCollector) readLoadAvg(m *SystemMetrics) error {
	data, err := os.ReadFile(filepath.Join(c.procPath, "loadavg"))
	if err != nil {
		return fmt.Errorf("read loadavg: %w", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) >= 3 {
		m.LoadAvg[0], _ = strconv.ParseFloat(fields[0], 64)
		m.LoadAvg[1], _ = strconv.ParseFloat(fields[1], 64)
		m.LoadAvg[2], _ = strconv.ParseFloat(fields[2], 64)
	}
	return nil
}

func (c *SystemCollector) readCPU(m *SystemMetrics) error {
	data, err := os.ReadFile(filepath.Join(c.procPath, "stat"))
	if err != nil {
		return fmt.Errorf("read stat: %w", err)
	}

	var cores int
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)[1:]
			var total, idle uint64
			for i, f := range fields {
				v, _ := strconv.ParseUint(f, 10, 64)
				total += v
				if i == 3 { // idle is the 4th field
					idle = v
				}
			}

			c.mu.Lock()
			if c.prevCPUTotal > 0 && total >= c.prevCPUTotal && idle >= c.prevCPUIdle {
				dTotal := total - c.prevCPUTotal
				dIdle := idle - c.prevCPUIdle
				if dTotal > 0 {
					m.CPUPercent = 100.0 * float64(dTotal-dIdle) / float64(dTotal)
				}
			}
			c.prevCPUIdle = idle
			c.prevCPUTotal = total
			c.mu.Unlock()
		}
		if strings.HasPrefix(line, "cpu") && !strings.HasPrefix(line, "cpu ") {
			cores++
		}
	}
	m.CPUCores = cores
	return nil
}

func (c *SystemCollector) readMemory(m *SystemMetrics) error {
	data, err := os.ReadFile(filepath.Join(c.procPath, "meminfo"))
	if err != nil {
		return fmt.Errorf("read meminfo: %w", err)
	}

	values := make(map[string]uint64)
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, " kB")
		v, _ := strconv.ParseUint(strings.TrimSpace(valStr), 10, 64)
		values[key] = v * 1024 // convert kB to bytes
	}

	m.MemTotal = values["MemTotal"]
	memAvailable := values["MemAvailable"]
	if m.MemTotal >= memAvailable {
		m.MemUsed = m.MemTotal - memAvailable
	}
	if m.MemTotal > 0 {
		m.MemPercent = 100.0 * float64(m.MemUsed) / float64(m.MemTotal)
	}

	m.SwapTotal = values["SwapTotal"]
	swapFree := values["SwapFree"]
	if m.SwapTotal >= swapFree {
		m.SwapUsed = m.SwapTotal - swapFree
	}
	return nil
}

func (c *SystemCollector) readDisk(m *SystemMetrics) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return
	}
	m.DiskTotal = stat.Blocks * uint64(stat.Bsize)
	m.DiskUsed = m.DiskTotal - (stat.Bavail * uint64(stat.Bsize))
	if m.DiskTotal > 0 {
		m.DiskPercent = 100.0 * float64(m.DiskUsed) / float64(m.DiskTotal)
	}
}
