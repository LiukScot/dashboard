package collectors

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseSyslogLineYearRollback(t *testing.T) {
	t.Parallel()
	now := time.Now()
	if now.Month() == time.December {
		t.Skip("rollback test not applicable in December")
	}
	// A December line parsed with the current year is in the future → rollback.
	line := "Dec 31 23:59:59 host sshd[42]: Connection closed"
	entry := parseSyslogLine(line, "auth")
	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	us, err := strconv.ParseInt(entry.Timestamp, 10, 64)
	if err != nil {
		t.Fatalf("bad timestamp: %v", err)
	}
	got := time.UnixMicro(us)
	if got.Year() != now.Year()-1 {
		t.Errorf("expected year %d after rollback, got %d", now.Year()-1, got.Year())
	}
}

// syslogTS returns a valid syslog timestamp "MMM _D HH:MM:SS" (always 15 chars).
// Go's "Jan  2" layout produces an extra space for double-digit days, so we
// build the string manually to match real syslog output.
func syslogTS(t time.Time) string {
	return fmt.Sprintf("%s %2d %02d:%02d:%02d",
		t.Month().String()[:3], t.Day(), t.Hour(), t.Minute(), t.Second())
}

func TestGetLogsUnitFilter(t *testing.T) {
	t.Parallel()
	logDir := t.TempDir()
	ts := syslogTS(time.Now())

	if err := os.WriteFile(filepath.Join(logDir, "syslog"), []byte(ts+" host kernel[1]: boot message\n"), 0o644); err != nil {
		t.Fatalf("write syslog: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "auth.log"), []byte(ts+" host sshd[9]: auth message\n"), 0o644); err != nil {
		t.Fatalf("write auth.log: %v", err)
	}

	coll := NewLogCollector(logDir)
	entries, err := coll.GetLogs("auth", -1, 100)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	for _, e := range entries {
		if e.Unit != "auth" {
			t.Errorf("expected unit=auth, got %q", e.Unit)
		}
	}
}

func TestGetLogsPriorityFilter(t *testing.T) {
	t.Parallel()
	logDir := t.TempDir()
	ts := syslogTS(time.Now())
	content := ts + " host app[1]: error connecting to database\n" +
		ts + " host app[2]: normal startup complete\n"
	if err := os.WriteFile(filepath.Join(logDir, "syslog"), []byte(content), 0o644); err != nil {
		t.Fatalf("write syslog: %v", err)
	}

	coll := NewLogCollector(logDir)
	// priority 3 = err; only the "error" line should pass
	entries, err := coll.GetLogs("", 3, 100)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	for _, e := range entries {
		if e.Priority > 3 {
			t.Errorf("got entry with priority %d > 3: %q", e.Priority, e.Message)
		}
	}
	if len(entries) == 0 {
		t.Error("expected at least one error-level entry")
	}
}

func TestReadLogFileReturnsScannerErrors(t *testing.T) {
	t.Parallel()

	logDir := t.TempDir()
	logFile := filepath.Join(logDir, "syslog")
	longLine := "Apr  1 18:30:00 host app[123]: " + strings.Repeat("x", 1024*1024+1) + "\n"
	if err := os.WriteFile(logFile, []byte(longLine), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	_, err := readLogFile(logFile, "syslog", -1, 500)
	if err == nil {
		t.Fatal("expected scanner error for oversized log line")
	}
}
