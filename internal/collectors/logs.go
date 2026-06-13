package collectors

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type LogEntry struct {
	Timestamp     string `json:"timestamp"`
	Unit          string `json:"unit"`
	Message       string `json:"message"`
	Priority      int    `json:"priority"`
	PriorityLabel string `json:"priorityLabel"`
	Hostname      string `json:"hostname"`
	PID           string `json:"pid"`
}

type LogCollector struct {
	logPath string
}

func NewLogCollector(logPath string) *LogCollector {
	return &LogCollector{logPath: logPath}
}

// GetLogs reads recent syslog-format log files from the mounted /var/log directory.
// Falls back to reading common log files when journalctl is not available (e.g., in containers).
func (l *LogCollector) GetLogs(unit string, priority int, limit int) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 100
	}

	var entries []LogEntry

	// Read from common log files
	logFiles := []struct {
		path     string
		unitName string
	}{
		{"syslog", "syslog"},
		{"messages", "system"},
		{"auth.log", "auth"},
		{"secure", "auth"},
		{"kern.log", "kernel"},
		{"daemon.log", "daemon"},
	}

	for _, lf := range logFiles {
		if unit != "" && !strings.Contains(lf.unitName, unit) {
			continue
		}

		fullPath := filepath.Join(l.logPath, lf.path)
		fileEntries, err := readLogFile(fullPath, lf.unitName, priority, limit)
		if err != nil {
			continue
		}
		entries = append(entries, fileEntries...)
	}

	// Sort by timestamp descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp > entries[j].Timestamp
	})

	if len(entries) > limit {
		entries = entries[:limit]
	}

	return entries, nil
}

func readLogFile(path string, unitName string, priorityFilter int, maxLines int) ([]LogEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	ring := make([]string, maxLines)
	pos, total := 0, 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		ring[pos%maxLines] = scanner.Text()
		pos++
		total++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var lines []string
	if total <= maxLines {
		lines = ring[:total]
	} else {
		start := pos % maxLines
		lines = append(ring[start:], ring[:start]...)
	}

	var entries []LogEntry
	for _, line := range lines {
		entry := parseSyslogLine(line, unitName)
		if entry == nil {
			continue
		}
		if priorityFilter >= 0 && priorityFilter <= 7 && entry.Priority > priorityFilter {
			continue
		}
		entries = append(entries, *entry)
	}

	return entries, nil
}

// parseSyslogLine parses common syslog format: "Mar 31 14:23:01 hostname process[pid]: message"
func parseSyslogLine(line string, unitName string) *LogEntry {
	if len(line) < 16 {
		return nil
	}

	// Parse timestamp (first 15 chars typically)
	// Try RFC3339 first (some logs use it)
	var ts string
	var rest string

	// Standard syslog: "Apr  1 18:30:00 hostname ..."
	if len(line) >= 15 {
		tsStr := line[:15]
		t, err := time.Parse("Jan  2 15:04:05", tsStr)
		if err == nil {
			now := time.Now()
			t = time.Date(now.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, now.Location())
			// Late-Dec lines parsed in early Jan would be tagged with the
			// current year and surface as "future" entries; roll the year
			// back when the parsed timestamp is more than a day ahead.
			if t.After(now.Add(24 * time.Hour)) {
				t = t.AddDate(-1, 0, 0)
			}
			ts = strconv.FormatInt(t.UnixMicro(), 10)
			rest = strings.TrimSpace(line[15:])
		}
	}

	if ts == "" {
		// Try ISO format
		if len(line) > 25 && (line[4] == '-' || line[10] == 'T') {
			ts = line[:25]
			rest = strings.TrimSpace(line[25:])
		} else {
			// Unparseable header — drop the line. Synthesizing time.Now()
			// as a fallback would surface unparseable old log lines as
			// "newest entries" once the desc sort runs.
			return nil
		}
	}

	// Parse "hostname process[pid]: message"
	parts := strings.SplitN(rest, " ", 3)
	hostname := ""
	message := rest
	processInfo := ""

	if len(parts) >= 3 {
		hostname = parts[0]
		processInfo = parts[1]
		message = parts[2]
	} else if len(parts) == 2 {
		hostname = parts[0]
		message = parts[1]
	}

	// Extract PID from process[pid]:
	pid := ""
	if idx := strings.Index(processInfo, "["); idx >= 0 {
		if end := strings.Index(processInfo[idx:], "]"); end >= 0 {
			pid = processInfo[idx+1 : idx+end]
		}
	}

	// Guess priority from message content
	pri := guessPriority(message)

	// Remove trailing colon from process name
	processInfo = strings.TrimSuffix(processInfo, ":")

	return &LogEntry{
		Timestamp:     ts,
		Unit:          unitName,
		Message:       message,
		Priority:      pri,
		PriorityLabel: priorityLabel(pri),
		Hostname:      hostname,
		PID:           pid,
	}
}

func guessPriority(msg string) int {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "emerg"):
		return 0
	case strings.Contains(lower, "alert"):
		return 1
	case strings.Contains(lower, "crit"):
		return 2
	case strings.Contains(lower, "error"), strings.Contains(lower, "fail"):
		return 3
	case strings.Contains(lower, "warn"):
		return 4
	case strings.Contains(lower, "notice"):
		return 5
	default:
		return 6
	}
}

func priorityLabel(p int) string {
	switch p {
	case 0:
		return "emerg"
	case 1:
		return "alert"
	case 2:
		return "crit"
	case 3:
		return "err"
	case 4:
		return "warning"
	case 5:
		return "notice"
	case 6:
		return "info"
	case 7:
		return "debug"
	default:
		return "unknown"
	}
}
