package database

import (
	"database/sql"
	"log/slog"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func New(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
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
	migrations := []string{
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

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return err
		}
	}

	return nil
}

// --- Duration operations ---

func (db *DB) DeleteDurationsByDay(day time.Time) error {
	_, err := db.Exec("DELETE FROM durations WHERE day = ?", day.Format("2006-01-02"))
	return err
}

func (db *DB) InsertDuration(d *Duration) error {
	_, err := db.Exec(`
		INSERT INTO durations (day, project, start_time, duration, dependencies, created_at)
		VALUES (?, ?, ?, ?, CASE WHEN ? = '' OR ? IS NULL THEN NULL ELSE jsonb(?) END, ?)
	`, d.Day.Format("2006-01-02"), d.Project, d.StartTime, d.Duration, d.Dependencies, d.Dependencies, d.Dependencies, time.Now())
	return err
}

func (db *DB) InsertDurations(durations []Duration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO durations (day, project, start_time, duration, dependencies, created_at)
		VALUES (?, ?, ?, ?, CASE WHEN ? = '' OR ? IS NULL THEN NULL ELSE jsonb(?) END, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, d := range durations {
		_, err := stmt.Exec(d.Day.Format("2006-01-02"), d.Project, d.StartTime, d.Duration, d.Dependencies, d.Dependencies, d.Dependencies, time.Now())
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) GetDurationsByDay(day time.Time) ([]Duration, error) {
	rows, err := db.Query(`
		SELECT id, day, project, start_time, duration, dependencies, created_at
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
		if err := rows.Scan(&d.ID, &dayStr, &d.Project, &d.StartTime, &d.Duration, &d.Dependencies, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.Day, _ = time.Parse("2006-01-02", dayStr)
		durations = append(durations, d)
	}
	return durations, rows.Err()
}

func (db *DB) CountDurationsByDay(day time.Time) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM durations WHERE day = ?", day.Format("2006-01-02")).Scan(&count)
	return count, err
}

// --- Project Duration operations ---

func (db *DB) DeleteProjectDurationsByDay(day time.Time) error {
	_, err := db.Exec("DELETE FROM project_durations WHERE day = ?", day.Format("2006-01-02"))
	return err
}

func (db *DB) InsertProjectDurations(durations []ProjectDuration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO project_durations (day, project, branch, entity, language, type, start_time, duration, dependencies, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, CASE WHEN ? = '' OR ? IS NULL THEN NULL ELSE jsonb(?) END, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, d := range durations {
		_, err := stmt.Exec(
			d.Day.Format("2006-01-02"), d.Project, d.Branch, d.Entity, d.Language,
			d.Type, d.StartTime, d.Duration, d.Dependencies, d.Dependencies, d.Dependencies, time.Now(),
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) GetProjectDurationsByDay(day time.Time, project string) ([]ProjectDuration, error) {
	query := `
		SELECT id, day, project, branch, entity, language, type, start_time, duration, dependencies, created_at
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
		if err := rows.Scan(&d.ID, &dayStr, &d.Project, &d.Branch, &d.Entity, &d.Language, &d.Type, &d.StartTime, &d.Duration, &d.Dependencies, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.Day, _ = time.Parse("2006-01-02", dayStr)
		durations = append(durations, d)
	}
	return durations, rows.Err()
}

// --- Heartbeat operations ---

func (db *DB) DeleteHeartbeatsByDay(day time.Time) error {
	_, err := db.Exec("DELETE FROM heartbeats WHERE day = ?", day.Format("2006-01-02"))
	return err
}

func (db *DB) InsertHeartbeats(heartbeats []HeartBeat) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO heartbeats (day, entity, type, category, time, project, branch, language, is_write, machine_id, lines, line_no, cursor_pos, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		_, err := stmt.Exec(
			h.Day.Format("2006-01-02"), h.Entity, h.Type, h.Category, h.Time, h.Project, h.Branch, h.Language,
			isWrite, h.MachineID, h.Lines, h.LineNo, h.CursorPos, time.Now(),
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) GetHeartbeatsByDay(day time.Time) ([]HeartBeat, error) {
	rows, err := db.Query(`
		SELECT id, day, entity, type, category, time, project, branch, language, is_write, machine_id, lines, line_no, cursor_pos, created_at
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
		if err := rows.Scan(&h.ID, &dayStr, &h.Entity, &h.Type, &h.Category, &h.Time, &h.Project, &h.Branch, &h.Language, &isWrite, &h.MachineID, &h.Lines, &h.LineNo, &h.CursorPos, &h.CreatedAt); err != nil {
			return nil, err
		}
		h.Day, _ = time.Parse("2006-01-02", dayStr)
		h.IsWrite = isWrite == 1
		heartbeats = append(heartbeats, h)
	}
	return heartbeats, rows.Err()
}

func (db *DB) CountHeartbeatsByDay(day time.Time) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM heartbeats WHERE day = ?", day.Format("2006-01-02")).Scan(&count)
	return count, err
}

// --- Project operations ---

func (db *DB) UpsertProject(p *Project) error {
	_, err := db.Exec(`
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
	`, p.UUID, p.Name, p.Repository, p.Badge, p.Color, p.HasPublicURL, p.LastHeartbeatAt, p.FirstHeartbeatAt, time.Now())
	return err
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

func (db *DB) UpsertDaySummary(day time.Time, totalSeconds float64) error {
	_, err := db.Exec(`
		INSERT INTO day_summaries (day, total_seconds, created_at)
		VALUES (?, ?, ?)
		ON CONFLICT(day) DO UPDATE SET total_seconds = excluded.total_seconds
	`, day.Format("2006-01-02"), totalSeconds, time.Now())
	return err
}

func (db *DB) GetDaySummary(day time.Time) (*DaySummary, error) {
	var s DaySummary
	var dayStr string
	err := db.QueryRow(`
		SELECT id, day, total_seconds, created_at
		FROM day_summaries WHERE day = ?
	`, day.Format("2006-01-02")).Scan(&s.ID, &dayStr, &s.TotalSeconds, &s.CreatedAt)
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

func (db *DB) DeleteDayStatsByDay(day time.Time) error {
	_, err := db.Exec("DELETE FROM day_stats WHERE day = ?", day.Format("2006-01-02"))
	return err
}

func (db *DB) InsertDayStats(stats []DayStats) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO day_stats (day, type, name, total_seconds, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(day, type, name) DO UPDATE SET total_seconds = excluded.total_seconds
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, s := range stats {
		_, err := stmt.Exec(s.Day.Format("2006-01-02"), s.Type, s.Name, s.TotalSeconds, time.Now())
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) GetDayStatsByDayAndType(day time.Time, statType string) ([]DayStats, error) {
	rows, err := db.Query(`
		SELECT id, day, type, name, total_seconds, created_at
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
		if err := rows.Scan(&s.ID, &dayStr, &s.Type, &s.Name, &s.TotalSeconds, &s.CreatedAt); err != nil {
			return nil, err
		}
		s.Day, _ = time.Parse("2006-01-02", dayStr)
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

func (db *DB) GetAggregatedStats(start, end time.Time, statType string) ([]struct {
	Name         string  `json:"name"`
	TotalSeconds float64 `json:"total_seconds"`
}, error) {
	rows, err := db.Query(`
		SELECT name, SUM(total_seconds) as total
		FROM day_stats WHERE day >= ? AND day <= ? AND type = ?
		GROUP BY name ORDER BY total DESC
	`, start.Format("2006-01-02"), end.Format("2006-01-02"), statType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []struct {
		Name         string  `json:"name"`
		TotalSeconds float64 `json:"total_seconds"`
	}
	for rows.Next() {
		var s struct {
			Name         string  `json:"name"`
			TotalSeconds float64 `json:"total_seconds"`
		}
		if err := rows.Scan(&s.Name, &s.TotalSeconds); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
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
	// First get all day summaries for the year
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

	dayMap := make(map[string]*YearlyActivityDay)
	for rows.Next() {
		var day string
		var totalSeconds float64
		if err := rows.Scan(&day, &totalSeconds); err != nil {
			return nil, err
		}
		// Normalize date to YYYY-MM-DD format (handle possible RFC3339 format from SQLite)
		normalizedDay := day
		if len(day) > 10 {
			normalizedDay = day[:10]
		}
		dayMap[normalizedDay] = &YearlyActivityDay{
			Date:         normalizedDay,
			TotalSeconds: totalSeconds,
			Projects:     []ProjectBreakdown{},
		}
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
		var day, name string
		var totalSeconds float64
		if err := projectRows.Scan(&day, &name, &totalSeconds); err != nil {
			return nil, err
		}
		// Normalize date to YYYY-MM-DD format
		normalizedDay := day
		if len(day) > 10 {
			normalizedDay = day[:10]
		}
		if dayData, exists := dayMap[normalizedDay]; exists {
			dayData.Projects = append(dayData.Projects, ProjectBreakdown{
				Name:         name,
				TotalSeconds: totalSeconds,
			})
		}
	}

	// Convert map to slice, ordered by date
	var result []YearlyActivityDay
	for _, v := range dayMap {
		result = append(result, *v)
	}

	// Sort by date
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].Date > result[j].Date {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result, nil
}

// --- Sync Log operations ---

func (db *DB) RecordSync(day time.Time, totalSeconds float64, status string) error {
	_, err := db.Exec(`
		INSERT INTO sync_log (day, synced_at, total_seconds, status)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(day) DO UPDATE SET synced_at = excluded.synced_at, total_seconds = excluded.total_seconds, status = excluded.status
	`, day.Format("2006-01-02"), time.Now(), totalSeconds, status)
	return err
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
type Duration = struct {
	ID           int64     `json:"id"`
	Day          time.Time `json:"day"`
	Project      string    `json:"project"`
	StartTime    float64   `json:"time"`
	Duration     float64   `json:"duration"`
	Dependencies string    `json:"dependencies,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
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
	CreatedAt    time.Time `json:"created_at"`
}

type HeartBeat = struct {
	ID        int64     `json:"id"`
	Day       time.Time `json:"day"`
	Entity    string    `json:"entity"`
	Type      string    `json:"type"`
	Category  string    `json:"category,omitempty"`
	Time      float64   `json:"time"`
	Project   string    `json:"project,omitempty"`
	Branch    string    `json:"branch,omitempty"`
	Language  string    `json:"language,omitempty"`
	IsWrite   bool      `json:"is_write"`
	MachineID string    `json:"machine_name_id,omitempty"`
	Lines     int       `json:"lines,omitempty"`
	LineNo    int       `json:"lineno,omitempty"`
	CursorPos int       `json:"cursorpos,omitempty"`
	CreatedAt time.Time `json:"created_at"`
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
	CreatedAt    time.Time `json:"created_at"`
}

type DayStats = struct {
	ID           int64     `json:"id"`
	Day          time.Time `json:"day"`
	Type         string    `json:"type"`
	Name         string    `json:"name"`
	TotalSeconds float64   `json:"total_seconds"`
	CreatedAt    time.Time `json:"created_at"`
}
