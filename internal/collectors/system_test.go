package collectors

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestSystemCollectorReadCPUConcurrent(t *testing.T) {
	procDir := t.TempDir()
	stat := "cpu  100 20 30 400 5 6 7 8 9 10\ncpu0 50 10 15 200 2 3 4 5 6 7\n"
	if err := os.WriteFile(filepath.Join(procDir, "stat"), []byte(stat), 0o644); err != nil {
		t.Fatalf("write stat: %v", err)
	}

	collector := NewSystemCollector(procDir)

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				var metrics SystemMetrics
				if err := collector.readCPU(&metrics); err != nil {
					t.Errorf("readCPU: %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()
}
