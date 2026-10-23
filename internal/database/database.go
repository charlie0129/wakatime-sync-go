package database

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func New(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, err
	}

	d := &DB{db}
	if err := d.migrate(); err != nil {
		return nil, err
	}

	slog.Info("database initialized", "path", path)
	return d, nil
}

func (db *DB) migrate() error {
	for _, m := range baseSchema {
		if _, err := db.Exec(m); err != nil {
			return err
		}
	}

	// Upgrade the schema version by version, tracked in PRAGMA user_version
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	for ; version < len(schemaMigrations); version++ {
		if err := db.applySchemaMigration(version + 1); err != nil {
			return fmt.Errorf("failed to migrate database schema to version %d: %w", version+1, err)
		}
		slog.Info("migrated database schema", "version", version+1)
	}

	return nil
}

func (db *DB) applySchemaMigration(version int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, m := range schemaMigrations[version-1] {
		if _, err := tx.Exec(m); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return err
	}
	return tx.Commit()
}

// aiColumns are the AI coding metrics stored for day summaries, day stats and durations, in the
// order of AIStats.values. Adding one requires a schema migration that adds it to these tables.
var aiColumns = []string{
	"ai_additions INTEGER",
	"ai_deletions INTEGER",
	"human_additions INTEGER",
	"human_deletions INTEGER",
	"ai_input_tokens INTEGER",
	"ai_cached_input_tokens INTEGER",
	"ai_output_tokens INTEGER",
	"ai_model_total_cost REAL",
	"ai_sessions INTEGER",
	"ai_prompt_events_total INTEGER",
	"ai_prompt_length_sum INTEGER",
	"ai_prompt_length_avg REAL",
	"ai_prompt_length_avg_per_session REAL",
	"ai_prompt_length_median_per_session REAL",
	"ai_prompt_events_avg_per_session REAL",
	"ai_prompt_events_median_per_session REAL",
}

// schemaMigrations upgrade baseSchema; migration i brings the schema to version i+1.
// Existing migrations must never change, only new ones may be appended.
var schemaMigrations = [][]string{
	aiStatsMigration(),
}

// aiStatsMigration adds AI coding stats, the heartbeat fields that were not stored, and user agents.
func aiStatsMigration() []string {
	var m []string
	for _, table := range []string{"day_summaries", "day_stats", "durations", "project_durations"} {
		for _, column := range []string{
			"ai_additions INTEGER",
			"ai_deletions INTEGER",
			"human_additions INTEGER",
			"human_deletions INTEGER",
			"ai_input_tokens INTEGER",
			"ai_cached_input_tokens INTEGER",
			"ai_output_tokens INTEGER",
			"ai_model_total_cost REAL",
			"ai_sessions INTEGER",
			"ai_prompt_events_total INTEGER",
			"ai_prompt_length_sum INTEGER",
			"ai_prompt_length_avg REAL",
			"ai_prompt_length_avg_per_session REAL",
			"ai_prompt_length_median_per_session REAL",
			"ai_prompt_events_avg_per_session REAL",
			"ai_prompt_events_median_per_session REAL",
		} {
			m = append(m, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", table, column))
		}
	}

	for _, column := range []string{
		"wakatime_id TEXT",
		"user_agent_id TEXT",
		"project_root_count INTEGER",
		"dependencies JSONB",
		"ai_line_changes INTEGER",
		"human_line_changes INTEGER",
		"ai_session TEXT",
		"ai_input_tokens INTEGER",
		"ai_cached_input_tokens INTEGER",
		"ai_output_tokens INTEGER",
		"ai_prompt_length INTEGER",
		"ai_subscription_plan TEXT",
	} {
		m = append(m, "ALTER TABLE heartbeats ADD COLUMN "+column)
	}

	return append(m,
		`ALTER TABLE durations ADD COLUMN ai_model_breakdown JSONB`,
		`ALTER TABLE project_durations ADD COLUMN ai_model_breakdown JSONB`,

		// Usage of each AI model per day, in total and per project or editor
		`CREATE TABLE day_ai_models (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			day DATE NOT NULL,
			type TEXT NOT NULL,
			name TEXT NOT NULL,
			model TEXT NOT NULL,
			lines INTEGER,
			input_tokens INTEGER,
			cached_input_tokens INTEGER,
			output_tokens INTEGER,
			cost REAL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(day, type, name, model)
		)`,
		`CREATE INDEX idx_day_ai_models_day ON day_ai_models(day)`,

		// Plugins which sent heartbeats, with the AI model they reported
		`CREATE TABLE user_agents (
			id TEXT PRIMARY KEY,
			value TEXT,
			editor TEXT,
			os TEXT,
			version TEXT,
			ai_model TEXT,
			ai_model_version TEXT,
			ai_model_complexity TEXT,
			is_browser_extension INTEGER DEFAULT 0,
			is_desktop_app INTEGER DEFAULT 0,
			first_seen_at TEXT,
			last_seen_at TEXT
		)`,
	)
}

// aiColumnNames returns the names of aiColumns, prefixed with prefix.
func aiColumnNames(prefix string) []string {
	names := make([]string, len(aiColumns))
	for i, column := range aiColumns {
		names[i] = prefix + strings.Fields(column)[0]
	}
	return names
}

// baseSchema is the schema before versioned migrations were introduced.
var baseSchema = []string{
	// Projects table
	`CREATE TABLE IF NOT EXISTS projects (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uuid TEXT UNIQUE,
		name TEXT NOT NULL,
		repository TEXT,
		badge TEXT,
		color TEXT,
		has_public_url INTEGER DEFAULT 0,
		last_heartbeat_at DATETIME,
		first_heartbeat_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_projects_name ON projects(name)`,

	// Durations table
	`CREATE TABLE IF NOT EXISTS durations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		day DATE NOT NULL,
		project TEXT,
		start_time REAL NOT NULL,
		duration REAL NOT NULL,
		dependencies JSONB,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_durations_day ON durations(day)`,
	`CREATE INDEX IF NOT EXISTS idx_durations_project ON durations(project)`,

	// Project durations table (detailed)
	`CREATE TABLE IF NOT EXISTS project_durations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		day DATE NOT NULL,
		project TEXT,
		branch TEXT,
		entity TEXT,
		language TEXT,
		type TEXT,
		start_time REAL NOT NULL,
		duration REAL NOT NULL,
		dependencies JSONB,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_project_durations_day ON project_durations(day)`,
	`CREATE INDEX IF NOT EXISTS idx_project_durations_project ON project_durations(project)`,

	// Heartbeats table
	`CREATE TABLE IF NOT EXISTS heartbeats (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		day DATE NOT NULL,
		entity TEXT NOT NULL,
		type TEXT,
		category TEXT,
		time REAL NOT NULL,
		project TEXT,
		branch TEXT,
		language TEXT,
		is_write INTEGER DEFAULT 0,
		machine_id TEXT,
		lines INTEGER,
		line_no INTEGER,
		cursor_pos INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_heartbeats_day ON heartbeats(day)`,
	`CREATE INDEX IF NOT EXISTS idx_heartbeats_time ON heartbeats(time)`,

	// Day summaries table (grand total per day)
	`CREATE TABLE IF NOT EXISTS day_summaries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		day DATE NOT NULL UNIQUE,
		total_seconds REAL NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE INDEX IF NOT EXISTS idx_day_summaries_day ON day_summaries(day)`,

	// Day stats table (breakdown by type: category, language, editor, os, project, dependency)
	`CREATE TABLE IF NOT EXISTS day_stats (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		day DATE NOT NULL,
		type TEXT NOT NULL,
		name TEXT NOT NULL,
		total_seconds REAL NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(day, type, name)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_day_stats_day ON day_stats(day)`,
	`CREATE INDEX IF NOT EXISTS idx_day_stats_type ON day_stats(type)`,

	// Sync log table (track what has been synced)
	`CREATE TABLE IF NOT EXISTS sync_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		day DATE NOT NULL UNIQUE,
		synced_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		total_seconds REAL,
		status TEXT DEFAULT 'success'
	)`,
	`CREATE INDEX IF NOT EXISTS idx_sync_log_day ON sync_log(day)`,
}

// --- Duration operations ---

// aiSelect returns the AI columns to select, prefixed with prefix, as 0 when NULL (data stored
// before AI stats were synced).
func aiSelect(prefix string) string {
	names := aiColumnNames(prefix)
	for i, name := range names {
		names[i] = "COALESCE(" + name + ", 0)"
	}
	return strings.Join(names, ", ")
}

func (db *DB) GetDurationsByDay(day time.Time) ([]Duration, error) {
	rows, err := db.Query(`
		SELECT id, day, COALESCE(project, ''), start_time, duration, COALESCE(json(dependencies), ''),
			COALESCE(json(ai_model_breakdown), ''), created_at, `+aiSelect("")+`
		FROM durations WHERE day = ? ORDER BY start_time
	`, day.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var durations []Duration
	for rows.Next() {
		var d Duration
		var dayStr string
		dest := []interface{}{&d.ID, &dayStr, &d.Project, &d.StartTime, &d.Duration, &d.Dependencies, &d.AIModelBreakdown, &d.CreatedAt}
		if err := rows.Scan(append(dest, d.pointers()...)...); err != nil {
			return nil, err
		}
		d.Day, _ = parseDay(dayStr)
		durations = append(durations, d)
	}
	return durations, rows.Err()
}

// --- Project Duration operations ---

func (db *DB) GetProjectDurationsByDay(day time.Time, project string) ([]ProjectDuration, error) {
	query := `
		SELECT id, day, COALESCE(project, ''), COALESCE(branch, ''), COALESCE(entity, ''), COALESCE(language, ''),
			COALESCE(type, ''), start_time, duration, COALESCE(json(dependencies), ''), COALESCE(json(ai_model_breakdown), ''),
			created_at, ` + aiSelect("") + `
		FROM project_durations WHERE day = ?
	`
	args := []interface{}{day.Format("2006-01-02")}
	if project != "" {
		query += " AND project = ?"
		args = append(args, project)
	}
	query += " ORDER BY start_time"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var durations []ProjectDuration
	for rows.Next() {
		var d ProjectDuration
		var dayStr string
		dest := []interface{}{&d.ID, &dayStr, &d.Project, &d.Branch, &d.Entity, &d.Language, &d.Type, &d.StartTime, &d.Duration, &d.Dependencies, &d.AIModelBreakdown, &d.CreatedAt}
		if err := rows.Scan(append(dest, d.pointers()...)...); err != nil {
			return nil, err
		}
		d.Day, _ = parseDay(dayStr)
		durations = append(durations, d)
	}
	return durations, rows.Err()
}

// --- Heartbeat operations ---

func (db *DB) GetHeartbeatsByDay(day time.Time) ([]HeartBeat, error) {
	// Heartbeats migrated from the old MySQL database lack most columns
	rows, err := db.Query(`
		SELECT id, COALESCE(wakatime_id, ''), day, entity, COALESCE(type, ''), COALESCE(category, ''), time,
			COALESCE(project, ''), COALESCE(project_root_count, 0), COALESCE(branch, ''), COALESCE(language, ''),
			COALESCE(json(dependencies), ''), COALESCE(is_write, 0), COALESCE(machine_id, ''), COALESCE(user_agent_id, ''),
			COALESCE(lines, 0), COALESCE(line_no, 0), COALESCE(cursor_pos, 0),
			COALESCE(ai_line_changes, 0), COALESCE(human_line_changes, 0), COALESCE(ai_session, ''),
			COALESCE(ai_input_tokens, 0), COALESCE(ai_cached_input_tokens, 0), COALESCE(ai_output_tokens, 0),
			COALESCE(ai_prompt_length, 0), COALESCE(ai_subscription_plan, ''), created_at
		FROM heartbeats WHERE day = ? ORDER BY time
	`, day.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var heartbeats []HeartBeat
	for rows.Next() {
		var h HeartBeat
		var dayStr string
		var isWrite int
		if err := rows.Scan(
			&h.ID, &h.WakaTimeID, &dayStr, &h.Entity, &h.Type, &h.Category, &h.Time,
			&h.Project, &h.ProjectRootCount, &h.Branch, &h.Language,
			&h.Dependencies, &isWrite, &h.MachineID, &h.UserAgentID,
			&h.Lines, &h.LineNo, &h.CursorPos,
			&h.AILineChanges, &h.HumanLineChanges, &h.AISession,
			&h.AIInputTokens, &h.AICachedInputTokens, &h.AIOutputTokens,
			&h.AIPromptLength, &h.AISubscriptionPlan, &h.CreatedAt,
		); err != nil {
			return nil, err
		}
		h.Day, _ = parseDay(dayStr)
		h.IsWrite = isWrite == 1
		heartbeats = append(heartbeats, h)
	}
	return heartbeats, rows.Err()
}

// --- Project operations ---

// UpsertProjects inserts or updates the given projects in a single transaction.
func (db *DB) UpsertProjects(projects []Project) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO projects (uuid, name, repository, badge, color, has_public_url, last_heartbeat_at, first_heartbeat_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(uuid) DO UPDATE SET
			name = excluded.name,
			repository = excluded.repository,
			badge = excluded.badge,
			color = excluded.color,
			has_public_url = excluded.has_public_url,
			last_heartbeat_at = excluded.last_heartbeat_at,
			first_heartbeat_at = excluded.first_heartbeat_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, p := range projects {
		if _, err := stmt.Exec(p.UUID, p.Name, p.Repository, p.Badge, p.Color, p.HasPublicURL, p.LastHeartbeatAt, p.FirstHeartbeatAt, time.Now()); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// UpsertUserAgents inserts or updates the given user agents in a single transaction.
func (db *DB) UpsertUserAgents(agents []UserAgent) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO user_agents (id, value, editor, os, version, ai_model, ai_model_version, ai_model_complexity, is_browser_extension, is_desktop_app, first_seen_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			value = excluded.value,
			editor = excluded.editor,
			os = excluded.os,
			version = excluded.version,
			ai_model = excluded.ai_model,
			ai_model_version = excluded.ai_model_version,
			ai_model_complexity = excluded.ai_model_complexity,
			is_browser_extension = excluded.is_browser_extension,
			is_desktop_app = excluded.is_desktop_app,
			first_seen_at = excluded.first_seen_at,
			last_seen_at = excluded.last_seen_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, a := range agents {
		if _, err := stmt.Exec(a.ID, a.Value, a.Editor, a.OS, a.Version, a.AIModel, a.AIModelVersion, a.AIModelComplexity, a.IsBrowserExtension, a.IsDesktopApp, a.FirstSeenAt, a.LastSeenAt); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) GetProjects(query string) ([]Project, error) {
	sql := "SELECT id, uuid, name, repository, badge, color, has_public_url, last_heartbeat_at, first_heartbeat_at, created_at FROM projects"
	var args []interface{}
	if query != "" {
		sql += " WHERE name LIKE ?"
		args = append(args, "%"+query+"%")
	}
	sql += " ORDER BY last_heartbeat_at DESC"

	rows, err := db.Query(sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.UUID, &p.Name, &p.Repository, &p.Badge, &p.Color, &p.HasPublicURL, &p.LastHeartbeatAt, &p.FirstHeartbeatAt, &p.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// --- Day Summary operations ---

func (db *DB) GetDaySummary(day time.Time) (*DaySummary, error) {
	var s DaySummary
	var dayStr string
	err := db.QueryRow(`
		SELECT id, day, total_seconds, created_at, `+aiSelect("")+`
		FROM day_summaries WHERE day = ?
	`, day.Format("2006-01-02")).Scan(append([]interface{}{&s.ID, &dayStr, &s.TotalSeconds, &s.CreatedAt}, s.pointers()...)...)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.Day, _ = time.Parse("2006-01-02", dayStr)
	return &s, nil
}

func (db *DB) GetDaySummaries(start, end time.Time) ([]DaySummary, error) {
	rows, err := db.Query(`
		SELECT id, day, total_seconds, created_at
		FROM day_summaries WHERE day >= ? AND day <= ? ORDER BY day
	`, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []DaySummary
	for rows.Next() {
		var s DaySummary
		var dayStr string
		if err := rows.Scan(&s.ID, &dayStr, &s.TotalSeconds, &s.CreatedAt); err != nil {
			return nil, err
		}
		s.Day, _ = time.Parse("2006-01-02", dayStr)
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}

// --- Day Stats operations ---

func (db *DB) GetDayStatsByDayAndType(day time.Time, statType string) ([]DayStats, error) {
	rows, err := db.Query(`
		SELECT id, day, type, name, total_seconds, created_at, `+aiSelect("")+`
		FROM day_stats WHERE day = ? AND type = ?
	`, day.Format("2006-01-02"), statType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []DayStats
	for rows.Next() {
		var s DayStats
		var dayStr string
		if err := rows.Scan(append([]interface{}{&s.ID, &dayStr, &s.Type, &s.Name, &s.TotalSeconds, &s.CreatedAt}, s.pointers()...)...); err != nil {
			return nil, err
		}
		s.Day, _ = time.Parse("2006-01-02", dayStr)
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// AggregatedStat is the total time of an item over a date range, with the sum of its AI coding
// metrics; averages and medians are left out as they cannot be summed.
type AggregatedStat struct {
	Name         string  `json:"name"`
	TotalSeconds float64 `json:"total_seconds"`
	AIStats
}

func (db *DB) GetAggregatedStats(start, end time.Time, statType string) ([]AggregatedStat, error) {
	rows, err := db.Query(`
		SELECT name, SUM(total_seconds) as total, `+aiSums("")+`
		FROM day_stats WHERE day >= ? AND day <= ? AND type = ?
		GROUP BY name ORDER BY total DESC
	`, start.Format("2006-01-02"), end.Format("2006-01-02"), statType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []AggregatedStat
	for rows.Next() {
		var s AggregatedStat
		if err := rows.Scan(append([]interface{}{&s.Name, &s.TotalSeconds}, s.sumPointers()...)...); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// summableAIColumns are the AI columns whose values add up over several days or items.
var summableAIColumns = []string{
	"ai_additions", "ai_deletions", "human_additions", "human_deletions",
	"ai_input_tokens", "ai_cached_input_tokens", "ai_output_tokens", "ai_model_total_cost",
	"ai_sessions", "ai_prompt_events_total", "ai_prompt_length_sum",
}

// aiSums returns the sums of summableAIColumns to select, prefixed with prefix.
func aiSums(prefix string) string {
	sums := make([]string, len(summableAIColumns))
	for i, column := range summableAIColumns {
		sums[i] = "COALESCE(SUM(" + prefix + column + "), 0)"
	}
	return strings.Join(sums, ", ")
}

// sumPointers returns pointers to the metrics in the order of summableAIColumns, to scan them.
func (a *AIStats) sumPointers() []interface{} {
	return []interface{}{
		&a.AIAdditions, &a.AIDeletions, &a.HumanAdditions, &a.HumanDeletions,
		&a.AIInputTokens, &a.AICachedInputTokens, &a.AIOutputTokens, &a.AIModelTotalCost,
		&a.AISessions, &a.AIPromptEventsTotal, &a.AIPromptLengthSum,
	}
}

func (db *DB) GetProjectDailyStats(start, end time.Time) ([]struct {
	Day          string  `json:"day"`
	Name         string  `json:"name"`
	TotalSeconds float64 `json:"total_seconds"`
}, error) {
	rows, err := db.Query(`
		SELECT day, name, total_seconds
		FROM day_stats WHERE day >= ? AND day <= ? AND type = 'project'
		ORDER BY day, total_seconds DESC
	`, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []struct {
		Day          string  `json:"day"`
		Name         string  `json:"name"`
		TotalSeconds float64 `json:"total_seconds"`
	}
	for rows.Next() {
		var s struct {
			Day          string  `json:"day"`
			Name         string  `json:"name"`
			TotalSeconds float64 `json:"total_seconds"`
		}
		if err := rows.Scan(&s.Day, &s.Name, &s.TotalSeconds); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// --- AI operations ---

// GetDayAIModels returns the usage of AI models on each day from start to end, in total and per
// project and editor.
func (db *DB) GetDayAIModels(start, end time.Time) ([]DayAIModel, error) {
	rows, err := db.Query(`
		SELECT day, type, name, model, COALESCE(lines, 0), COALESCE(input_tokens, 0),
			COALESCE(cached_input_tokens, 0), COALESCE(output_tokens, 0), COALESCE(cost, 0)
		FROM day_ai_models WHERE day >= ? AND day <= ?
		ORDER BY day, type, name, cost DESC, lines DESC
	`, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []DayAIModel
	for rows.Next() {
		var m DayAIModel
		var dayStr string
		if err := rows.Scan(&dayStr, &m.Type, &m.Name, &m.Model, &m.Lines, &m.InputTokens, &m.CachedInputTokens, &m.OutputTokens, &m.Cost); err != nil {
			return nil, err
		}
		if m.Day, err = parseDay(dayStr); err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, rows.Err()
}

// AIModelUsage is the usage of an AI model over a date range.
type AIModelUsage struct {
	Model             string  `json:"name"`
	Lines             int64   `json:"lines"`
	InputTokens       int64   `json:"input_tokens"`
	CachedInputTokens int64   `json:"cached_input_tokens"`
	OutputTokens      int64   `json:"output_tokens"`
	Cost              float64 `json:"cost"`
}

// GetAIModelUsage returns the total usage of each AI model from start to end, most expensive first.
func (db *DB) GetAIModelUsage(start, end time.Time) ([]AIModelUsage, error) {
	rows, err := db.Query(`
		SELECT model, COALESCE(SUM(lines), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(cached_input_tokens), 0),
			COALESCE(SUM(output_tokens), 0), COALESCE(SUM(cost), 0) AS total_cost
		FROM day_ai_models WHERE day >= ? AND day <= ? AND type = 'total'
		GROUP BY model ORDER BY total_cost DESC, SUM(lines) DESC, model
	`, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var models []AIModelUsage
	for rows.Next() {
		var m AIModelUsage
		if err := rows.Scan(&m.Model, &m.Lines, &m.InputTokens, &m.CachedInputTokens, &m.OutputTokens, &m.Cost); err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	return models, rows.Err()
}

// GetAIModelNames returns every AI model ever used, most expensive first. Ordering colors by it
// rather than by the usage in a date range lets each model keep its color across date ranges.
func (db *DB) GetAIModelNames() ([]string, error) {
	rows, err := db.Query(`
		SELECT model FROM day_ai_models WHERE type = 'total'
		GROUP BY model ORDER BY SUM(cost) DESC, SUM(lines) DESC, model
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// AIModelVersionUsage is the usage of an AI model version over a date range, from heartbeats.
type AIModelVersionUsage struct {
	Model             string `json:"model"`
	Version           string `json:"version"`
	Complexity        string `json:"complexity"`
	InputTokens       int64  `json:"input_tokens"`
	CachedInputTokens int64  `json:"cached_input_tokens"`
	OutputTokens      int64  `json:"output_tokens"`
	Sessions          int64  `json:"sessions"`
	Heartbeats        int64  `json:"heartbeats"`
}

// GetAIModelVersions returns the token usage of each AI model version from start to end, most used
// first. Versions are only known from the user agents of heartbeats; WakaTime does not attribute
// lines or costs to them.
func (db *DB) GetAIModelVersions(start, end time.Time) ([]AIModelVersionUsage, error) {
	rows, err := db.Query(`
		SELECT u.ai_model, COALESCE(u.ai_model_version, ''), COALESCE(u.ai_model_complexity, ''),
			COALESCE(SUM(h.ai_input_tokens), 0), COALESCE(SUM(h.ai_cached_input_tokens), 0), COALESCE(SUM(h.ai_output_tokens), 0),
			COUNT(DISTINCT NULLIF(h.ai_session, '')), COUNT(*)
		FROM heartbeats h JOIN user_agents u ON u.id = h.user_agent_id
		WHERE h.day >= ? AND h.day <= ? AND COALESCE(u.ai_model, '') != ''
		GROUP BY 1, 2, 3
		ORDER BY SUM(h.ai_input_tokens + h.ai_cached_input_tokens + h.ai_output_tokens) DESC, COUNT(*) DESC
	`, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []AIModelVersionUsage
	for rows.Next() {
		var v AIModelVersionUsage
		if err := rows.Scan(&v.Model, &v.Version, &v.Complexity, &v.InputTokens, &v.CachedInputTokens, &v.OutputTokens, &v.Sessions, &v.Heartbeats); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// AITotals is the AI coding activity over a date range.
type AITotals struct {
	AIStats
	// AICodingSeconds is the time spent in the AI Coding category
	AICodingSeconds float64 `json:"ai_coding_seconds"`
	// ActiveDays is the number of days with AI coding activity
	ActiveDays int64 `json:"active_days"`
}

// GetAITotals returns the AI coding activity from start to end. Sessions are counted once even when
// they span several days, except on days whose heartbeats were synced before AI stats were.
func (db *DB) GetAITotals(start, end time.Time) (*AITotals, error) {
	startStr, endStr := start.Format("2006-01-02"), end.Format("2006-01-02")
	var t AITotals

	if err := db.QueryRow(`
		SELECT `+aiSums("")+`,
			COUNT(CASE WHEN COALESCE(ai_additions + ai_deletions + ai_input_tokens + ai_output_tokens + ai_sessions, 0) > 0 THEN 1 END)
		FROM day_summaries WHERE day >= ? AND day <= ?
	`, startStr, endStr).Scan(append(t.sumPointers(), &t.ActiveDays)...); err != nil {
		return nil, err
	}

	if err := db.QueryRow(`
		SELECT
			(SELECT COUNT(DISTINCT ai_session) FROM heartbeats
				WHERE day >= ? AND day <= ? AND COALESCE(ai_session, '') != '')
			+ (SELECT COALESCE(SUM(s.ai_sessions), 0) FROM day_summaries s
				WHERE s.day >= ? AND s.day <= ? AND NOT EXISTS (
					SELECT 1 FROM heartbeats h WHERE h.day = s.day AND COALESCE(h.ai_session, '') != ''))
	`, startStr, endStr, startStr, endStr).Scan(&t.AISessions); err != nil {
		return nil, err
	}

	if err := db.QueryRow(`
		SELECT COALESCE(SUM(total_seconds), 0) FROM day_stats
		WHERE day >= ? AND day <= ? AND type = 'category' AND LOWER(name) = 'ai coding'
	`, startStr, endStr).Scan(&t.AICodingSeconds); err != nil {
		return nil, err
	}

	if t.AIPromptEventsTotal > 0 {
		t.AIPromptLengthAvg = float64(t.AIPromptLengthSum) / float64(t.AIPromptEventsTotal)
	}
	return &t, nil
}

// --- Yearly Activity operations (for GitHub-style heatmap) ---

// GetAvailableYears returns distinct years that have data in day_summaries
func (db *DB) GetAvailableYears() ([]int, error) {
	rows, err := db.Query(`
		SELECT DISTINCT CAST(strftime('%Y', day) AS INTEGER) as year
		FROM day_summaries
		ORDER BY year DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var years []int
	for rows.Next() {
		var year int
		if err := rows.Scan(&year); err != nil {
			return nil, err
		}
		years = append(years, year)
	}
	return years, rows.Err()
}

// YearlyActivityDay represents activity data for a single day
type YearlyActivityDay struct {
	Date         string             `json:"date"`
	TotalSeconds float64            `json:"total_seconds"`
	Projects     []ProjectBreakdown `json:"projects,omitempty"`
}

type ProjectBreakdown struct {
	Name         string  `json:"name"`
	TotalSeconds float64 `json:"total_seconds"`
}

// GetYearlyActivity returns daily totals and project breakdown for an entire year
func (db *DB) GetYearlyActivity(year int) ([]YearlyActivityDay, error) {
	startDate := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	endDate := time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC).Format("2006-01-02")

	rows, err := db.Query(`
		SELECT day, total_seconds
		FROM day_summaries
		WHERE day >= ? AND day <= ?
		ORDER BY day
	`, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []YearlyActivityDay
	index := make(map[string]int)
	for rows.Next() {
		var dayStr string
		var totalSeconds float64
		if err := rows.Scan(&dayStr, &totalSeconds); err != nil {
			return nil, err
		}
		day, err := parseDay(dayStr)
		if err != nil {
			return nil, err
		}
		date := day.Format("2006-01-02")
		index[date] = len(result)
		result = append(result, YearlyActivityDay{Date: date, TotalSeconds: totalSeconds, Projects: []ProjectBreakdown{}})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Get project breakdown for each day
	projectRows, err := db.Query(`
		SELECT day, name, total_seconds
		FROM day_stats
		WHERE day >= ? AND day <= ? AND type = 'project'
		ORDER BY day, total_seconds DESC
	`, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer projectRows.Close()

	for projectRows.Next() {
		var dayStr, name string
		var totalSeconds float64
		if err := projectRows.Scan(&dayStr, &name, &totalSeconds); err != nil {
			return nil, err
		}
		day, err := parseDay(dayStr)
		if err != nil {
			return nil, err
		}
		if i, ok := index[day.Format("2006-01-02")]; ok {
			result[i].Projects = append(result[i].Projects, ProjectBreakdown{Name: name, TotalSeconds: totalSeconds})
		}
	}

	return result, projectRows.Err()
}

// --- Dashboard operations ---

// DayActivity is the coding activity of a day, with its AI coding metrics.
type DayActivity struct {
	Date            string  `json:"date"`
	TotalSeconds    float64 `json:"total_seconds"`
	AICodingSeconds float64 `json:"ai_coding_seconds"`
	AILines         int64   `json:"ai_lines"`
	HumanLines      int64   `json:"human_lines"`
	AITokens        int64   `json:"ai_tokens"`
	AICost          float64 `json:"ai_cost"`
	// ModelCosts is the estimated cost of each AI model used that day
	ModelCosts map[string]float64 `json:"model_costs"`
}

// GetDailyActivity returns the activity of every day from start to end, including days without any.
func (db *DB) GetDailyActivity(start, end time.Time) ([]DayActivity, error) {
	startStr, endStr := start.Format("2006-01-02"), end.Format("2006-01-02")

	var days []DayActivity
	index := make(map[string]int)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		date := d.Format("2006-01-02")
		index[date] = len(days)
		days = append(days, DayActivity{Date: date, ModelCosts: map[string]float64{}})
	}

	// dayOf returns the day of a row, which SQLite may return as a full timestamp.
	dayOf := func(s string) (*DayActivity, error) {
		day, err := parseDay(s)
		if err != nil {
			return nil, err
		}
		if i, ok := index[day.Format("2006-01-02")]; ok {
			return &days[i], nil
		}
		return nil, nil
	}

	rows, err := db.Query(`
		SELECT day, total_seconds,
			COALESCE(ai_additions + ai_deletions, 0), COALESCE(human_additions + human_deletions, 0),
			COALESCE(ai_input_tokens + ai_cached_input_tokens + ai_output_tokens, 0), COALESCE(ai_model_total_cost, 0)
		FROM day_summaries WHERE day >= ? AND day <= ?
	`, startStr, endStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var dayStr string
		var a DayActivity
		if err := rows.Scan(&dayStr, &a.TotalSeconds, &a.AILines, &a.HumanLines, &a.AITokens, &a.AICost); err != nil {
			return nil, err
		}
		day, err := dayOf(dayStr)
		if err != nil {
			return nil, err
		}
		if day != nil {
			a.Date, a.ModelCosts = day.Date, day.ModelCosts
			*day = a
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	categoryRows, err := db.Query(`
		SELECT day, SUM(total_seconds) FROM day_stats
		WHERE day >= ? AND day <= ? AND type = 'category' AND LOWER(name) = 'ai coding'
		GROUP BY day
	`, startStr, endStr)
	if err != nil {
		return nil, err
	}
	defer categoryRows.Close()
	for categoryRows.Next() {
		var dayStr string
		var seconds float64
		if err := categoryRows.Scan(&dayStr, &seconds); err != nil {
			return nil, err
		}
		day, err := dayOf(dayStr)
		if err != nil {
			return nil, err
		}
		if day != nil {
			day.AICodingSeconds = seconds
		}
	}
	if err := categoryRows.Err(); err != nil {
		return nil, err
	}

	modelRows, err := db.Query(`
		SELECT day, model, SUM(cost) FROM day_ai_models
		WHERE day >= ? AND day <= ? AND type = 'total' AND cost > 0
		GROUP BY day, model
	`, startStr, endStr)
	if err != nil {
		return nil, err
	}
	defer modelRows.Close()
	for modelRows.Next() {
		var dayStr, model string
		var cost float64
		if err := modelRows.Scan(&dayStr, &model, &cost); err != nil {
			return nil, err
		}
		day, err := dayOf(dayStr)
		if err != nil {
			return nil, err
		}
		if day != nil {
			day.ModelCosts[model] = cost
		}
	}
	return days, modelRows.Err()
}

// GetRangeTotals returns the total time from start to end, and the number of days with any.
func (db *DB) GetRangeTotals(start, end time.Time) (totalSeconds float64, activeDays int64, err error) {
	err = db.QueryRow(`
		SELECT COALESCE(SUM(total_seconds), 0), COUNT(CASE WHEN total_seconds > 0 THEN 1 END)
		FROM day_summaries WHERE day >= ? AND day <= ?
	`, start.Format("2006-01-02"), end.Format("2006-01-02")).Scan(&totalSeconds, &activeDays)
	return totalSeconds, activeDays, err
}

// DurationSpan is a span of coding time, from a stored duration.
type DurationSpan struct {
	Day       time.Time
	StartTime float64 // UNIX timestamp
	Duration  float64 // seconds
	// AI is set when AI coding tools were used during the span
	AI bool
}

// GetDurationSpans returns the spans of coding time of every day from start to end, in order.
func (db *DB) GetDurationSpans(start, end time.Time) ([]DurationSpan, error) {
	rows, err := db.Query(`
		SELECT day, start_time, duration,
			COALESCE(ai_input_tokens + ai_cached_input_tokens + ai_output_tokens + ai_additions + ai_deletions, 0) > 0
		FROM durations WHERE day >= ? AND day <= ?
		ORDER BY day, start_time
	`, start.Format("2006-01-02"), end.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var spans []DurationSpan
	for rows.Next() {
		var dayStr string
		var span DurationSpan
		var ai int
		if err := rows.Scan(&dayStr, &span.StartTime, &span.Duration, &ai); err != nil {
			return nil, err
		}
		if span.Day, err = parseDay(dayStr); err != nil {
			return nil, err
		}
		span.AI = ai != 0
		spans = append(spans, span)
	}
	return spans, rows.Err()
}

// --- Day sync operations ---

// DayData holds everything synced from WakaTime for a single day.
type DayData struct {
	Day time.Time
	// Summary is nil when WakaTime returned no summary for the day.
	Summary          *DaySummary
	Stats            []DayStats
	AIModels         []DayAIModel
	Durations        []Duration
	ProjectDurations []ProjectDuration
	Heartbeats       []HeartBeat
}

// SaveDayResult describes the outcome of SaveDay.
type SaveDayResult struct {
	// Kept lists the parts whose stored data was kept because the new data was smaller.
	Kept []string
	// TotalSeconds is the day's total as stored after saving.
	TotalSeconds float64
}

// shrinkTolerance absorbs floating point noise when comparing amounts of time in seconds.
const shrinkTolerance = 1.0

// dayPart is one kind of data stored per day.
type dayPart struct {
	name string
	// sizeQuery returns the size of the stored data for a day, to compare with newSize. The first
	// value is the main measure (time or count); any following ones only break ties.
	sizeQuery string
	newSize   []float64
	tolerance float64
	replace   func(tx *sql.Tx, day string) error
}

// SaveDay replaces the stored data for data.Day and records a successful sync, in a single
// transaction so that a failure never leaves the day half-written.
//
// Unless force is set, each part (summary, durations, project durations, heartbeats) is only
// replaced when the new data is not smaller than the stored data, i.e. less time or fewer
// heartbeats, or as much of them but fewer line changes or AI tokens. A re-sync can thus never
// shrink the archive, e.g. when WakaTime stops serving old data.
func (db *DB) SaveDay(data *DayData, force bool) (*SaveDayResult, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	day := data.Day.Format("2006-01-02")
	var parts []dayPart

	// Stored sizes are the time (or count), line changes and AI tokens
	const sizeColumns = `
		COALESCE(SUM(ai_additions + ai_deletions + human_additions + human_deletions), 0),
		COALESCE(SUM(ai_input_tokens + ai_cached_input_tokens + ai_output_tokens), 0)`

	if data.Summary != nil {
		parts = append(parts, dayPart{
			name:      "summary",
			sizeQuery: "SELECT COALESCE(SUM(total_seconds), 0)," + sizeColumns + " FROM day_summaries WHERE day = ?",
			newSize:   []float64{data.Summary.TotalSeconds, float64(data.Summary.LineChanges()), float64(data.Summary.Tokens())},
			tolerance: shrinkTolerance,
			replace: func(tx *sql.Tx, day string) error {
				return replaceDaySummary(tx, day, data.Summary, data.Stats, data.AIModels)
			},
		})
	}

	durationsSize := make([]float64, 3)
	for _, d := range data.Durations {
		durationsSize[0] += d.Duration
		durationsSize[1] += float64(d.LineChanges())
		durationsSize[2] += float64(d.Tokens())
	}
	projectDurationsSize := make([]float64, 3)
	for _, d := range data.ProjectDurations {
		projectDurationsSize[0] += d.Duration
		projectDurationsSize[1] += float64(d.LineChanges())
		projectDurationsSize[2] += float64(d.Tokens())
	}
	heartbeatsSize := []float64{float64(len(data.Heartbeats)), 0}
	for _, h := range data.Heartbeats {
		heartbeatsSize[1] += float64(h.AIInputTokens + h.AICachedInputTokens + h.AIOutputTokens)
	}

	parts = append(parts,
		dayPart{
			name:      "durations",
			sizeQuery: "SELECT COALESCE(SUM(duration), 0)," + sizeColumns + " FROM durations WHERE day = ?",
			newSize:   durationsSize,
			tolerance: shrinkTolerance,
			replace: func(tx *sql.Tx, day string) error {
				return replaceDurations(tx, day, data.Durations)
			},
		},
		dayPart{
			name:      "project_durations",
			sizeQuery: "SELECT COALESCE(SUM(duration), 0)," + sizeColumns + " FROM project_durations WHERE day = ?",
			newSize:   projectDurationsSize,
			tolerance: shrinkTolerance,
			replace: func(tx *sql.Tx, day string) error {
				return replaceProjectDurations(tx, day, data.ProjectDurations)
			},
		},
		dayPart{
			name: "heartbeats",
			sizeQuery: `SELECT COUNT(*), COALESCE(SUM(ai_input_tokens + ai_cached_input_tokens + ai_output_tokens), 0)
				FROM heartbeats WHERE day = ?`,
			newSize: heartbeatsSize,
			replace: func(tx *sql.Tx, day string) error {
				return replaceHeartbeats(tx, day, data.Heartbeats)
			},
		},
	)

	result := &SaveDayResult{}
	for _, part := range parts {
		if !force {
			storedSize := make([]float64, len(part.newSize))
			dest := make([]interface{}, len(storedSize))
			for i := range storedSize {
				dest[i] = &storedSize[i]
			}
			if err := tx.QueryRow(part.sizeQuery, day).Scan(dest...); err != nil {
				return nil, err
			}
			if shrinks(part.newSize, storedSize, part.tolerance) {
				result.Kept = append(result.Kept, part.name)
				continue
			}
		}
		if err := part.replace(tx, day); err != nil {
			return nil, err
		}
	}

	if err := tx.QueryRow("SELECT COALESCE(SUM(total_seconds), 0) FROM day_summaries WHERE day = ?", day).Scan(&result.TotalSeconds); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(`
		INSERT INTO sync_log (day, synced_at, total_seconds, status)
		VALUES (?, ?, ?, 'success')
		ON CONFLICT(day) DO UPDATE SET synced_at = excluded.synced_at, total_seconds = excluded.total_seconds, status = excluded.status
	`, day, time.Now(), result.TotalSeconds); err != nil {
		return nil, err
	}

	return result, tx.Commit()
}

// shrinks reports whether newSize is smaller than storedSize. Values are compared in order, each one
// only mattering when all previous ones are equal; tolerance applies to the first value.
func shrinks(newSize, storedSize []float64, tolerance float64) bool {
	for i := range newSize {
		tol := 0.0
		if i == 0 {
			tol = tolerance
		}
		if newSize[i] < storedSize[i]-tol {
			return true
		}
		if newSize[i] > storedSize[i]+tol {
			return false
		}
	}
	return false
}

func replaceDaySummary(tx *sql.Tx, day string, summary *DaySummary, stats []DayStats, models []DayAIModel) error {
	aiNames := strings.Join(aiColumnNames(""), ", ")
	aiUpdates := strings.Join(aiColumnNames("excluded."), ", ")
	if _, err := tx.Exec(`
		INSERT INTO day_summaries (day, total_seconds, created_at, `+aiNames+`)
		VALUES (?, ?, ?`+placeholders(len(aiColumns))+`)
		ON CONFLICT(day) DO UPDATE SET (total_seconds, `+aiNames+`) = (excluded.total_seconds, `+aiUpdates+`)
	`, append([]interface{}{day, summary.TotalSeconds, time.Now()}, summary.values()...)...); err != nil {
		return err
	}

	if _, err := tx.Exec("DELETE FROM day_stats WHERE day = ?", day); err != nil {
		return err
	}

	// Items can share a name, e.g. machines with the same host name, so their time adds up
	stmt, err := tx.Prepare(`
		INSERT INTO day_stats (day, type, name, total_seconds, created_at, ` + aiNames + `)
		VALUES (?, ?, ?, ?, ?` + placeholders(len(aiColumns)) + `)
		ON CONFLICT(day, type, name) DO UPDATE SET total_seconds = day_stats.total_seconds + excluded.total_seconds
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, s := range stats {
		if _, err := stmt.Exec(append([]interface{}{day, s.Type, s.Name, s.TotalSeconds, time.Now()}, s.values()...)...); err != nil {
			return err
		}
	}

	if _, err := tx.Exec("DELETE FROM day_ai_models WHERE day = ?", day); err != nil {
		return err
	}

	modelStmt, err := tx.Prepare(`
		INSERT INTO day_ai_models (day, type, name, model, lines, input_tokens, cached_input_tokens, output_tokens, cost, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(day, type, name, model) DO UPDATE SET
			lines = day_ai_models.lines + excluded.lines,
			input_tokens = day_ai_models.input_tokens + excluded.input_tokens,
			cached_input_tokens = day_ai_models.cached_input_tokens + excluded.cached_input_tokens,
			output_tokens = day_ai_models.output_tokens + excluded.output_tokens,
			cost = day_ai_models.cost + excluded.cost
	`)
	if err != nil {
		return err
	}
	defer modelStmt.Close()

	for _, m := range models {
		if _, err := modelStmt.Exec(day, m.Type, m.Name, m.Model, m.Lines, m.InputTokens, m.CachedInputTokens, m.OutputTokens, m.Cost, time.Now()); err != nil {
			return err
		}
	}
	return nil
}

func replaceDurations(tx *sql.Tx, day string, durations []Duration) error {
	if _, err := tx.Exec("DELETE FROM durations WHERE day = ?", day); err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO durations (day, project, start_time, duration, dependencies, ai_model_breakdown, created_at, ` + strings.Join(aiColumnNames(""), ", ") + `)
		VALUES (?, ?, ?, ?, ` + jsonbValue + `, ` + jsonbValue + `, ?` + placeholders(len(aiColumns)) + `)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, d := range durations {
		args := []interface{}{day, d.Project, d.StartTime, d.Duration}
		args = append(args, jsonbArgs(d.Dependencies)...)
		args = append(args, jsonbArgs(d.AIModelBreakdown)...)
		args = append(args, time.Now())
		if _, err := stmt.Exec(append(args, d.values()...)...); err != nil {
			return err
		}
	}
	return nil
}

func replaceProjectDurations(tx *sql.Tx, day string, durations []ProjectDuration) error {
	if _, err := tx.Exec("DELETE FROM project_durations WHERE day = ?", day); err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO project_durations (day, project, branch, entity, language, type, start_time, duration, dependencies, ai_model_breakdown, created_at, ` + strings.Join(aiColumnNames(""), ", ") + `)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ` + jsonbValue + `, ` + jsonbValue + `, ?` + placeholders(len(aiColumns)) + `)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, d := range durations {
		args := []interface{}{day, d.Project, d.Branch, d.Entity, d.Language, d.Type, d.StartTime, d.Duration}
		args = append(args, jsonbArgs(d.Dependencies)...)
		args = append(args, jsonbArgs(d.AIModelBreakdown)...)
		args = append(args, time.Now())
		if _, err := stmt.Exec(append(args, d.values()...)...); err != nil {
			return err
		}
	}
	return nil
}

func replaceHeartbeats(tx *sql.Tx, day string, heartbeats []HeartBeat) error {
	if _, err := tx.Exec("DELETE FROM heartbeats WHERE day = ?", day); err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO heartbeats (
			day, entity, type, category, time, project, branch, language, is_write, machine_id, lines, line_no, cursor_pos, created_at,
			wakatime_id, user_agent_id, project_root_count, dependencies, ai_line_changes, human_line_changes, ai_session,
			ai_input_tokens, ai_cached_input_tokens, ai_output_tokens, ai_prompt_length, ai_subscription_plan
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ` + jsonbValue + `, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, h := range heartbeats {
		isWrite := 0
		if h.IsWrite {
			isWrite = 1
		}
		args := []interface{}{
			day, h.Entity, h.Type, h.Category, h.Time, h.Project, h.Branch, h.Language,
			isWrite, h.MachineID, h.Lines, h.LineNo, h.CursorPos, time.Now(),
			h.WakaTimeID, h.UserAgentID, h.ProjectRootCount,
		}
		args = append(args, jsonbArgs(h.Dependencies)...)
		args = append(args,
			h.AILineChanges, h.HumanLineChanges, h.AISession,
			h.AIInputTokens, h.AICachedInputTokens, h.AIOutputTokens, h.AIPromptLength, h.AISubscriptionPlan,
		)
		if _, err := stmt.Exec(args...); err != nil {
			return err
		}
	}
	return nil
}

// jsonbValue stores a JSON text argument as JSONB, or NULL when it is empty; see jsonbArgs.
const jsonbValue = "CASE WHEN ? = '' OR ? IS NULL THEN NULL ELSE jsonb(?) END"

// jsonbArgs returns the arguments of jsonbValue.
func jsonbArgs(json string) []interface{} {
	return []interface{}{json, json, json}
}

// placeholders returns n query placeholders, each preceded by a comma.
func placeholders(n int) string {
	return strings.Repeat(", ?", n)
}

// parseDay parses a day as returned by SQLite, either a date or a full RFC 3339 timestamp.
func parseDay(s string) (time.Time, error) {
	if len(s) > 10 {
		s = s[:10]
	}
	return time.Parse("2006-01-02", s)
}

// --- Sync Log operations ---

// RecordSyncFailure marks the day's last sync as failed, keeping the data stored by earlier syncs.
func (db *DB) RecordSyncFailure(day time.Time) error {
	_, err := db.Exec(`
		INSERT INTO sync_log (day, synced_at, status)
		VALUES (?, ?, 'failed')
		ON CONFLICT(day) DO UPDATE SET synced_at = excluded.synced_at, status = excluded.status
	`, day.Format("2006-01-02"), time.Now())
	return err
}

// GetFailedSyncDays returns the days whose last sync failed, oldest first.
func (db *DB) GetFailedSyncDays() ([]time.Time, error) {
	rows, err := db.Query("SELECT day FROM sync_log WHERE status != 'success' ORDER BY day")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var days []time.Time
	for rows.Next() {
		var dayStr string
		if err := rows.Scan(&dayStr); err != nil {
			return nil, err
		}
		day, err := parseDay(dayStr)
		if err != nil {
			return nil, err
		}
		days = append(days, day)
	}
	return days, rows.Err()
}

func (db *DB) GetLastSyncedDay() (time.Time, error) {
	var dayStr string
	err := db.QueryRow("SELECT day FROM sync_log WHERE status = 'success' ORDER BY day DESC LIMIT 1").Scan(&dayStr)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	// Try parsing as date-only first, then as full timestamp
	t, err := time.Parse("2006-01-02", dayStr)
	if err != nil {
		// SQLite might return full ISO format
		t, err = time.Parse(time.RFC3339, dayStr)
		if err != nil {
			// Try without timezone
			t, err = time.Parse("2006-01-02T15:04:05", dayStr)
		}
	}
	return t, err
}

func (db *DB) IsDaySynced(day time.Time) (bool, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sync_log WHERE day = ? AND status = 'success'", day.Format("2006-01-02")).Scan(&count)
	return count > 0, err
}

// Type aliases for external usage
// AIStats holds the AI coding metrics of a day, a day stats item or a duration.
type AIStats struct {
	AIAdditions                    int64   `json:"ai_additions"`
	AIDeletions                    int64   `json:"ai_deletions"`
	HumanAdditions                 int64   `json:"human_additions"`
	HumanDeletions                 int64   `json:"human_deletions"`
	AIInputTokens                  int64   `json:"ai_input_tokens"`
	AICachedInputTokens            int64   `json:"ai_cached_input_tokens"`
	AIOutputTokens                 int64   `json:"ai_output_tokens"`
	AIModelTotalCost               float64 `json:"ai_model_total_cost"`
	AISessions                     int64   `json:"ai_sessions"`
	AIPromptEventsTotal            int64   `json:"ai_prompt_events_total"`
	AIPromptLengthSum              int64   `json:"ai_prompt_length_sum"`
	AIPromptLengthAvg              float64 `json:"ai_prompt_length_avg"`
	AIPromptLengthAvgPerSession    float64 `json:"ai_prompt_length_avg_per_session"`
	AIPromptLengthMedianPerSession float64 `json:"ai_prompt_length_median_per_session"`
	AIPromptEventsAvgPerSession    float64 `json:"ai_prompt_events_avg_per_session"`
	AIPromptEventsMedianPerSession float64 `json:"ai_prompt_events_median_per_session"`
}

// values returns the metrics in the order of aiColumns.
func (a *AIStats) values() []interface{} {
	return []interface{}{
		a.AIAdditions, a.AIDeletions, a.HumanAdditions, a.HumanDeletions,
		a.AIInputTokens, a.AICachedInputTokens, a.AIOutputTokens, a.AIModelTotalCost,
		a.AISessions, a.AIPromptEventsTotal, a.AIPromptLengthSum, a.AIPromptLengthAvg,
		a.AIPromptLengthAvgPerSession, a.AIPromptLengthMedianPerSession,
		a.AIPromptEventsAvgPerSession, a.AIPromptEventsMedianPerSession,
	}
}

// pointers returns pointers to the metrics in the order of aiColumns, to scan them.
func (a *AIStats) pointers() []interface{} {
	return []interface{}{
		&a.AIAdditions, &a.AIDeletions, &a.HumanAdditions, &a.HumanDeletions,
		&a.AIInputTokens, &a.AICachedInputTokens, &a.AIOutputTokens, &a.AIModelTotalCost,
		&a.AISessions, &a.AIPromptEventsTotal, &a.AIPromptLengthSum, &a.AIPromptLengthAvg,
		&a.AIPromptLengthAvgPerSession, &a.AIPromptLengthMedianPerSession,
		&a.AIPromptEventsAvgPerSession, &a.AIPromptEventsMedianPerSession,
	}
}

// LineChanges returns the number of lines added or removed, by AI or by hand.
func (a *AIStats) LineChanges() int64 {
	return a.AIAdditions + a.AIDeletions + a.HumanAdditions + a.HumanDeletions
}

// Tokens returns the number of input, cached input and output tokens.
func (a *AIStats) Tokens() int64 {
	return a.AIInputTokens + a.AICachedInputTokens + a.AIOutputTokens
}

type Duration = struct {
	ID           int64     `json:"id"`
	Day          time.Time `json:"day"`
	Project      string    `json:"project"`
	StartTime    float64   `json:"time"`
	Duration     float64   `json:"duration"`
	Dependencies string    `json:"dependencies,omitempty"`
	AIStats
	// AIModelBreakdown is a JSON array of the AI models used, see DayAIModel
	AIModelBreakdown string    `json:"ai_model_breakdown,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type ProjectDuration = struct {
	ID           int64     `json:"id"`
	Day          time.Time `json:"day"`
	Project      string    `json:"project"`
	Branch       string    `json:"branch,omitempty"`
	Entity       string    `json:"entity,omitempty"`
	Language     string    `json:"language,omitempty"`
	Type         string    `json:"type,omitempty"`
	StartTime    float64   `json:"time"`
	Duration     float64   `json:"duration"`
	Dependencies string    `json:"dependencies,omitempty"`
	AIStats
	// AIModelBreakdown is a JSON array of the AI models used, see DayAIModel
	AIModelBreakdown string    `json:"ai_model_breakdown,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type HeartBeat = struct {
	ID                  int64     `json:"id"`
	WakaTimeID          string    `json:"wakatime_id,omitempty"`
	Day                 time.Time `json:"day"`
	Entity              string    `json:"entity"`
	Type                string    `json:"type"`
	Category            string    `json:"category,omitempty"`
	Time                float64   `json:"time"`
	Project             string    `json:"project,omitempty"`
	ProjectRootCount    int       `json:"project_root_count,omitempty"`
	Branch              string    `json:"branch,omitempty"`
	Language            string    `json:"language,omitempty"`
	Dependencies        string    `json:"dependencies,omitempty"`
	IsWrite             bool      `json:"is_write"`
	MachineID           string    `json:"machine_name_id,omitempty"`
	UserAgentID         string    `json:"user_agent_id,omitempty"`
	Lines               int       `json:"lines,omitempty"`
	LineNo              int       `json:"lineno,omitempty"`
	CursorPos           int       `json:"cursorpos,omitempty"`
	AILineChanges       int       `json:"ai_line_changes,omitempty"`
	HumanLineChanges    int       `json:"human_line_changes,omitempty"`
	AISession           string    `json:"ai_session,omitempty"`
	AIInputTokens       int64     `json:"ai_input_tokens,omitempty"`
	AICachedInputTokens int64     `json:"ai_cached_input_tokens,omitempty"`
	AIOutputTokens      int64     `json:"ai_output_tokens,omitempty"`
	AIPromptLength      int       `json:"ai_prompt_length,omitempty"`
	AISubscriptionPlan  string    `json:"ai_subscription_plan,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

type Project = struct {
	ID               int64     `json:"id"`
	UUID             string    `json:"uuid"`
	Name             string    `json:"name"`
	Repository       string    `json:"repository,omitempty"`
	Badge            string    `json:"badge,omitempty"`
	Color            string    `json:"color,omitempty"`
	HasPublicURL     bool      `json:"has_public_url"`
	LastHeartbeatAt  time.Time `json:"last_heartbeat_at,omitempty"`
	FirstHeartbeatAt time.Time `json:"first_heartbeat_at,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type DaySummary = struct {
	ID           int64     `json:"id"`
	Day          time.Time `json:"day"`
	TotalSeconds float64   `json:"total_seconds"`
	AIStats
	CreatedAt time.Time `json:"created_at"`
}

type DayStats = struct {
	ID           int64     `json:"id"`
	Day          time.Time `json:"day"`
	Type         string    `json:"type"`
	Name         string    `json:"name"`
	TotalSeconds float64   `json:"total_seconds"`
	AIStats
	CreatedAt time.Time `json:"created_at"`
}

// DayAIModel is the usage of an AI model on a day, in total or for a project or editor.
type DayAIModel struct {
	Day time.Time `json:"day"`
	// Type is total, project or editor
	Type string `json:"type"`
	// Name is the project or editor name, empty for the total
	Name              string  `json:"name"`
	Model             string  `json:"model"`
	Lines             int64   `json:"lines"`
	InputTokens       int64   `json:"input_tokens"`
	CachedInputTokens int64   `json:"cached_input_tokens"`
	OutputTokens      int64   `json:"output_tokens"`
	Cost              float64 `json:"cost"`
}

// UserAgent is a plugin which sent heartbeats, with the AI model it reported.
type UserAgent struct {
	ID                 string `json:"id"`
	Value              string `json:"value"`
	Editor             string `json:"editor"`
	OS                 string `json:"os"`
	Version            string `json:"version"`
	AIModel            string `json:"ai_model"`
	AIModelVersion     string `json:"ai_model_version"`
	AIModelComplexity  string `json:"ai_model_complexity"`
	IsBrowserExtension bool   `json:"is_browser_extension"`
	IsDesktopApp       bool   `json:"is_desktop_app"`
	FirstSeenAt        string `json:"first_seen_at"`
	LastSeenAt         string `json:"last_seen_at"`
}
