package config

import (
	"os"
	"strconv"
)

type Config struct {
	Host         string
	Port         int
	DBPath       string
	ProcPath     string
	LogPath      string
	DockerSocket string
	AllowedOrigins string
	CookieSecure bool
	SessionTTL   int
	PublicDir    string
}

func Load() *Config {
	return &Config{
		Host:           envOr("HOST", "0.0.0.0"),
		Port:           envInt("PORT", 4200),
		DBPath:         envOr("DB_PATH", "./data/dashboard.sqlite"),
		ProcPath:       envOr("PROC_PATH", "/proc"),
		LogPath:        envOr("LOG_PATH", "/var/log"),
		DockerSocket:   envOr("DOCKER_SOCKET", "/var/run/docker.sock"),
		AllowedOrigins: envOr("ALLOWED_ORIGINS", "http://localhost:4200,http://127.0.0.1:4200,http://localhost:5173,http://127.0.0.1:5173"),
		CookieSecure:   envOr("COOKIE_SECURE", "false") == "true",
		SessionTTL:     envInt("SESSION_TTL", 60*60*24*30),
		PublicDir:      envOr("PUBLIC_DIR", "./frontend/build"),
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
