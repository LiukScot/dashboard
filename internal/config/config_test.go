package config

import (
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Parallel()
	cfg := Load()
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host=%q, want 0.0.0.0", cfg.Host)
	}
	if cfg.Port != 4200 {
		t.Errorf("Port=%d, want 4200", cfg.Port)
	}
	if !cfg.CookieSecure {
		t.Error("CookieSecure should default to true")
	}
	if cfg.SessionTTL != 60*60*24*30 {
		t.Errorf("SessionTTL=%d, want %d", cfg.SessionTTL, 60*60*24*30)
	}
	if cfg.MetricsInterval != 60 {
		t.Errorf("MetricsInterval=%d, want 60", cfg.MetricsInterval)
	}
	if len(cfg.CronPaths) == 0 {
		t.Error("CronPaths should have defaults")
	}
}

func TestLoadEnvOverride(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("COOKIE_SECURE", "false")
	t.Setenv("SESSION_TTL", "3600")

	cfg := Load()
	if cfg.Port != 9999 {
		t.Errorf("Port=%d, want 9999", cfg.Port)
	}
	if cfg.CookieSecure {
		t.Error("CookieSecure should be false when COOKIE_SECURE=false")
	}
	if cfg.SessionTTL != 3600 {
		t.Errorf("SessionTTL=%d, want 3600", cfg.SessionTTL)
	}
}

func TestLoadCronPathsSplit(t *testing.T) {
	t.Setenv("CRON_PATHS", "/etc/crontab,/etc/cron.d/*")

	cfg := Load()
	if len(cfg.CronPaths) != 2 {
		t.Errorf("CronPaths len=%d, want 2: %v", len(cfg.CronPaths), cfg.CronPaths)
	}
	if cfg.CronPaths[0] != "/etc/crontab" {
		t.Errorf("CronPaths[0]=%q, want /etc/crontab", cfg.CronPaths[0])
	}
}
