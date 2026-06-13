package collectors

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Fail2BanStatus struct {
	Jails      []JailStatus `json:"jails"`
	TotalBans  int          `json:"totalBans"`
	TotalJails int          `json:"totalJails"`
}

type JailStatus struct {
	Name       string   `json:"name"`
	BannedIPs  []string `json:"bannedIPs"`
	BanCount   int      `json:"banCount"`
	TotalBans  int      `json:"totalBans"`
	TotalFails int      `json:"totalFails"`
}

type BanEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Jail      string    `json:"jail"`
	IP        string    `json:"ip"`
	Action    string    `json:"action"`
}

type Fail2BanCollector struct {
	logPath string
}

func NewFail2BanCollector(logPath string) *Fail2BanCollector {
	return &Fail2BanCollector{logPath: logPath}
}

func (f *Fail2BanCollector) GetStatus() (*Fail2BanStatus, error) {
	out, err := exec.Command("fail2ban-client", "status").Output()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("fail2ban-client status: %w", err)
	}

	jailNames := parseJailList(string(out))
	status := &Fail2BanStatus{}

	for _, name := range jailNames {
		if !validJailName.MatchString(name) {
			continue
		}
		jail, err := f.getJailStatus(name)
		if err != nil {
			log.Printf("fail2ban: get jail %q status: %v", name, err)
			continue
		}
		status.Jails = append(status.Jails, *jail)
		status.TotalBans += jail.BanCount
	}
	status.TotalJails = len(status.Jails)

	return status, nil
}

func (f *Fail2BanCollector) getJailStatus(name string) (*JailStatus, error) {
	out, err := exec.Command("fail2ban-client", "status", name).Output()
	if err != nil {
		return nil, fmt.Errorf("fail2ban-client status %s: %w", name, err)
	}

	jail := &JailStatus{Name: name}
	lines := strings.Split(string(out), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Currently banned:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &jail.BanCount)
			}
		}
		if strings.Contains(line, "Total banned:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &jail.TotalBans)
			}
		}
		if strings.Contains(line, "Total failed:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &jail.TotalFails)
			}
		}
		if strings.Contains(line, "Banned IP list:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				ips := strings.TrimSpace(parts[1])
				if ips != "" {
					for _, ip := range strings.Fields(ips) {
						jail.BannedIPs = append(jail.BannedIPs, ip)
					}
				}
			}
		}
	}

	return jail, nil
}

var banLogPattern = regexp.MustCompile(`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2},\d+)\s+fail2ban\.\w+\s+\[\d+\]:\s+\w+\s+\[([\w.-]+)\]\s+(Ban|Unban)\s+(\S+)`)

// validJailName requires names to start with alphanumeric to prevent leading
// dashes being interpreted as flags by fail2ban-client (argument injection).
var validJailName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func (f *Fail2BanCollector) GetRecentBans(limit int) ([]BanEvent, error) {
	logFile := filepath.Join(f.logPath, "fail2ban.log")
	file, err := os.Open(logFile)
	if err != nil {
		if os.IsNotExist(err) {
			return []BanEvent{}, nil
		}
		return nil, fmt.Errorf("open fail2ban log: %w", err)
	}
	defer file.Close()

	var events []BanEvent
	scanner := bufio.NewScanner(file)
	// Default 64KiB token cap aborts the whole scan on a single long line;
	// fail2ban traceback dumps can exceed it. Match logs.go.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		matches := banLogPattern.FindStringSubmatch(scanner.Text())
		if matches == nil {
			continue
		}

		// fail2ban uses comma for milliseconds (e.g. "2024-01-15 10:30:45,123")
		// Go's time.Parse uses dot for fractional seconds, so replace comma
		tsStr := strings.Replace(matches[1], ",", ".", 1)
		ts, err := time.Parse("2006-01-02 15:04:05.000", tsStr)
		if err != nil {
			continue
		}

		events = append(events, BanEvent{
			Timestamp: ts,
			Jail:      matches[2],
			Action:    strings.ToLower(matches[3]),
			IP:        matches[4],
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan fail2ban log: %w", err)
	}

	// Return last N events
	if len(events) > limit {
		events = events[len(events)-limit:]
	}

	// Reverse to get newest first
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}

	return events, nil
}

func parseJailList(output string) []string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "Jail list:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				return nil
			}
			var jails []string
			for _, j := range strings.Split(parts[1], ",") {
				j = strings.TrimSpace(j)
				if j != "" {
					jails = append(jails, j)
				}
			}
			return jails
		}
	}
	return nil
}
