package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/charlie0129/wakatime-sync-go/internal/config"
	"github.com/charlie0129/wakatime-sync-go/internal/database"
	"github.com/charlie0129/wakatime-sync-go/internal/sync"
)

type Handler struct {
	cfg    *config.Config
	db     *database.DB
	syncer *sync.Syncer
}

func NewHandler(cfg *config.Config, db *database.DB, syncer *sync.Syncer) *Handler {
	return &Handler{
		cfg:    cfg,
		db:     db,
		syncer: syncer,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// API routes that resemble official WakaTime API
	mux.HandleFunc("GET /api/v1/users/current/durations", h.getDurations)
	mux.HandleFunc("GET /api/v1/users/current/heartbeats", h.getHeartbeats)
	mux.HandleFunc("GET /api/v1/users/current/summaries", h.getSummaries)
	mux.HandleFunc("GET /api/v1/users/current/projects", h.getProjects)

	// Additional convenience endpoints
	mux.HandleFunc("GET /api/v1/stats/daily", h.getDailyStats)
	mux.HandleFunc("GET /api/v1/stats/range", h.getRangeStats)

	// Sync endpoints
	mux.HandleFunc("POST /api/v1/sync", h.triggerSync)
	mux.HandleFunc("GET /api/v1/sync/status", h.getSyncStatus)

	// Health check
	mux.HandleFunc("GET /health", h.healthCheck)

	// Serve static files from web/dist (for production)
	mux.Handle("/", http.FileServer(http.Dir("web/dist")))
}

// --- Response helpers ---

type APIResponse struct {
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, APIResponse{Error: message})
}

func parseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

// --- Handlers ---

// getDurations returns durations for a specific day
// GET /api/v1/users/current/durations?date=2024-01-01
func (h *Handler) getDurations(w http.ResponseWriter, r *http.Request) {
	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		dateStr = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	}

	day, err := parseDate(dateStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid date format, use YYYY-MM-DD")
		return
	}

	project := r.URL.Query().Get("project")

	var data interface{}
	if project != "" {
		durations, err := h.db.GetProjectDurationsByDay(day, project)
		if err != nil {
			slog.Error("failed to get project durations", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to get durations")
			return
		}
		// Format response like WakaTime API
		formatted := make([]map[string]interface{}, len(durations))
		for i, d := range durations {
			formatted[i] = map[string]interface{}{
				"project":  d.Project,
				"time":     d.StartTime,
				"duration": d.Duration,
				"entity":   d.Entity,
				"language": d.Language,
				"branch":   d.Branch,
				"type":     d.Type,
			}
		}
		data = formatted
	} else {
		durations, err := h.db.GetDurationsByDay(day)
		if err != nil {
			slog.Error("failed to get durations", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to get durations")
			return
		}
		// Format response like WakaTime API
		formatted := make([]map[string]interface{}, len(durations))
		for i, d := range durations {
			formatted[i] = map[string]interface{}{
				"project":  d.Project,
				"time":     d.StartTime,
				"duration": d.Duration,
			}
		}
		data = formatted
	}

	loc := h.cfg.GetTimezone()
	startOfDay := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	endOfDay := startOfDay.Add(24*time.Hour - time.Second)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":     data,
		"start":    startOfDay.Format(time.RFC3339),
		"end":      endOfDay.Format(time.RFC3339),
		"timezone": loc.String(),
	})
}

// getHeartbeats returns heartbeats for a specific day
// GET /api/v1/users/current/heartbeats?date=2024-01-01
func (h *Handler) getHeartbeats(w http.ResponseWriter, r *http.Request) {
	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		dateStr = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	}

	day, err := parseDate(dateStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid date format, use YYYY-MM-DD")
		return
	}

	heartbeats, err := h.db.GetHeartbeatsByDay(day)
	if err != nil {
		slog.Error("failed to get heartbeats", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get heartbeats")
		return
	}

	// Format response like WakaTime API
	formatted := make([]map[string]interface{}, len(heartbeats))
	for i, hb := range heartbeats {
		formatted[i] = map[string]interface{}{
			"entity":          hb.Entity,
			"type":            hb.Type,
			"category":        hb.Category,
			"time":            hb.Time,
			"project":         hb.Project,
			"branch":          hb.Branch,
			"language":        hb.Language,
			"is_write":        hb.IsWrite,
			"machine_name_id": hb.MachineID,
			"lines":           hb.Lines,
			"lineno":          hb.LineNo,
			"cursorpos":       hb.CursorPos,
		}
	}

	loc := h.cfg.GetTimezone()
	startOfDay := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	endOfDay := startOfDay.Add(24*time.Hour - time.Second)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data":     formatted,
		"start":    startOfDay.Format(time.RFC3339),
		"end":      endOfDay.Format(time.RFC3339),
		"timezone": loc.String(),
	})
}

// getSummaries returns summaries for a date range
// GET /api/v1/users/current/summaries?start=2024-01-01&end=2024-01-07
func (h *Handler) getSummaries(w http.ResponseWriter, r *http.Request) {
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	if startStr == "" || endStr == "" {
		// Default to last 7 days
		endStr = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		startStr = time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	}

	start, err := parseDate(startStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid start date format")
		return
	}

	end, err := parseDate(endStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid end date format")
		return
	}

	if start.After(end) {
		writeError(w, http.StatusBadRequest, "start date must be before end date")
		return
	}

	// Build daily summaries
	summaries := []map[string]interface{}{}
	var cumulativeSeconds float64

	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		dayData := h.buildDaySummary(d)
		summaries = append(summaries, dayData)

		if grandTotal, ok := dayData["grand_total"].(map[string]interface{}); ok {
			if totalSecs, ok := grandTotal["total_seconds"].(float64); ok {
				cumulativeSeconds += totalSecs
			}
		}
	}

	// Calculate daily average
	totalDays := int(end.Sub(start).Hours()/24) + 1
	avgSeconds := float64(0)
	if totalDays > 0 {
		avgSeconds = cumulativeSeconds / float64(totalDays)
	}

	loc := h.cfg.GetTimezone()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": summaries,
		"cumulative_total": map[string]interface{}{
			"seconds": cumulativeSeconds,
			"text":    formatDuration(cumulativeSeconds),
			"digital": formatDigital(cumulativeSeconds),
		},
		"daily_average": map[string]interface{}{
			"seconds":                 avgSeconds,
			"text":                    formatDuration(avgSeconds),
			"days_including_holidays": totalDays,
		},
		"start": start.Format("2006-01-02") + "T00:00:00" + formatTimezoneOffset(loc),
		"end":   end.Format("2006-01-02") + "T23:59:59" + formatTimezoneOffset(loc),
	})
}

func (h *Handler) buildDaySummary(day time.Time) map[string]interface{} {
	summary, _ := h.db.GetDaySummary(day)
	totalSeconds := float64(0)
	if summary != nil {
		totalSeconds = summary.TotalSeconds
	}

	// Get stats breakdowns
	categories, _ := h.db.GetDayStatsByDayAndType(day, "category")
	languages, _ := h.db.GetDayStatsByDayAndType(day, "language")
	editors, _ := h.db.GetDayStatsByDayAndType(day, "editor")
	operating_systems, _ := h.db.GetDayStatsByDayAndType(day, "os")
	projects, _ := h.db.GetDayStatsByDayAndType(day, "project")
	dependencies, _ := h.db.GetDayStatsByDayAndType(day, "dependency")
	machines, _ := h.db.GetDayStatsByDayAndType(day, "machine")

	loc := h.cfg.GetTimezone()

	return map[string]interface{}{
		"grand_total": map[string]interface{}{
			"total_seconds": totalSeconds,
			"digital":       formatDigital(totalSeconds),
			"hours":         int(totalSeconds / 3600),
			"minutes":       int(totalSeconds/60) % 60,
			"text":          formatDuration(totalSeconds),
		},
		"categories":        formatStatsItems(categories, totalSeconds),
		"languages":         formatStatsItems(languages, totalSeconds),
		"editors":           formatStatsItems(editors, totalSeconds),
		"operating_systems": formatStatsItems(operating_systems, totalSeconds),
		"projects":          formatStatsItems(projects, totalSeconds),
		"dependencies":      formatStatsItems(dependencies, totalSeconds),
		"machines":          formatMachineItems(machines, totalSeconds),
		"range": map[string]interface{}{
			"date":     day.Format("2006-01-02"),
			"start":    day.Format("2006-01-02") + "T00:00:00" + formatTimezoneOffset(loc),
			"end":      day.Format("2006-01-02") + "T23:59:59" + formatTimezoneOffset(loc),
			"text":     day.Format("Mon Jan 2, 2006"),
			"timezone": loc.String(),
		},
	}
}

func formatStatsItems(stats []database.DayStats, totalSeconds float64) []map[string]interface{} {
	items := make([]map[string]interface{}, len(stats))
	for i, s := range stats {
		percent := float64(0)
		if totalSeconds > 0 {
			percent = (s.TotalSeconds / totalSeconds) * 100
		}
		items[i] = map[string]interface{}{
			"name":          s.Name,
			"total_seconds": s.TotalSeconds,
			"percent":       percent,
			"digital":       formatDigital(s.TotalSeconds),
			"hours":         int(s.TotalSeconds / 3600),
			"minutes":       int(s.TotalSeconds/60) % 60,
			"seconds":       int(s.TotalSeconds) % 60,
			"text":          formatDuration(s.TotalSeconds),
		}
	}
	return items
}

func formatMachineItems(stats []database.DayStats, totalSeconds float64) []map[string]interface{} {
	items := make([]map[string]interface{}, len(stats))
	for i, s := range stats {
		percent := float64(0)
		if totalSeconds > 0 {
			percent = (s.TotalSeconds / totalSeconds) * 100
		}
		items[i] = map[string]interface{}{
			"name":            s.Name,
			"machine_name_id": s.Name, // Use name as ID since we don't store separate ID
			"total_seconds":   s.TotalSeconds,
			"percent":         percent,
			"digital":         formatDigital(s.TotalSeconds),
			"hours":           int(s.TotalSeconds / 3600),
			"minutes":         int(s.TotalSeconds/60) % 60,
			"seconds":         int(s.TotalSeconds) % 60,
			"text":            formatDuration(s.TotalSeconds),
		}
	}
	return items
}

func formatDuration(seconds float64) string {
	hours := int(seconds / 3600)
	mins := int(seconds/60) % 60
	if hours > 0 {
		return strconv.Itoa(hours) + " hrs " + strconv.Itoa(mins) + " mins"
	}
	return strconv.Itoa(mins) + " mins"
}

func formatDigital(seconds float64) string {
	hours := int(seconds / 3600)
	mins := int(seconds/60) % 60
	return strconv.Itoa(hours) + ":" + padZero(mins)
}

func padZero(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func formatTimezoneOffset(loc *time.Location) string {
	t := time.Now().In(loc)
	_, offset := t.Zone()
	hours := offset / 3600
	mins := (offset % 3600) / 60
	if offset >= 0 {
		return "+" + padZero(hours) + ":" + padZero(mins)
	}
	return "-" + padZero(-hours) + ":" + padZero(-mins)
}

// getProjects returns all projects
// GET /api/v1/users/current/projects?q=search
func (h *Handler) getProjects(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")

	projects, err := h.db.GetProjects(query)
	if err != nil {
		slog.Error("failed to get projects", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get projects")
		return
	}

	formatted := make([]map[string]interface{}, len(projects))
	for i, p := range projects {
		formatted[i] = map[string]interface{}{
			"id":                 p.UUID,
			"name":               p.Name,
			"repository":         p.Repository,
			"badge":              p.Badge,
			"color":              p.Color,
			"has_public_url":     p.HasPublicURL,
			"last_heartbeat_at":  formatTime(p.LastHeartbeatAt),
			"first_heartbeat_at": formatTime(p.FirstHeartbeatAt),
			"created_at":         formatTime(p.CreatedAt),
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": formatted,
	})
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// getDailyStats returns daily totals for a date range
// GET /api/v1/stats/daily?start=2024-01-01&end=2024-01-31
func (h *Handler) getDailyStats(w http.ResponseWriter, r *http.Request) {
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	if startStr == "" || endStr == "" {
		// Default to last 30 days
		endStr = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		startStr = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	}

	start, err := parseDate(startStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid start date format")
		return
	}

	end, err := parseDate(endStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid end date format")
		return
	}

	summaries, err := h.db.GetDaySummaries(start, end)
	if err != nil {
		slog.Error("failed to get daily stats", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}

	// Create a map for quick lookup
	summaryMap := make(map[string]float64)
	for _, s := range summaries {
		summaryMap[s.Day.Format("2006-01-02")] = s.TotalSeconds
	}

	// Fill in all days including zeros
	var data []map[string]interface{}
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		totalSeconds := summaryMap[dateStr]
		data = append(data, map[string]interface{}{
			"date":          dateStr,
			"total_seconds": totalSeconds,
			"text":          formatDuration(totalSeconds),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": data,
	})
}

// getRangeStats returns aggregated stats for a date range
// GET /api/v1/stats/range?start=2024-01-01&end=2024-01-31
func (h *Handler) getRangeStats(w http.ResponseWriter, r *http.Request) {
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	if startStr == "" || endStr == "" {
		endStr = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		startStr = time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	}

	start, err := parseDate(startStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid start date format")
		return
	}

	end, err := parseDate(endStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid end date format")
		return
	}

	// Get aggregated stats
	categories, _ := h.db.GetAggregatedStats(start, end, "category")
	languages, _ := h.db.GetAggregatedStats(start, end, "language")
	editors, _ := h.db.GetAggregatedStats(start, end, "editor")
	operating_systems, _ := h.db.GetAggregatedStats(start, end, "os")
	projects, _ := h.db.GetAggregatedStats(start, end, "project")

	// Get daily project breakdown
	projectDaily, _ := h.db.GetProjectDailyStats(start, end)

	// Calculate total
	var totalSeconds float64
	for _, p := range projects {
		totalSeconds += p.TotalSeconds
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_seconds":     totalSeconds,
		"text":              formatDuration(totalSeconds),
		"categories":        formatAggStats(categories, totalSeconds),
		"languages":         formatAggStats(languages, totalSeconds),
		"editors":           formatAggStats(editors, totalSeconds),
		"operating_systems": formatAggStats(operating_systems, totalSeconds),
		"projects":          formatAggStats(projects, totalSeconds),
		"projects_daily":    projectDaily,
		"start":             startStr,
		"end":               endStr,
	})
}

func formatAggStats(stats []struct {
	Name         string  `json:"name"`
	TotalSeconds float64 `json:"total_seconds"`
}, totalSeconds float64) []map[string]interface{} {
	items := make([]map[string]interface{}, len(stats))
	for i, s := range stats {
		percent := float64(0)
		if totalSeconds > 0 {
			percent = (s.TotalSeconds / totalSeconds) * 100
		}
		items[i] = map[string]interface{}{
			"name":          s.Name,
			"total_seconds": s.TotalSeconds,
			"percent":       percent,
			"text":          formatDuration(s.TotalSeconds),
		}
	}
	return items
}

// triggerSync manually triggers a sync
// POST /api/v1/sync?days=7&api_key=xxx
func (h *Handler) triggerSync(w http.ResponseWriter, r *http.Request) {
	// Check API key
	apiKey := r.URL.Query().Get("api_key")
	if apiKey == "" {
		apiKey = r.FormValue("apiKey")
	}
	if apiKey != h.cfg.WakaTimeAPI {
		writeError(w, http.StatusUnauthorized, "invalid api key")
		return
	}

	daysStr := r.URL.Query().Get("days")
	if daysStr == "" {
		daysStr = r.FormValue("day")
	}
	days, err := strconv.Atoi(daysStr)
	if err != nil || days <= 0 {
		days = 1
	}

	// Run sync in background
	go func() {
		if err := h.syncer.SyncDays(days); err != nil {
			slog.Error("sync failed", "error", err)
		}
		// Also sync projects
		h.syncer.SyncProjects()
	}()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "sync started",
		"days":    days,
	})
}

// getSyncStatus returns sync status
// GET /api/v1/sync/status
func (h *Handler) getSyncStatus(w http.ResponseWriter, r *http.Request) {
	lastSynced, err := h.db.GetLastSyncedDay()
	if err != nil {
		slog.Error("failed to get sync status", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get sync status")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"last_synced_day": lastSynced.Format("2006-01-02"),
	})
}

func (h *Handler) healthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
	})
}
