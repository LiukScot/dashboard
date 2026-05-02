package collectors

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadLogFileReturnsScannerErrors(t *testing.T) {
	t.Parallel()

	logDir := t.TempDir()
	logFile := filepath.Join(logDir, "syslog")
	longLine := "Apr  1 18:30:00 host app[123]: " + strings.Repeat("x", 1024*1024+1) + "\n"
	if err := os.WriteFile(logFile, []byte(longLine), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	_, err := readLogFile(logFile, "syslog", -1)
	if err == nil {
		t.Fatal("expected scanner error for oversized log line")
	}
}
