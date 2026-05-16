package db

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenCreatesDirectoryAndConfiguresPragmas(t *testing.T) {
	t.Parallel()

	// Nested non-existent dir: Open must create the full path.
	dbPath := filepath.Join(t.TempDir(), "nested", "deeper", "dashboard.sqlite")
	database, err := Open(dbPath)
	require.NoError(t, err, "open should create parent directories")
	t.Cleanup(func() { _ = database.Close() })

	// WAL journal mode persists in the file; reading the pragma confirms
	// the connection-string options took effect.
	var journal string
	require.NoError(t, database.QueryRow("PRAGMA journal_mode").Scan(&journal))
	assert.Equal(t, "wal", strings.ToLower(journal))

	// foreign_keys is per-connection; the only way to verify is to read it
	// back through the same handle.
	var fk int
	require.NoError(t, database.QueryRow("PRAGMA foreign_keys").Scan(&fk))
	assert.Equal(t, 1, fk, "foreign_keys must be ON")
}

func TestOpenSingleWriterPool(t *testing.T) {
	t.Parallel()
	// Open is supposed to cap MaxOpenConns to 1 to avoid "database is
	// locked" with SQLite's single-writer model. Stats expose this.
	dbPath := filepath.Join(t.TempDir(), "dashboard.sqlite")
	database, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	assert.Equal(t, 1, database.Stats().MaxOpenConnections)
}

func TestRunMigrationsIsIdempotent(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "dashboard.sqlite")
	database, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	require.NoError(t, RunMigrations(database))
	// Second run on the same DB must not fail (CREATE IF NOT EXISTS is
	// the contract).
	require.NoError(t, RunMigrations(database), "migrations must be idempotent")
	// And a third for paranoia.
	require.NoError(t, RunMigrations(database))
}

func TestRunMigrationsRecordsSchemaVersion(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "dashboard.sqlite")
	database, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	require.NoError(t, RunMigrations(database))

	var value string
	require.NoError(t, database.QueryRow(
		`SELECT value FROM app_meta WHERE key = 'schema_version'`,
	).Scan(&value))

	gotVersion, err := strconv.Atoi(value)
	require.NoError(t, err, "schema version must be numeric")
	assert.Equal(t, SchemaVersion, gotVersion, "persisted schema_version must match the SchemaVersion constant")
}

func TestRunMigrationsUpdatesSchemaVersionInPlace(t *testing.T) {
	t.Parallel()
	// Simulate an older version recorded in app_meta; RunMigrations must
	// overwrite via the ON CONFLICT DO UPDATE branch.
	dbPath := filepath.Join(t.TempDir(), "dashboard.sqlite")
	database, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	require.NoError(t, RunMigrations(database))
	_, err = database.Exec(
		`UPDATE app_meta SET value = '1' WHERE key = 'schema_version'`,
	)
	require.NoError(t, err)

	require.NoError(t, RunMigrations(database))

	var value string
	require.NoError(t, database.QueryRow(
		`SELECT value FROM app_meta WHERE key = 'schema_version'`,
	).Scan(&value))
	got, err := strconv.Atoi(value)
	require.NoError(t, err)
	assert.Equal(t, SchemaVersion, got)
}

func TestOpenFailsOnUnwritableDirectory(t *testing.T) {
	t.Parallel()
	// /proc is read-only; MkdirAll under it must fail with a wrapped
	// error so the caller can see the cause.
	_, err := Open("/proc/no-write/dashboard.sqlite")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create db dir")
}
