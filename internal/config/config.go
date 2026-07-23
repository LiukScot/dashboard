package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Host           string
	Port           int
	DBPath         string
	ProcPath       string
	LogPath        string
	DockerSocket   string
	CronPaths      []string
	AllowedOrigins string
	CookieSecure   bool
	SessionTTL      int
	PublicDir       string
	MetricsInterval int
}

func Load() *Config {
	return &Config{
		Host:           envOr("HOST", "0.0.0.0"),
		Port:           envInt("PORT", 4200),
		DBPath:         envOr("DB_PATH", "./data/dashboard.sqlite"),
		ProcPath:       envOr("PROC_PATH", "/proc"),
		LogPath:        envOr("LOG_PATH", "/var/log"),
		DockerSocket:   envOr("DOCKER_SOCKET", "/var/run/docker.sock"),
		CronPaths:      envList("CRON_PATHS", "/etc/crontab,/etc/cron.d/*,/var/spool/cron/*,/var/spool/cron/crontabs/*,/var/spool/cron/tabs/*"),
		AllowedOrigins: envOr("ALLOWED_ORIGINS", "http://localhost:4200,http://127.0.0.1:4200,http://localhost:5173,http://127.0.0.1:5173"),
		CookieSecure:   envOr("COOKIE_SECURE", "true") == "true",
		SessionTTL:     envInt("SESSION_TTL", 60*60*24*30),
		PublicDir:       envOr("PUBLIC_DIR", "./frontend/build"),
		MetricsInterval: envInt("METRICS_INTERVAL", 60),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envList(key string, fallback string) []string {
	raw := envOr(key, fallback)
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
