package collectors

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LiukScot/dashboard/internal/db"
)

func TestParseCronLineSystemCrontabIncludesUser(t *testing.T) {
	t.Parallel()

	job, ok, warning := parseCronLine("15 10 * * 1 root /usr/local/bin/backup", "/etc/crontab", 12)
	if !ok {
		t.Fatalf("expected line to parse, warning %q", warning)
	}
	if job.Schedule != "15 10 * * 1" {
		t.Fatalf("unexpected schedule %q", job.Schedule)
	}
	if job.User != "root" {
		t.Fatalf("unexpected user %q", job.User)
	}
	if job.Command != "/usr/local/bin/backup" {
		t.Fatalf("unexpected command %q", job.Command)
	}
}

func TestParseCronLineUserSpoolInfersUserFromFilename(t *testing.T) {
	t.Parallel()

	job, ok, warning := parseCronLine("0 2 * * * /home/luca/scripts/sync-repos.sh", "/var/spool/cron/luca", 1)
	if !ok {
		t.Fatalf("expected line to parse, warning %q", warning)
	}
	if job.Schedule != "0 2 * * *" {
		t.Fatalf("unexpected schedule %q", job.Schedule)
	}
	if job.User != "luca" {
		t.Fatalf("unexpected user %q", job.User)
	}
	if job.Command != "/home/luca/scripts/sync-repos.sh" {
		t.Fatalf("unexpected command %q", job.Command)
	}
}

func TestParseCronLineHostMountedUserSpoolInfersUserFromFilename(t *testing.T) {
	t.Parallel()

	job, ok, warning := parseCronLine("0 3 * * * /usr/bin/python3 /home/luca/scripts/sync-youtube-music.py", "/host/var/spool/cron/luca", 2)
	if !ok {
		t.Fatalf("expected line to parse, warning %q", warning)
	}
	if job.User != "luca" {
		t.Fatalf("unexpected user %q", job.User)
	}
}

func TestParseCronExprExpandsWeeklyOccurrences(t *testing.T) {
	t.Parallel()

	expr, err := parseCronExpr("0 9 * * 1,3")
	if err != nil {
		t.Fatalf("parse expr: %v", err)
	}

	start := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	got := expr.occurrences(start, start.AddDate(0, 0, 7))
	if len(got) != 2 {
		t.Fatalf("expected 2 occurrences, got %d", len(got))
	}
	if got[0].Weekday() != time.Monday || got[1].Weekday() != time.Wednesday {
		t.Fatalf("unexpected weekdays %s and %s", got[0].Weekday(), got[1].Weekday())
	}
}

func TestParseCronExprSupportsNamedWeekday(t *testing.T) {
	t.Parallel()

	expr, err := parseCronExpr("0 9 * * sun")
	if err != nil {
		t.Fatalf("parse expr: %v", err)
	}

	start := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	got := expr.occurrences(start, start.AddDate(0, 0, 7))
	if len(got) != 1 || got[0].Weekday() != time.Sunday {
		t.Fatalf("expected one Sunday occurrence, got %#v", got)
	}
}

func TestCronCollectorReadsCronFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "crontab")
	if err := os.WriteFile(path, []byte("0 */6 * * * root /bin/echo hello\n"), 0600); err != nil {
		t.Fatalf("write crontab: %v", err)
	}

	collector := NewCronCollector(nil, []string{path}, "")
	jobs, warnings := collector.ReadJobs()
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Command != "/bin/echo hello" {
		t.Fatalf("unexpected command %q", jobs[0].Command)
	}
}

func TestParseCronLineSupportsHourlyNickname(t *testing.T) {
	t.Parallel()

	job, ok, warning := parseCronLine("@hourly root /usr/local/bin/hourly-task", "/etc/cron.d/custom", 3)
	if !ok {
		t.Fatalf("expected nickname line to parse, warning %q", warning)
	}
	if job.Schedule != "0 * * * *" {
		t.Fatalf("unexpected schedule %q", job.Schedule)
	}
	if job.User != "root" {
		t.Fatalf("unexpected user %q", job.User)
	}
}

func TestParseCronLogLine(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	observedAt, user, command, ok := parseCronLogLine("Apr 29 10:15:01 host CRON[123]: (root) CMD (/usr/local/bin/backup)", now)
	if !ok {
		t.Fatal("expected cron log line to parse")
	}
	if observedAt.Format(time.RFC3339) != "2026-04-29T10:15:01Z" {
		t.Fatalf("unexpected observed time %s", observedAt.Format(time.RFC3339))
	}
	if user != "root" {
		t.Fatalf("unexpected user %q", user)
	}
	if command != "/usr/local/bin/backup" {
		t.Fatalf("unexpected command %q", command)
	}
}

func TestCronCollectorHidesJobsFromWeek(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "dashboard.sqlite")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.RunMigrations(database); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	cronPath := filepath.Join(dir, "crontab")
	if err := os.WriteFile(cronPath, []byte("0 * * * * root run-parts /etc/cron.hourly\n"), 0600); err != nil {
		t.Fatalf("write crontab: %v", err)
	}

	collector := NewCronCollector(database, []string{cronPath}, "")
	week, err := collector.Week(time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("load week: %v", err)
	}
	if len(week.Jobs) != 1 {
		t.Fatalf("expected job before hide, got %d", len(week.Jobs))
	}

	if err := collector.HideJob(week.Jobs[0].Fingerprint); err != nil {
		t.Fatalf("hide job: %v", err)
	}

	week, err = collector.Week(time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("reload week: %v", err)
	}
	if len(week.Jobs) != 0 || len(week.Occurrences) != 0 {
		t.Fatalf("expected hidden job to be filtered, got %d jobs and %d occurrences", len(week.Jobs), len(week.Occurrences))
	}

	if err := collector.ResetHiddenJobs(); err != nil {
		t.Fatalf("reset hidden jobs: %v", err)
	}
	count, err := collector.HiddenJobCount()
	if err != nil {
		t.Fatalf("count hidden jobs: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected hidden count 0, got %d", count)
	}
}

func TestCronCollectorPrunesStaleHiddenJobs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "dashboard.sqlite")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.RunMigrations(database); err != nil {
		t.Fatalf("migrations: %v", err)
	}

	cronPath := filepath.Join(dir, "crontab")
	if err := os.WriteFile(cronPath, []byte("0 * * * * root run-parts /etc/cron.hourly\n"), 0600); err != nil {
		t.Fatalf("write crontab: %v", err)
	}

	collector := NewCronCollector(database, []string{cronPath}, "")
	week, err := collector.Week(time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("load week: %v", err)
	}
	if err := collector.HideJob(week.Jobs[0].Fingerprint); err != nil {
		t.Fatalf("hide job: %v", err)
	}

	if err := os.WriteFile(cronPath, []byte("15 * * * * root /usr/local/bin/backup\n"), 0600); err != nil {
		t.Fatalf("rewrite crontab: %v", err)
	}

	count, err := collector.HiddenJobCount()
	if err != nil {
		t.Fatalf("count hidden jobs: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected stale hidden job to be pruned, got %d", count)
	}
}

func TestParseCronLogLineUsesRequestedWeekYear(t *testing.T) {
	t.Parallel()

	reference := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	observedAt, _, _, ok := parseCronLogLine("Dec 31 23:59:59 host CRON[123]: (root) CMD (/usr/local/bin/backup)", reference)
	if !ok {
		t.Fatal("expected cron log line to parse")
	}
	if observedAt.Year() != 2024 {
		t.Fatalf("expected prior year inference, got %s", observedAt.Format(time.RFC3339))
	}
}

func TestSanitizeWarningStripsRawLogLine(t *testing.T) {
	t.Parallel()

	in := `insert history ab12 at 2025-01-02T03:04:05Z from journalctl: disk full (line: "Jan  2 03:04:05 host CRON[42]: (luca) CMD (/home/luca/secret.sh)")`
	got := sanitizeWarning(in)

	if strings.Contains(got, "line:") {
		t.Fatalf("raw log line fragment not stripped: %q", got)
	}
	if strings.Contains(got, "luca") || strings.Contains(got, "secret.sh") {
		t.Fatalf("PII from log line leaked: %q", got)
	}
	if !strings.Contains(got, "disk full") {
		t.Fatalf("diagnostic detail should be kept: %q", got)
	}
}

func TestSanitizeWarningReducesAbsolutePathsToBasename(t *testing.T) {
	t.Parallel()

	got := sanitizeWarning("read /var/spool/cron/luca: permission denied")
	if strings.Contains(got, "/var/spool/cron") {
		t.Fatalf("absolute path not reduced: %q", got)
	}
	if !strings.Contains(got, "luca") {
		t.Fatalf("basename should be retained for diagnostics: %q", got)
	}
	if !strings.Contains(got, "permission denied") {
		t.Fatalf("error detail should be kept: %q", got)
	}
}

func TestSanitizeWarningLeavesPlainTextUntouched(t *testing.T) {
	t.Parallel()

	in := "unsupported cron nickname \"@reboot\""
	if got := sanitizeWarning(in); got != in {
		t.Fatalf("plain warning altered: %q -> %q", in, got)
	}
}
