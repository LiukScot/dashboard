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
}

const SchemaVersion = 1
