package collectors

import (
	"bufio"
	"encoding/json"
	"fmt"
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
		fileEntries, err := readLogFile(fullPath, lf.unitName, priority)
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

func readLogFile(path string, unitName string, priorityFilter int) ([]LogEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Read from end — get last 500 lines max
	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	// Take last 500 lines
	if len(lines) > 500 {
		lines = lines[len(lines)-500:]
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
			t = t.AddDate(time.Now().Year(), 0, 0)
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
			ts = strconv.FormatInt(time.Now().UnixMicro(), 10)
			rest = line
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

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		switch s := v.(type) {
		case string:
			return s
		case float64:
			return strconv.FormatFloat(s, 'f', -1, 64)
		default:
			return fmt.Sprintf("%v", s)
		}
	}
	return ""
}

// ParseJournalJSON parses JSON output from journalctl (for future use when journalctl is available)
func ParseJournalJSON(data []byte) []LogEntry {
	var entries []LogEntry
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		pri := 6
		if p, ok := raw["PRIORITY"]; ok {
			switch v := p.(type) {
			case string:
				pri, _ = strconv.Atoi(v)
			case float64:
				pri = int(v)
			}
		}

		entries = append(entries, LogEntry{
			Timestamp:     getString(raw, "__REALTIME_TIMESTAMP"),
			Unit:          getString(raw, "_SYSTEMD_UNIT"),
			Message:       getString(raw, "MESSAGE"),
			Priority:      pri,
			PriorityLabel: priorityLabel(pri),
			Hostname:      getString(raw, "_HOSTNAME"),
			PID:           getString(raw, "_PID"),
		})
	}
	return entries
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

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
