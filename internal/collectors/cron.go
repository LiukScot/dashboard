package collectors

import (
	"bufio"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type CronCollector struct {
	db      *sql.DB
	paths   []string
	logPath string
}

type CronJob struct {
	Fingerprint string `json:"fingerprint"`
	Source      string `json:"source"`
	Line        int    `json:"line"`
	Schedule    string `json:"schedule"`
	User        string `json:"user"`
	Command     string `json:"command"`
}

type CronOccurrence struct {
	ID           string `json:"id"`
	JobID        string `json:"jobId"`
	ScheduledAt  string `json:"scheduledAt"`
	DayKey       string `json:"dayKey"`
	MinutesOfDay int    `json:"minutesOfDay"`
	DisplayTime  string `json:"displayTime"`
	Status       string `json:"status"`
	Source       string `json:"source"`
	User         string `json:"user"`
	Command      string `json:"command"`
}

type CronHistoryItem struct {
	JobID       string `json:"jobId"`
	ScheduledAt string `json:"scheduledAt"`
	ObservedAt  string `json:"observedAt"`
	Status      string `json:"status"`
	Source      string `json:"source"`
	Message     string `json:"message"`
}

type CronWeek struct {
	Start           string            `json:"start"`
	End             string            `json:"end"`
	Days            []string          `json:"days"`
	Timezone        string            `json:"timezone"`
	HistoryCoverage string            `json:"historyCoverage"`
	HiddenJobCount  int               `json:"hiddenJobCount"`
	Jobs            []CronJob         `json:"jobs"`
	Occurrences     []CronOccurrence  `json:"occurrences"`
	History         []CronHistoryItem `json:"history"`
	Warnings        []string          `json:"warnings"`
}

type cronExpr struct {
	minutes     []int
	hours       []int
	dom         []int
	months      []int
	dow         []int
	domWildcard bool
	dowWildcard bool
}

func NewCronCollector(database *sql.DB, paths []string, logPath string) *CronCollector {
	return &CronCollector{db: database, paths: paths, logPath: logPath}
}

func (c *CronCollector) Week(start time.Time) (CronWeek, error) {
	weekStart := startOfDay(start)
	weekEnd := weekStart.AddDate(0, 0, 7)
	jobs, warnings := c.ReadJobs()
	hiddenCount := 0
	if c.db != nil {
		warnings = append(warnings, c.upsertJobs(jobs)...)
		_, importWarnings := c.importLogHistory(jobs, weekStart, weekEnd)
		warnings = append(warnings, importWarnings...)
		hidden, err := c.hiddenJobIDs()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("hidden jobs: %v", err))
		} else {
			current := jobSet(jobs)
			hidden = c.pruneStaleHidden(hidden, current)
			hiddenCount = len(hidden)
			jobs = visibleJobs(jobs, hidden)
		}
	}

	occurrences := make([]CronOccurrence, 0)
	for _, job := range jobs {
		expr, err := parseCronExpr(job.Schedule)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("parse %s:%d: %v", job.Source, job.Line, err))
			continue
		}
		for _, when := range expr.occurrences(weekStart, weekEnd) {
			occurrences = append(occurrences, CronOccurrence{
				ID:           job.Fingerprint + "-" + when.Format("200601021504"),
				JobID:        job.Fingerprint,
				ScheduledAt:  when.Format(time.RFC3339),
				DayKey:       when.Format("2006-01-02"),
				MinutesOfDay: when.Hour()*60 + when.Minute(),
				DisplayTime:  when.Format("15:04"),
				Status:       statusForOccurrence(when),
				Source:       job.Source,
				User:         job.User,
				Command:      job.Command,
			})
		}
	}

	sort.Slice(occurrences, func(i, j int) bool {
		return occurrences[i].ScheduledAt < occurrences[j].ScheduledAt
	})

	history, err := c.history(weekStart, weekEnd)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("history: %v", err))
	}
	visibleJobIDs := map[string]bool{}
	for _, job := range jobs {
		visibleJobIDs[job.Fingerprint] = true
	}
	filteredHistory := make([]CronHistoryItem, 0, len(history))
	for _, item := range history {
		if visibleJobIDs[item.JobID] {
			filteredHistory = append(filteredHistory, item)
		}
	}
	history = filteredHistory
	if history == nil {
		history = []CronHistoryItem{}
	}
	if jobs == nil {
		jobs = []CronJob{}
	}
	if occurrences == nil {
		occurrences = []CronOccurrence{}
	}
	if warnings == nil {
		warnings = []string{}
	}
	historyByOccurrence := map[string]CronHistoryItem{}
	for _, item := range history {
		historyByOccurrence[item.JobID+"\x00"+item.ScheduledAt] = item
	}
	for i := range occurrences {
		if item, ok := historyByOccurrence[occurrences[i].JobID+"\x00"+occurrences[i].ScheduledAt]; ok {
			occurrences[i].Status = item.Status
		}
	}

	coverage := "none"
	if len(history) > 0 {
		coverage = "partial"
	}

	return CronWeek{
		Start:           weekStart.Format("2006-01-02"),
		End:             weekEnd.Add(-time.Second).Format("2006-01-02"),
		Days:            dayKeys(weekStart, 7),
		Timezone:        weekStart.Location().String(),
		HistoryCoverage: coverage,
		HiddenJobCount:  hiddenCount,
		Jobs:            jobs,
		Occurrences:     occurrences,
		History:         history,
		Warnings:        warnings,
	}, nil
}

func (c *CronCollector) ReadJobs() ([]CronJob, []string) {
	files, warnings := c.resolveFiles()
	jobs := make([]CronJob, 0)
	seen := map[string]bool{}

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("read %s: %v", file, err))
			continue
		}
		for lineNo, raw := range strings.Split(string(content), "\n") {
			job, ok, warning := parseCronLine(raw, file, lineNo+1)
			if warning != "" {
				warnings = append(warnings, warning)
			}
			if !ok || seen[job.Fingerprint] {
				continue
			}
			seen[job.Fingerprint] = true
			jobs = append(jobs, job)
		}
	}

	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].Source == jobs[j].Source {
			return jobs[i].Line < jobs[j].Line
		}
		return jobs[i].Source < jobs[j].Source
	})

	return jobs, warnings
}

func (c *CronCollector) HideJob(fingerprint string) error {
	if c.db == nil {
		return fmt.Errorf("database unavailable")
	}
	_, err := c.db.Exec(
		`INSERT INTO cron_hidden_jobs(job_id, hidden_at)
		 VALUES(?, datetime('now'))
		 ON CONFLICT(job_id) DO UPDATE SET hidden_at=datetime('now')`,
		fingerprint,
	)
	return err
}

func (c *CronCollector) ResetHiddenJobs() error {
	if c.db == nil {
		return fmt.Errorf("database unavailable")
	}
	_, err := c.db.Exec(`DELETE FROM cron_hidden_jobs`)
	return err
}

func (c *CronCollector) HiddenJobCount() (int, error) {
	if c.db == nil {
		return 0, fmt.Errorf("database unavailable")
	}
	jobs, _ := c.ReadJobs()
	hidden, err := c.hiddenJobIDs()
	if err != nil {
		return 0, err
	}
	hidden = c.pruneStaleHidden(hidden, jobSet(jobs))
	return len(hidden), nil
}

func (c *CronCollector) hiddenJobIDs() (map[string]bool, error) {
	rows, err := c.db.Query(`SELECT job_id FROM cron_hidden_jobs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hidden := map[string]bool{}
	for rows.Next() {
		var jobID string
		if err := rows.Scan(&jobID); err != nil {
			return nil, err
		}
		hidden[jobID] = true
	}
	return hidden, rows.Err()
}

func visibleJobs(jobs []CronJob, hidden map[string]bool) []CronJob {
	visible := make([]CronJob, 0, len(jobs))
	for _, job := range jobs {
		if !hidden[job.Fingerprint] {
			visible = append(visible, job)
		}
	}
	return visible
}

func jobSet(jobs []CronJob) map[string]bool {
	current := make(map[string]bool, len(jobs))
	for _, job := range jobs {
		current[job.Fingerprint] = true
	}
	return current
}

func (c *CronCollector) pruneStaleHidden(hidden map[string]bool, current map[string]bool) map[string]bool {
	active := map[string]bool{}
	for fp := range hidden {
		if current[fp] {
			active[fp] = true
			continue
		}
		if c.db != nil {
			if _, err := c.db.Exec(`DELETE FROM cron_hidden_jobs WHERE job_id = ?`, fp); err != nil {
				log.Printf("prune stale hidden cron job %s: %v", fp, err)
			}
		}
	}
	return active
}

func (c *CronCollector) resolveFiles() ([]string, []string) {
	var files []string
	var warnings []string
	for _, path := range c.paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		matches, err := filepath.Glob(path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("glob %s: %v", path, err))
			continue
		}
		if len(matches) == 0 && !strings.ContainsAny(path, "*?[") {
			matches = []string{path}
		}
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("stat %s: %v", match, err))
				continue
			}
			if info.IsDir() {
				continue
			}
			files = append(files, match)
		}
	}
	sort.Strings(files)
	return files, warnings
}

func parseCronLine(raw string, source string, lineNo int) (CronJob, bool, string) {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") || (strings.Contains(line, "=") && !strings.Contains(line, " ")) {
		return CronJob{}, false, ""
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return CronJob{}, false, ""
	}

	scheduleFields, warning := normalizeScheduleFields(fields[0])
	if warning != "" {
		return CronJob{}, false, fmt.Sprintf("skip %s:%d: %s", source, lineNo, warning)
	}

	commandStart := 5
	user := cronSourceUser(source)
	if len(scheduleFields) > 0 {
		fields = append(scheduleFields, fields[1:]...)
	}
	if len(fields) < 6 {
		return CronJob{}, false, fmt.Sprintf("skip %s:%d: malformed cron line", source, lineNo)
	}
	if usesSystemCronUser(source) {
		if len(fields) < 7 {
			return CronJob{}, false, fmt.Sprintf("skip %s:%d: missing cron user", source, lineNo)
		}
		user = fields[5]
		commandStart = 6
	}
	schedule := strings.Join(fields[:5], " ")
	command := strings.Join(fields[commandStart:], " ")
	if command == "" {
		return CronJob{}, false, fmt.Sprintf("skip %s:%d: missing command", source, lineNo)
	}
	job := CronJob{
		Fingerprint: fingerprint(schedule, user, command, source),
		Source:      source,
		Line:        lineNo,
		Schedule:    schedule,
		User:        user,
		Command:     command,
	}
	return job, true, ""
}

var cronNicknames = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

func normalizeScheduleFields(first string) ([]string, string) {
	if !strings.HasPrefix(first, "@") {
		return nil, ""
	}
	if expanded, ok := cronNicknames[strings.ToLower(first)]; ok {
		return strings.Fields(expanded), ""
	}
	return nil, fmt.Sprintf("unsupported cron nickname %q", first)
}

func usesSystemCronUser(source string) bool {
	base := filepath.Base(source)
	return source == "/etc/crontab" || strings.Contains(source, "/etc/cron.d/") || base == "crontab"
}

func cronSourceUser(source string) string {
	for _, prefix := range []string{
		"/var/spool/cron/",
		"/var/spool/cron/crontabs/",
		"/var/spool/cron/tabs/",
		"/host/var/spool/cron/",
		"/host/var/spool/cron/crontabs/",
		"/host/var/spool/cron/tabs/",
		"/app/data/cron-user-spool/",
	} {
		if strings.HasPrefix(source, prefix) {
			return filepath.Base(source)
		}
	}
	return ""
}

// fingerprintLen is the number of hex chars kept from the SHA-256 digest.
// 16 hex chars = 64 bits: collision probability ~1e-15 for <1M distinct jobs.
const fingerprintLen = 16

func fingerprint(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])[:fingerprintLen]
}

func parseCronExpr(raw string) (cronExpr, error) {
	fields := strings.Fields(raw)
	if len(fields) != 5 {
		return cronExpr{}, fmt.Errorf("expected 5 fields")
	}
	fields[3] = normalizeNamedField(fields[3], monthNames)
	fields[4] = normalizeNamedField(fields[4], weekdayNames)
	minutes, _, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return cronExpr{}, fmt.Errorf("minute: %w", err)
	}
	hours, _, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return cronExpr{}, fmt.Errorf("hour: %w", err)
	}
	dom, domWildcard, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return cronExpr{}, fmt.Errorf("day-of-month: %w", err)
	}
	months, _, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return cronExpr{}, fmt.Errorf("month: %w", err)
	}
	dow, dowWildcard, err := parseCronField(fields[4], 0, 7)
	if err != nil {
		return cronExpr{}, fmt.Errorf("day-of-week: %w", err)
	}
	for i, day := range dow {
		if day == 7 {
			dow[i] = 0
		}
	}
	dow = uniqueSorted(dow)
	return cronExpr{minutes: minutes, hours: hours, dom: dom, months: months, dow: dow, domWildcard: domWildcard, dowWildcard: dowWildcard}, nil
}

var monthNames = map[string]string{
	"jan": "1", "feb": "2", "mar": "3", "apr": "4", "may": "5", "jun": "6",
	"jul": "7", "aug": "8", "sep": "9", "oct": "10", "nov": "11", "dec": "12",
}

var weekdayNames = map[string]string{
	"sun": "0", "mon": "1", "tue": "2", "wed": "3", "thu": "4", "fri": "5", "sat": "6",
}

func normalizeNamedField(raw string, mapping map[string]string) string {
	result := strings.ToLower(raw)
	for name, value := range mapping {
		result = strings.ReplaceAll(result, name, value)
	}
	return result
}

func parseCronField(raw string, min int, max int) ([]int, bool, error) {
	values := map[int]bool{}
	wildcard := raw == "*"
	for _, part := range strings.Split(raw, ",") {
		step := 1
		base := part
		if strings.Contains(part, "/") {
			pieces := strings.Split(part, "/")
			if len(pieces) != 2 {
				return nil, false, fmt.Errorf("bad step %q", part)
			}
			base = pieces[0]
			n, err := strconv.Atoi(pieces[1])
			if err != nil || n <= 0 {
				return nil, false, fmt.Errorf("bad step %q", part)
			}
			step = n
		}
		start, end := min, max
		switch {
		case base == "*":
		case strings.Contains(base, "-"):
			bounds := strings.Split(base, "-")
			if len(bounds) != 2 {
				return nil, false, fmt.Errorf("bad range %q", base)
			}
			var err error
			start, err = strconv.Atoi(bounds[0])
			if err != nil {
				return nil, false, fmt.Errorf("bad range %q", base)
			}
			end, err = strconv.Atoi(bounds[1])
			if err != nil {
				return nil, false, fmt.Errorf("bad range %q", base)
			}
		default:
			n, err := strconv.Atoi(base)
			if err != nil {
				return nil, false, fmt.Errorf("bad value %q", base)
			}
			start, end = n, n
		}
		if start < min || end > max || start > end {
			return nil, false, fmt.Errorf("value out of range %q", part)
		}
		for value := start; value <= end; value += step {
			values[value] = true
		}
	}
	result := make([]int, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Ints(result)
	return result, wildcard, nil
}

func (expr cronExpr) occurrences(start time.Time, end time.Time) []time.Time {
	var result []time.Time
	for day := startOfDay(start); day.Before(end); day = day.AddDate(0, 0, 1) {
		if !expr.matchesDate(day) {
			continue
		}
		for _, hour := range expr.hours {
			for _, minute := range expr.minutes {
				when := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, day.Location())
				if !when.Before(start) && when.Before(end) {
					result = append(result, when)
				}
			}
		}
	}
	return result
}

func (expr cronExpr) matchesDate(day time.Time) bool {
	if !contains(expr.months, int(day.Month())) {
		return false
	}
	domMatch := contains(expr.dom, day.Day())
	dowMatch := contains(expr.dow, int(day.Weekday()))
	if expr.domWildcard || expr.dowWildcard {
		return domMatch && dowMatch
	}
	return domMatch || dowMatch
}

func contains(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func uniqueSorted(values []int) []int {
	seen := map[int]bool{}
	result := make([]int, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Ints(result)
	return result
}

func startOfDay(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func statusForOccurrence(when time.Time) string {
	if when.After(time.Now()) {
		return "scheduled"
	}
	return "planned"
}

func (c *CronCollector) upsertJobs(jobs []CronJob) []string {
	if len(jobs) == 0 {
		return nil
	}
	tx, err := c.db.Begin()
	if err != nil {
		return []string{fmt.Sprintf("begin upsert tx: %v", err)}
	}
	stmt, err := tx.Prepare(
		`INSERT INTO cron_jobs(fingerprint, source, line, schedule, user, command, enabled, updated_at)
		 VALUES(?, ?, ?, ?, ?, ?, 1, datetime('now'))
		 ON CONFLICT(fingerprint) DO UPDATE SET
			source=excluded.source,
			line=excluded.line,
			schedule=excluded.schedule,
			user=excluded.user,
			command=excluded.command,
			enabled=1,
			updated_at=datetime('now')`)
	if err != nil {
		_ = tx.Rollback()
		return []string{fmt.Sprintf("prepare upsert: %v", err)}
	}
	defer stmt.Close()
	var warnings []string
	for _, job := range jobs {
		if _, err := stmt.Exec(job.Fingerprint, job.Source, job.Line, job.Schedule, job.User, job.Command); err != nil {
			warnings = append(warnings, fmt.Sprintf("store %s:%d: %v", job.Source, job.Line, err))
		}
	}
	if err := tx.Commit(); err != nil {
		_ = tx.Rollback()
		return append(warnings, fmt.Sprintf("commit upsert tx: %v", err))
	}
	return warnings
}

func (c *CronCollector) history(start time.Time, end time.Time) ([]CronHistoryItem, error) {
	if c.db == nil {
		return nil, nil
	}
	rows, err := c.db.Query(
		`SELECT job_id, scheduled_at, observed_at, status, source, message
		 FROM cron_run_history
		 WHERE scheduled_at >= ? AND scheduled_at < ?
		 ORDER BY scheduled_at ASC`,
		start.Format(time.RFC3339), end.Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []CronHistoryItem
	for rows.Next() {
		var item CronHistoryItem
		if err := rows.Scan(&item.JobID, &item.ScheduledAt, &item.ObservedAt, &item.Status, &item.Source, &item.Message); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (c *CronCollector) importLogHistory(jobs []CronJob, start time.Time, end time.Time) (int, []string) {
	if c.logPath == "" {
		return 0, nil
	}
	reference := end.Add(-time.Second)
	byCommand := map[string]CronJob{}
	for _, job := range jobs {
		key := historyKey(job.User, job.Command)
		byCommand[key] = job
	}

	if imported, warnings, usedJournal := c.importJournalHistory(byCommand, start, end); usedJournal {
		return imported, warnings
	}

	logFiles := []string{
		filepath.Join(c.logPath, "cron"),
		filepath.Join(c.logPath, "syslog"),
		filepath.Join(c.logPath, "messages"),
	}
	imported := 0
	var warnings []string

	tx, err := c.db.Begin()
	if err != nil {
		return 0, []string{fmt.Sprintf("history tx: %v", err)}
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, logFile := range logFiles {
		file, err := os.Open(logFile)
		if err != nil {
			if !os.IsNotExist(err) {
				warnings = append(warnings, fmt.Sprintf("read %s: %v", logFile, err))
			}
			continue
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			observedAt, user, command, ok := parseCronLogLine(scanner.Text(), reference)
			if !ok || observedAt.Before(start) || !observedAt.Before(end) {
				continue
			}
			job, exists := byCommand[historyKey(user, command)]
			if !exists {
				continue
			}
			if err := txInsertHistory(tx, job.Fingerprint, observedAt, "observed", logFile, scanner.Text()); err != nil {
				warnings = append(warnings, fmt.Sprintf("insert history %s at %s from %s: %v (line: %q)", job.Fingerprint, observedAt.Format(time.RFC3339), logFile, err, scanner.Text()))
			} else {
				imported++
			}
		}
		if err := scanner.Err(); err != nil {
			warnings = append(warnings, fmt.Sprintf("scan %s: %v", logFile, err))
		}
		_ = file.Close()
	}

	if err := tx.Commit(); err != nil {
		return 0, append(warnings, fmt.Sprintf("history commit: %v", err))
	}
	committed = true

	return imported, warnings
}

func (c *CronCollector) importJournalHistory(byCommand map[string]CronJob, start time.Time, end time.Time) (int, []string, bool) {
	if _, err := exec.LookPath("journalctl"); err != nil {
		return 0, nil, false
	}

	args := []string{
		"--since", start.Format("2006-01-02 15:04:05"),
		"--until", end.Format("2006-01-02 15:04:05"),
		"--no-pager",
		"--grep=CRON",
	}
	for _, dir := range []string{"/var/log/journal", "/run/log/journal"} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			args = append(args, "--directory="+dir)
			break
		}
	}

	cmd := exec.Command("journalctl", args...)
	output, err := cmd.Output()
	if err != nil {
		// journalctl is present but errored (perms, no readable journal,
		// etc.); signal the caller to fall back to syslog files instead of
		// silently masking the failure as a successful empty journal.
		return 0, []string{fmt.Sprintf("journalctl: %v", err)}, false
	}

	tx, err := c.db.Begin()
	if err != nil {
		return 0, []string{fmt.Sprintf("history tx: %v", err)}, true
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	imported := 0
	var warnings []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		observedAt, user, command, ok := parseCronLogLine(scanner.Text(), end.Add(-time.Second))
		if !ok || observedAt.Before(start) || !observedAt.Before(end) {
			continue
		}
		job, exists := byCommand[historyKey(user, command)]
		if !exists {
			continue
		}
		if err := txInsertHistory(tx, job.Fingerprint, observedAt, "observed", "journalctl", scanner.Text()); err != nil {
			warnings = append(warnings, fmt.Sprintf("insert history %s at %s from journalctl: %v (line: %q)", job.Fingerprint, observedAt.Format(time.RFC3339), err, scanner.Text()))
		} else {
			imported++
		}
	}
	if err := scanner.Err(); err != nil {
		warnings = append(warnings, fmt.Sprintf("journalctl scan: %v", err))
	}

	if err := tx.Commit(); err != nil {
		return 0, append(warnings, fmt.Sprintf("history commit: %v", err)), true
	}
	committed = true

	return imported, warnings, true
}

func parseCronLogLine(line string, now time.Time) (time.Time, string, string, bool) {
	if !strings.Contains(line, "CRON") || !strings.Contains(line, " CMD (") {
		return time.Time{}, "", "", false
	}
	parts := strings.Fields(line)
	if len(parts) < 6 {
		return time.Time{}, "", "", false
	}
	when, err := time.ParseInLocation("Jan _2 15:04:05", strings.Join(parts[:3], " "), now.Location())
	if err != nil {
		return time.Time{}, "", "", false
	}
	when = time.Date(now.Year(), when.Month(), when.Day(), when.Hour(), when.Minute(), when.Second(), 0, now.Location())
	if when.After(now.Add(24 * time.Hour)) {
		when = when.AddDate(-1, 0, 0)
	}

	userStart := strings.Index(line, "): (")
	prefixLen := 4
	if userStart == -1 {
		userStart = strings.Index(line, ": (")
		prefixLen = 3
	}
	if userStart == -1 {
		return time.Time{}, "", "", false
	}
	afterPrefix := line[userStart+prefixLen:]
	userEnd := strings.Index(afterPrefix, ") CMD (")
	if userEnd == -1 {
		return time.Time{}, "", "", false
	}
	user := afterPrefix[:userEnd]
	command := strings.TrimSuffix(afterPrefix[userEnd+7:], ")")
	if user == "" || command == "" {
		return time.Time{}, "", "", false
	}
	return when, user, command, true
}

// txInsertHistory batches
// thousands of inserts per call, so a single Begin/Commit beats the per-row
// round trips against the SetMaxOpenConns(1) sqlite pool.
func txInsertHistory(tx *sql.Tx, jobID string, observedAt time.Time, status string, source string, message string) error {
	_, err := tx.Exec(
		`INSERT OR IGNORE INTO cron_run_history(job_id, scheduled_at, observed_at, status, source, message)
		 VALUES(?, ?, ?, ?, ?, ?)`,
		jobID,
		observedAt.Truncate(time.Minute).Format(time.RFC3339),
		observedAt.Format(time.RFC3339),
		status,
		source,
		message,
	)
	return err
}

func historyKey(user string, command string) string {
	return strings.TrimSpace(user) + "\x00" + strings.TrimSpace(command)
}

func dayKeys(start time.Time, count int) []string {
	days := make([]string, 0, count)
	for i := 0; i < count; i++ {
		days = append(days, start.AddDate(0, 0, i).Format("2006-01-02"))
	}
	return days
}
