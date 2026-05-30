package collectors

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetRecentBansParsesCommaMilliseconds(t *testing.T) {
	t.Parallel()

	logDir := t.TempDir()
	logFile := filepath.Join(logDir, "fail2ban.log")
	line := "2026-04-20 12:34:56,789 fail2ban.actions [1234]: NOTICE [sshd] Ban 1.2.3.4\n"
	if err := os.WriteFile(logFile, []byte(line), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	collector := NewFail2BanCollector(logDir)
	events, err := collector.GetRecentBans(10)
	if err != nil {
		t.Fatalf("GetRecentBans: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 ban event, got %d", len(events))
	}

	want := time.Date(2026, 4, 20, 12, 34, 56, 789000000, time.UTC)
	if !events[0].Timestamp.Equal(want) {
		t.Fatalf("expected timestamp %s, got %s", want, events[0].Timestamp)
	}

	if events[0].Jail != "sshd" {
		t.Fatalf("expected jail sshd, got %q", events[0].Jail)
	}
	if events[0].Action != "ban" {
		t.Fatalf("expected action ban, got %q", events[0].Action)
	}
	if events[0].IP != "1.2.3.4" {
		t.Fatalf("expected IP 1.2.3.4, got %q", events[0].IP)
	}
}

func TestGetRecentBansParsesHyphenatedJailNames(t *testing.T) {
	t.Parallel()

	logDir := t.TempDir()
	logFile := filepath.Join(logDir, "fail2ban.log")
	line := "2026-04-20 12:34:56,789 fail2ban.actions [1234]: NOTICE [sshd-ddos] Ban 1.2.3.4\n"
	if err := os.WriteFile(logFile, []byte(line), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	collector := NewFail2BanCollector(logDir)
	events, err := collector.GetRecentBans(10)
	if err != nil {
		t.Fatalf("GetRecentBans: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 ban event, got %d", len(events))
	}

	if events[0].Jail != "sshd-ddos" {
		t.Fatalf("expected jail sshd-ddos, got %q", events[0].Jail)
	}
}

func TestValidJailNameAcceptsNormalNames(t *testing.T) {
	t.Parallel()
	valid := []string{"sshd", "nginx-auth", "apache.auth", "jail_1", "a-b.c_d"}
	for _, name := range valid {
		assert.True(t, validJailName.MatchString(name), "expected %q to be valid", name)
	}
}

func TestValidJailNameRejectsMalformed(t *testing.T) {
	t.Parallel()
	invalid := []string{"--version", "jail name", "jail/name", ""}
	for _, name := range invalid {
		assert.False(t, validJailName.MatchString(name), "expected %q to be invalid", name)
	}
}
