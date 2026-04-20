package collectors

import (
	"bufio"
	"fmt"
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
		return nil, fmt.Errorf("fail2ban-client status: %w", err)
	}

	jailNames := parseJailList(string(out))
	status := &Fail2BanStatus{
		TotalJails: len(jailNames),
	}

	for _, name := range jailNames {
		jail, err := f.getJailStatus(name)
		if err != nil {
			continue
		}
		status.Jails = append(status.Jails, *jail)
		status.TotalBans += jail.BanCount
	}

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

var banLogPattern = regexp.MustCompile(`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2},\d+)\s+fail2ban\.\w+\s+\[\d+\]:\s+\w+\s+\[(\w+)\]\s+(Ban|Unban)\s+(\S+)`)

func (f *Fail2BanCollector) GetRecentBans(limit int) ([]BanEvent, error) {
	logFile := filepath.Join(f.logPath, "fail2ban.log")
	file, err := os.Open(logFile)
	if err != nil {
		return nil, fmt.Errorf("open fail2ban log: %w", err)
	}
	defer file.Close()

	var events []BanEvent
	scanner := bufio.NewScanner(file)

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
