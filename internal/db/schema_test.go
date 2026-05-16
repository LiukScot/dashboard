package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// schemaIndexes returns the names of every non-implicit index that SQLite
// reports for the migrated schema. Implicit indexes (the kind SQLite creates
// for UNIQUE / PRIMARY KEY) start with "sqlite_autoindex_" and are filtered
// out so the assertions target deliberately created ones.
func schemaIndexes(t *testing.T) map[string]bool {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "dashboard.sqlite")
	database, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })

	require.NoError(t, RunMigrations(database))

	rows, err := database.Query(
		`SELECT name FROM sqlite_master WHERE type = 'index' AND name NOT LIKE 'sqlite_autoindex_%'`,
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = rows.Close() })

	got := map[string]bool{}
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		got[name] = true
	}
	require.NoError(t, rows.Err())
	return got
}

func TestMigrationsCreateCoreTables(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "dashboard.sqlite")
	database, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	require.NoError(t, RunMigrations(database))

	rows, err := database.Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
	require.NoError(t, err)
	defer rows.Close()

	got := map[string]bool{}
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		got[name] = true
	}
	require.NoError(t, rows.Err())

	for _, want := range []string{
		"app_meta", "users", "sessions",
		"cron_jobs", "cron_run_history", "cron_hidden_jobs",
		"metrics_history",
	} {
		assert.Truef(t, got[want], "table %q must exist after migrations", want)
	}
}

func TestMigrationsCreateScheduledAtIndex(t *testing.T) {
	t.Parallel()
	indexes := schemaIndexes(t)
	// Range-scan index added by the 2026-05-09 codebase-analysis fix.
	assert.True(t, indexes["idx_cron_run_history_scheduled_at"],
		"idx_cron_run_history_scheduled_at must exist for /api/v1/cron/week range scan")
}

func TestMigrationsCreateMetricsHistoryIndex(t *testing.T) {
	t.Parallel()
	indexes := schemaIndexes(t)
	assert.True(t, indexes["idx_metrics_history_resolution_ts"],
		"idx_metrics_history_resolution_ts must exist for Query() time-window lookups")
}

// TestMigrationsCreateSessionsUserIDIndex is the regression test for the
// pending codebase-analysis finding: sessions.user_id is an FK with no
// index, so cascading delete / per-user session lookups full-scan.
func TestMigrationsCreateSessionsUserIDIndex(t *testing.T) {
	t.Parallel()
	indexes := schemaIndexes(t)
	assert.True(t, indexes["idx_sessions_user_id"],
		"idx_sessions_user_id must exist (sessions.user_id is an FK without index)")
}

// TestMigrationsCreateCronRunHistoryJobIDIndex is the regression test for
// the pending codebase-analysis finding: cron_run_history.job_id is an FK
// with ON DELETE CASCADE but no index, so cascade delete full-scans.
func TestMigrationsCreateCronRunHistoryJobIDIndex(t *testing.T) {
	t.Parallel()
	indexes := schemaIndexes(t)
	assert.True(t, indexes["idx_cron_run_history_job_id"],
		"idx_cron_run_history_job_id must exist (FK without index makes ON DELETE CASCADE full-scan)")
}
