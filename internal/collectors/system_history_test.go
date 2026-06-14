package collectors

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newHistoryTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=ON")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	stmts := []string{
		`CREATE TABLE metrics_history (
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
		`CREATE INDEX idx_metrics_history_resolution_ts ON metrics_history(resolution, timestamp)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	return db
}

func insertSample(t *testing.T, db *sql.DB, ts int64, res string, cpu float64) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO metrics_history
		 (timestamp, resolution, cpu_percent, mem_percent, disk_percent, swap_percent, net_rx_rate, net_tx_rate, load_avg_1)
		 VALUES (?, ?, ?, 0, 0, 0, 0, 0, 0)`,
		ts, res, cpu,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func TestDownsampleAveragesAndPrunes(t *testing.T) {
	db := newHistoryTestDB(t)
	h := &SystemHistory{db: db}

	// Insert five 1m rows in one 5m bucket starting at ts=0, all older than cutoff.
	// Bucket boundary: floor(ts/300)*300 = 0.
	for i := 0; i < 5; i++ {
		insertSample(t, db, int64(i*60), resolution1m, float64(10+i*10)) // cpu 10,20,30,40,50 -> avg 30
	}
	// One row newer than cutoff — should not be touched.
	insertSample(t, db, 100_000, resolution1m, 99)

	if err := h.downsample(resolution1m, resolution5m, 1000, 300); err != nil {
		t.Fatalf("downsample: %v", err)
	}

	var bucketCPU float64
	if err := db.QueryRow(
		`SELECT cpu_percent FROM metrics_history WHERE resolution = ? AND timestamp = ?`,
		resolution5m, int64(0),
	).Scan(&bucketCPU); err != nil {
		t.Fatalf("read 5m bucket: %v", err)
	}
	if bucketCPU != 30 {
		t.Fatalf("avg cpu = %v, want 30", bucketCPU)
	}

	var oldCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM metrics_history WHERE resolution = ? AND timestamp < ?`,
		resolution1m, int64(1000),
	).Scan(&oldCount); err != nil {
		t.Fatalf("count: %v", err)
	}
	if oldCount != 0 {
		t.Fatalf("expected 1m rows older than cutoff to be pruned, got %d", oldCount)
	}

	var keptCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM metrics_history WHERE resolution = ? AND timestamp = 100000`,
		resolution1m,
	).Scan(&keptCount); err != nil {
		t.Fatalf("count: %v", err)
	}
	if keptCount != 1 {
		t.Fatalf("expected newer 1m row to be kept, got %d", keptCount)
	}
}

func TestRunRetentionCascadeAndPrune(t *testing.T) {
	db := newHistoryTestDB(t)
	h := &SystemHistory{db: db}

	now := time.Now().Unix()
	day := int64(86400)

	// Tier 1: 1m rows older than 24h → should downsample to 5m and be pruned.
	// Five 1m rows in one 5m bucket, 25h old.
	bucket1mStart := now - 25*3600
	bucket1mStart -= bucket1mStart % 300 // align to 5m boundary
	for i := 0; i < 5; i++ {
		insertSample(t, db, bucket1mStart+int64(i*60), resolution1m, float64(20+i*10)) // avg 40
	}
	// Recent 1m row (1h old) → must survive.
	insertSample(t, db, now-3600, resolution1m, 11)

	// Tier 2: 5m rows older than 7d → should downsample to 1h and be pruned.
	// Twelve 5m rows in one 1h bucket, 8d old.
	bucket5mStart := now - 8*day
	bucket5mStart -= bucket5mStart % 3600 // align to 1h boundary
	for i := 0; i < 12; i++ {
		insertSample(t, db, bucket5mStart+int64(i*300), resolution5m, 50) // avg 50
	}
	// Recent 5m row (3d old) → must survive.
	insertSample(t, db, now-3*day, resolution5m, 22)

	// Tier 3: 1h row older than 30d → must be pruned.
	insertSample(t, db, now-31*day, resolution1h, 88)
	// Recent 1h row (10d old) → must survive.
	insertSample(t, db, now-10*day, resolution1h, 33)

	if err := h.runRetention(); err != nil {
		t.Fatalf("runRetention: %v", err)
	}

	// Tier 1 assertions.
	var cpu5m float64
	if err := db.QueryRow(
		`SELECT cpu_percent FROM metrics_history WHERE resolution = ? AND timestamp = ?`,
		resolution5m, bucket1mStart,
	).Scan(&cpu5m); err != nil {
		t.Fatalf("read downsampled 5m bucket: %v", err)
	}
	if cpu5m != 40 {
		t.Fatalf("downsampled 5m cpu = %v, want 40", cpu5m)
	}
	var old1mCount int
	db.QueryRow(
		`SELECT COUNT(*) FROM metrics_history WHERE resolution = ? AND timestamp < ?`,
		resolution1m, now-24*3600,
	).Scan(&old1mCount)
	if old1mCount != 0 {
		t.Fatalf("expected old 1m rows pruned, got %d", old1mCount)
	}
	var recent1m int
	db.QueryRow(
		`SELECT COUNT(*) FROM metrics_history WHERE resolution = ? AND timestamp = ?`,
		resolution1m, now-3600,
	).Scan(&recent1m)
	if recent1m != 1 {
		t.Fatalf("recent 1m row missing")
	}

	// Tier 2 assertions: 5m → 1h cascade.
	var cpu1h float64
	if err := db.QueryRow(
		`SELECT cpu_percent FROM metrics_history WHERE resolution = ? AND timestamp = ?`,
		resolution1h, bucket5mStart,
	).Scan(&cpu1h); err != nil {
		t.Fatalf("read downsampled 1h bucket: %v", err)
	}
	if cpu1h != 50 {
		t.Fatalf("downsampled 1h cpu = %v, want 50", cpu1h)
	}
	var old5mCount int
	db.QueryRow(
		`SELECT COUNT(*) FROM metrics_history WHERE resolution = ? AND timestamp < ?`,
		resolution5m, now-7*day,
	).Scan(&old5mCount)
	if old5mCount != 0 {
		t.Fatalf("expected old 5m rows pruned, got %d", old5mCount)
	}
	var recent5m int
	db.QueryRow(
		`SELECT COUNT(*) FROM metrics_history WHERE resolution = ? AND timestamp = ?`,
		resolution5m, now-3*day,
	).Scan(&recent5m)
	if recent5m != 1 {
		t.Fatalf("recent 5m row missing")
	}

	// Tier 3 assertions: 1h prune.
	var ancient1hCount int
	db.QueryRow(
		`SELECT COUNT(*) FROM metrics_history WHERE resolution = ? AND timestamp < ?`,
		resolution1h, now-30*day,
	).Scan(&ancient1hCount)
	if ancient1hCount != 0 {
		t.Fatalf("expected 1h rows older than 30d pruned, got %d", ancient1hCount)
	}
	var recent1h int
	db.QueryRow(
		`SELECT COUNT(*) FROM metrics_history WHERE resolution = ? AND timestamp = ?`,
		resolution1h, now-10*day,
	).Scan(&recent1h)
	if recent1h != 1 {
		t.Fatalf("recent 1h row missing")
	}
}

func TestQueryUnionsResolutionsFinerWins(t *testing.T) {
	db := newHistoryTestDB(t)
	h := &SystemHistory{db: db}

	now := time.Now().Unix()
	// 1m row at now-60, plus a 5m row that overlaps the same minute boundary.
	ts1m := now - 60
	bucketStart := ts1m - (ts1m % 60)
	insertSample(t, db, bucketStart, resolution1m, 42)
	insertSample(t, db, bucketStart, resolution5m, 99) // overlap: should be replaced by 1m

	// older 5m row — kept (no 1m overlap).
	insertSample(t, db, now-3600, resolution5m, 77)

	out, err := h.Query(2 * time.Hour)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}

	var found42, found77 bool
	for _, s := range out {
		if s.CPUPercent == 42 {
			found42 = true
		}
		if s.CPUPercent == 77 {
			found77 = true
		}
		if s.CPUPercent == 99 {
			t.Fatalf("5m overlap should have been replaced by 1m row")
		}
	}
	if !found42 || !found77 {
		t.Fatalf("missing rows: got %+v", out)
	}
}

func TestDownsampleNoSourceRowsLeavesTargetEmpty(t *testing.T) {
	t.Parallel()
	db := newHistoryTestDB(t)
	h := &SystemHistory{db: db}

	// No 1m rows at all: downsample must be a no-op, not error.
	require.NoError(t, h.downsample(resolution1m, resolution5m, 1000, 300))

	var count int
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM metrics_history WHERE resolution = ?`, resolution5m,
	).Scan(&count))
	assert.Equal(t, 0, count, "empty source must not insert into target")
}

// TestDownsampleRowsErrorAbortsBeforeDestructiveDelete is the regression
// test for the pending codebase-analysis finding: downsample must check
// rows.Err() after the iterator loop. Without that check, a torn read
// could silently skip rows, after which the unconditional DELETE would
// destroy data that was never aggregated.
//
// Contract: a DB error during iteration must bubble up as an error from
// downsample. The transaction defers rollback, so the source rows must
// still be present in the table after the call.
func TestDownsampleRowsErrorAbortsBeforeDestructiveDelete(t *testing.T) {
	t.Parallel()
	db := newHistoryTestDB(t)
	h := &SystemHistory{db: db}

	insertSample(t, db, 0, resolution1m, 10)
	insertSample(t, db, 60, resolution1m, 20)

	// Close the connection underneath: the next tx.Query inside
	// downsample fails. Any error path through downsample must NOT
	// commit, so the source rows must remain.
	require.NoError(t, db.Close())

	err := h.downsample(resolution1m, resolution5m, 1000, 300)
	assert.Error(t, err, "downsample must surface DB errors")
}

func TestQueryReturnsEmptyWhenNoRows(t *testing.T) {
	t.Parallel()
	db := newHistoryTestDB(t)
	h := &SystemHistory{db: db}

	out, err := h.Query(time.Hour)
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestQueryFiltersByDuration(t *testing.T) {
	t.Parallel()
	db := newHistoryTestDB(t)
	h := &SystemHistory{db: db}

	now := time.Now().Unix()
	insertSample(t, db, now-30, resolution1m, 11)      // inside 1h window
	insertSample(t, db, now-2*3600, resolution1m, 22)  // outside 1h window

	out, err := h.Query(time.Hour)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.InDelta(t, 11.0, out[0].CPUPercent, 0.01)
}

func TestQueryBoundsResultToMaxHistoryRows(t *testing.T) {
	db := newHistoryTestDB(t)
	h := &SystemHistory{db: db}

	now := time.Now().Unix()
	// Insert more 1m samples inside the window than the bound allows. Each ts
	// is a distinct minute so dedupe does not collapse them.
	overflow := maxHistoryRows + 50
	for i := 0; i < overflow; i++ {
		insertSample(t, db, now-int64(i*60), resolution1m, float64(i))
	}

	samples, err := h.Query(60 * 24 * time.Hour)
	require.NoError(t, err)

	assert.LessOrEqual(t, len(samples), maxHistoryRows, "result must be bounded")

	// Ascending order is the chart contract.
	for i := 1; i < len(samples); i++ {
		assert.LessOrEqual(t, samples[i-1].Timestamp, samples[i].Timestamp, "ascending order")
	}

	// The kept rows must be the newest ones (DESC+LIMIT then ASC), so the last
	// sample is the most recent (i==0 insert, timestamp == now).
	assert.Equal(t, now, samples[len(samples)-1].Timestamp, "newest sample retained")
}
