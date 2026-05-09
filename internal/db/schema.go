package db

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS app_meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS users (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		email         TEXT    NOT NULL UNIQUE,
		password_hash TEXT    NOT NULL,
		created_at    TEXT    NOT NULL DEFAULT (datetime('now'))
	)`,
	`CREATE TABLE IF NOT EXISTS sessions (
		id         TEXT PRIMARY KEY,
		user_id    INTEGER NOT NULL REFERENCES users(id),
		expires_at TEXT    NOT NULL,
		created_at TEXT    NOT NULL DEFAULT (datetime('now'))
	)`,
	`CREATE TABLE IF NOT EXISTS cron_jobs (
		fingerprint TEXT PRIMARY KEY,
		source      TEXT NOT NULL,
		line        INTEGER NOT NULL,
		schedule    TEXT NOT NULL,
		user        TEXT NOT NULL,
		command     TEXT NOT NULL,
		enabled     INTEGER NOT NULL DEFAULT 1,
		updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
	)`,
	`CREATE TABLE IF NOT EXISTS cron_run_history (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id      TEXT NOT NULL REFERENCES cron_jobs(fingerprint) ON DELETE CASCADE,
		scheduled_at TEXT NOT NULL,
		observed_at  TEXT NOT NULL,
		status      TEXT NOT NULL,
		source      TEXT NOT NULL,
		message     TEXT NOT NULL DEFAULT '',
		UNIQUE(job_id, observed_at, source)
	)`,
	`CREATE TABLE IF NOT EXISTS cron_hidden_jobs (
		job_id    TEXT PRIMARY KEY,
		hidden_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`,
	`CREATE TABLE IF NOT EXISTS metrics_history (
		timestamp    INTEGER NOT NULL,
		resolution   TEXT    NOT NULL,
		cpu_percent  REAL    NOT NULL,
		mem_percent  REAL    NOT NULL,
		disk_percent REAL    NOT NULL,
		swap_percent REAL    NOT NULL,
		net_rx_rate  REAL    NOT NULL,
		net_tx_rate  REAL    NOT NULL,
		load_avg_1   REAL    NOT NULL,
		PRIMARY KEY (timestamp, resolution)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_metrics_history_resolution_ts
		ON metrics_history(resolution, timestamp)`,
}

const SchemaVersion = 4
