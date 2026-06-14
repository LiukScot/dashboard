package collectors

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

type HistorySample struct {
	Timestamp   int64   `json:"timestamp"`
	CPUPercent  float64 `json:"cpuPercent"`
	MemPercent  float64 `json:"memPercent"`
	DiskPercent float64 `json:"diskPercent"`
	SwapPercent float64 `json:"swapPercent"`
	NetRxRate   float64 `json:"netRxRate"`
	NetTxRate   float64 `json:"netTxRate"`
	LoadAvg1    float64 `json:"loadAvg1"`
}

const (
	resolution1m  = "1m"
	resolution5m  = "5m"
	resolution1h  = "1h"
	retain1mHours = 24
	retain5mDays  = 7
	retain1hDays  = 30

	secondsPerHour = 3600
	secondsPerDay  = 86400
	bucket5mSecs   = 5 * 60
	bucket1hSecs   = secondsPerHour

	// maxHistoryRows bounds a single Query() to the number of samples the
	// retention policy can legitimately keep: 1m for 24h, 5m for 7d, 1h for
	// 30d, with headroom for the brief overlap before downsampling runs. It
	// stops a stalled retention loop (or an over-wide range) from loading
	// tens of thousands of rows into memory at once (AGENTS.md §12).
	maxHistoryRows = (retain1mHours * 60) +
		(retain5mDays * 24 * (secondsPerHour / bucket5mSecs)) +
		(retain1hDays * 24) +
		1000
)

type SystemHistory struct {
	db        *sql.DB
	collector *SystemCollector
	interval  time.Duration
}

func NewSystemHistory(db *sql.DB, collector *SystemCollector, interval time.Duration) *SystemHistory {
	return &SystemHistory{db: db, collector: collector, interval: interval}
}

// Run starts the persistence + retention loops. Returns when ctx is cancelled.
func (h *SystemHistory) Run(ctx context.Context) {
	if h.interval <= 0 {
		h.interval = 60 * time.Second
	}
	go h.persistLoop(ctx)
	go h.retentionLoop(ctx)
}

func (h *SystemHistory) persistLoop(ctx context.Context) {
	t := time.NewTicker(h.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := h.snapshot(); err != nil {
				log.Printf("metrics history snapshot: %v", err)
			}
		}
	}
}

func (h *SystemHistory) snapshot() error {
	sys, err := h.collector.Collect()
	if err != nil {
		return fmt.Errorf("collect: %w", err)
	}
	nets, _ := h.collector.CollectNetwork()
	var rx, tx float64
	for _, n := range nets {
		rx += n.RxRate
		tx += n.TxRate
	}

	swapPercent := 0.0
	if sys.SwapTotal > 0 {
		swapPercent = 100.0 * float64(sys.SwapUsed) / float64(sys.SwapTotal)
	}

	_, err = h.db.Exec(
		`INSERT OR REPLACE INTO metrics_history
		 (timestamp, resolution, cpu_percent, mem_percent, disk_percent, swap_percent, net_rx_rate, net_tx_rate, load_avg_1)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sys.Timestamp.Unix(), resolution1m,
		sys.CPUPercent, sys.MemPercent, sys.DiskPercent, swapPercent,
		rx, tx, sys.LoadAvg[0],
	)
	if err != nil {
		return fmt.Errorf("insert: %w", err)
	}
	return nil
}

func (h *SystemHistory) retentionLoop(ctx context.Context) {
	// First pass shortly after start, then hourly.
	first := time.NewTimer(5 * time.Minute)
	defer first.Stop()
	select {
	case <-ctx.Done():
		return
	case <-first.C:
	}
	if err := h.runRetention(); err != nil {
		log.Printf("metrics retention: %v", err)
	}

	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := h.runRetention(); err != nil {
				log.Printf("metrics retention: %v", err)
			}
		}
	}
}

// runRetention downsamples 1m→5m past 24h, 5m→1h past 7d, deletes 1h past 30d.
func (h *SystemHistory) runRetention() error {
	now := time.Now().Unix()
	cutoff1m := now - int64(retain1mHours*secondsPerHour)
	cutoff5m := now - int64(retain5mDays*secondsPerDay)
	cutoff1h := now - int64(retain1hDays*secondsPerDay)

	if err := h.downsample(resolution1m, resolution5m, cutoff1m, bucket5mSecs); err != nil {
		return fmt.Errorf("downsample 1m→5m: %w", err)
	}
	if err := h.downsample(resolution5m, resolution1h, cutoff5m, bucket1hSecs); err != nil {
		return fmt.Errorf("downsample 5m→1h: %w", err)
	}
	if _, err := h.db.Exec(
		`DELETE FROM metrics_history WHERE resolution = ? AND timestamp < ?`,
		resolution1h, cutoff1h,
	); err != nil {
		return fmt.Errorf("prune 1h: %w", err)
	}
	return nil
}

// downsample averages rows in srcRes older than cutoff into bucketSec buckets and
// inserts into dstRes. Source rows older than cutoff are then deleted.
func (h *SystemHistory) downsample(srcRes, dstRes string, cutoff, bucketSec int64) error {
	tx, err := h.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query(
		`SELECT (timestamp / ?) * ? AS bucket,
				AVG(cpu_percent), AVG(mem_percent), AVG(disk_percent), AVG(swap_percent),
				AVG(net_rx_rate), AVG(net_tx_rate), AVG(load_avg_1)
		   FROM metrics_history
		  WHERE resolution = ? AND timestamp < ?
		  GROUP BY bucket`,
		bucketSec, bucketSec, srcRes, cutoff,
	)
	if err != nil {
		return err
	}

	type bucket struct {
		ts                                 int64
		cpu, mem, disk, swap, rx, tx, load float64
	}
	var buckets []bucket
	for rows.Next() {
		var b bucket
		if err := rows.Scan(&b.ts, &b.cpu, &b.mem, &b.disk, &b.swap, &b.rx, &b.tx, &b.load); err != nil {
			rows.Close()
			return err
		}
		buckets = append(buckets, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	stmt, err := tx.Prepare(
		`INSERT OR REPLACE INTO metrics_history
		 (timestamp, resolution, cpu_percent, mem_percent, disk_percent, swap_percent, net_rx_rate, net_tx_rate, load_avg_1)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, b := range buckets {
		if _, err := stmt.Exec(b.ts, dstRes, b.cpu, b.mem, b.disk, b.swap, b.rx, b.tx, b.load); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(
		`DELETE FROM metrics_history WHERE resolution = ? AND timestamp < ?`,
		srcRes, cutoff,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// Query returns history samples for the given duration. Rows from all
// resolutions inside the window are unioned; per-bucket dedupe prefers
// the finest resolution available (1m > 5m > 1h).
func (h *SystemHistory) Query(rangeDur time.Duration) ([]HistorySample, error) {
	since := time.Now().Add(-rangeDur).Unix()

	// Bound the scan to the newest maxHistoryRows so a stalled retention loop
	// can't pull tens of thousands of rows into memory; the inner DESC+LIMIT
	// keeps the most recent samples, the outer ASC restores chart order.
	rows, err := h.db.Query(
		`SELECT timestamp, resolution, cpu_percent, mem_percent, disk_percent,
				swap_percent, net_rx_rate, net_tx_rate, load_avg_1
		   FROM (
				SELECT timestamp, resolution, cpu_percent, mem_percent, disk_percent,
					   swap_percent, net_rx_rate, net_tx_rate, load_avg_1
				  FROM metrics_history
				 WHERE timestamp >= ?
				 ORDER BY timestamp DESC
				 LIMIT ?
		   )
		  ORDER BY timestamp ASC`,
		since, maxHistoryRows,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// resPriority: lower = finer
	resPriority := map[string]int{resolution1m: 0, resolution5m: 1, resolution1h: 2}
	type seen struct {
		idx  int
		prio int
	}
	bucket := make(map[int64]seen)
	var out []HistorySample

	for rows.Next() {
		var s HistorySample
		var res string
		if err := rows.Scan(&s.Timestamp, &res, &s.CPUPercent, &s.MemPercent, &s.DiskPercent,
			&s.SwapPercent, &s.NetRxRate, &s.NetTxRate, &s.LoadAvg1); err != nil {
			return nil, err
		}
		// Bucket by 1m boundary so finer overlaps win.
		key := s.Timestamp - (s.Timestamp % 60)
		prio := resPriority[res]
		if existing, ok := bucket[key]; ok {
			if prio < existing.prio {
				out[existing.idx] = s
				bucket[key] = seen{idx: existing.idx, prio: prio}
			}
			continue
		}
		bucket[key] = seen{idx: len(out), prio: prio}
		out = append(out, s)
	}
	return out, rows.Err()
}
