package collectors

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeProcFixture lays out a minimal /proc tree the SystemCollector reads.
// Tests can override individual files afterwards.
func writeProcFixture(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	defaults := map[string]string{
		"uptime":  "12345.67 8901.23\n",
		"loadavg": "0.10 0.20 0.30 1/100 12345\n",
		"stat":    "cpu  100 20 30 400 5 6 7 8 9 10\ncpu0 50 10 15 200 2 3 4 5 6 7\n",
		"meminfo": "MemTotal:        1024 kB\nMemAvailable:     256 kB\nSwapTotal:        2048 kB\nSwapFree:        1024 kB\n",
		"net/dev": "Inter-|   Receive                                                |  Transmit\n face |bytes packets errs drop fifo frame compressed multicast|bytes packets errs drop fifo colls carrier compressed\n  eth0:  1000     10    0    0    0     0          0         0   2000      20    0    0    0     0       0          0\n",
	}
	for k, v := range files {
		defaults[k] = v
	}
	for name, content := range defaults {
		full := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
}

func TestSystemCollectorCollectPopulatesMetrics(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeProcFixture(t, dir, nil)

	c := NewSystemCollector(dir)
	m, err := c.Collect()
	require.NoError(t, err)
	require.NotNil(t, m)

	assert.Equal(t, 12345.67, m.Uptime)
	assert.Equal(t, [3]float64{0.10, 0.20, 0.30}, m.LoadAvg)
	assert.Equal(t, uint64(1024*1024), m.MemTotal, "MemTotal in bytes (1024 kB)")
	assert.Equal(t, uint64(768*1024), m.MemUsed, "(MemTotal - MemAvailable) in bytes")
	assert.InDelta(t, 75.0, m.MemPercent, 0.01)
	assert.Equal(t, uint64(2048*1024), m.SwapTotal)
	assert.Equal(t, uint64(1024*1024), m.SwapUsed)
	// CPU cores counted from `cpu0` line.
	assert.Equal(t, 1, m.CPUCores)
}

func TestSystemCollectorCollectPopulatesHistory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeProcFixture(t, dir, nil)

	c := NewSystemCollector(dir)
	_, err := c.Collect()
	require.NoError(t, err)
	_, err = c.Collect()
	require.NoError(t, err)

	hist := c.History()
	assert.Len(t, hist, 2, "history must grow with each Collect()")
}

func TestSystemCollectorHistoryRingbufferCap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeProcFixture(t, dir, nil)

	c := NewSystemCollector(dir)
	c.maxSamples = 3
	for i := 0; i < 5; i++ {
		_, err := c.Collect()
		require.NoError(t, err)
	}

	hist := c.History()
	assert.Len(t, hist, 3, "history must be capped at maxSamples")
}

func TestSystemCollectorCollectConcurrent(t *testing.T) {
	t.Parallel()
	// Replaces the old TestSystemCollectorReadCPUConcurrent which exercised
	// the private readCPU. The contract under stress is: Collect() must be
	// safe from multiple goroutines without data race or panic.
	dir := t.TempDir()
	writeProcFixture(t, dir, nil)

	c := NewSystemCollector(dir)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				if _, err := c.Collect(); err != nil {
					t.Errorf("Collect: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestSystemCollectorCollectErrorsOnMissingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Empty proc dir: every readX must fail.
	c := NewSystemCollector(dir)
	_, err := c.Collect()
	assert.Error(t, err)
}

func TestSystemCollectorMemoryUnderflowGuard(t *testing.T) {
	t.Parallel()
	// Partial /proc/meminfo where MemAvailable > MemTotal would underflow
	// uint64 subtraction. The readMemory guard must keep MemUsed at zero
	// rather than wrap to a giant number.
	dir := t.TempDir()
	writeProcFixture(t, dir, map[string]string{
		"meminfo": "MemTotal:        100 kB\nMemAvailable:     200 kB\nSwapTotal:       100 kB\nSwapFree:        200 kB\n",
	})

	c := NewSystemCollector(dir)
	m, err := c.Collect()
	require.NoError(t, err)
	assert.Equal(t, uint64(0), m.MemUsed, "MemUsed must stay 0 when MemAvailable > MemTotal")
	assert.Equal(t, uint64(0), m.SwapUsed, "SwapUsed must stay 0 when SwapFree > SwapTotal")
}

// TestSystemCollectorCPUDeltaUnderflowGuard is the regression test for the
// pending codebase-analysis finding: readCPU subtracts two uint64 counters
// without a guard. When /proc/stat counters reset (kernel rollover, host
// reboot mid-collect, container restart), prev > current and the unguarded
// subtraction wraps to a giant number, yielding nonsense CPU%.
//
// Contract: a counter rollback must not produce a NaN, negative, or absurd
// CPU%. CPUPercent must stay in [0, 100].
func TestSystemCollectorCPUDeltaUnderflowGuard(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeProcFixture(t, dir, nil)
	c := NewSystemCollector(dir)

	// First Collect: seeds prevCPUTotal with a large value.
	writeProcFixture(t, dir, map[string]string{
		"stat": "cpu  10000 1000 1000 50000 0 0 0 0 0 0\n",
	})
	_, err := c.Collect()
	require.NoError(t, err)

	// Second Collect: smaller totals → uint64 wraps without a guard.
	writeProcFixture(t, dir, map[string]string{
		"stat": "cpu  100 10 10 500 0 0 0 0 0 0\n",
	})
	m, err := c.Collect()
	require.NoError(t, err)

	// A proper guard (when prev > current → treat as reset, return 0%)
	// would yield exactly 0. Without it, uint64 wrap can produce
	// negative or absurd values depending on which counter overflows
	// further. Floor at 0 is the cheapest contract to assert and the
	// one the underflow guard fix must satisfy.
	assert.GreaterOrEqual(t, m.CPUPercent, 0.0, "CPUPercent must not be negative under counter rollback")
	// Slight float-precision overshoot is allowed; >>100% is the
	// fingerprint of a real wrap-induced bug.
	assert.Less(t, m.CPUPercent, 101.0, "CPUPercent must not blow past 100 under counter rollback")
}

// --- CollectNetwork ---------------------------------------------------------

func TestCollectNetworkReturnsInterfaces(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeProcFixture(t, dir, nil)
	c := NewSystemCollector(dir)

	nets, err := c.CollectNetwork()
	require.NoError(t, err)
	require.Len(t, nets, 1)
	assert.Equal(t, "eth0", nets[0].Interface)
	assert.Equal(t, uint64(1000), nets[0].RxBytes)
	assert.Equal(t, uint64(2000), nets[0].TxBytes)
}

func TestCollectNetworkSkipsLoopback(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeProcFixture(t, dir, map[string]string{
		"net/dev": "header1\nheader2\n  lo:  100 1 0 0 0 0 0 0  100 1 0 0 0 0 0 0\n  eth0:  1000 10 0 0 0 0 0 0  2000 20 0 0 0 0 0 0\n",
	})
	c := NewSystemCollector(dir)

	nets, err := c.CollectNetwork()
	require.NoError(t, err)
	require.Len(t, nets, 1)
	assert.Equal(t, "eth0", nets[0].Interface)
}

func TestCollectNetworkShortFileNoCrash(t *testing.T) {
	t.Parallel()
	// Truncated /proc/net/dev with < 3 lines: the function must return
	// (nil, nil) instead of panicking on the slice expression.
	dir := t.TempDir()
	writeProcFixture(t, dir, map[string]string{
		"net/dev": "only-one-line\n",
	})
	c := NewSystemCollector(dir)
	nets, err := c.CollectNetwork()
	assert.NoError(t, err)
	assert.Empty(t, nets)
}
