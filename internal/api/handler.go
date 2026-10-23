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
	// now returns the current time; tests replace it
	now func() time.Time
}

func NewHandler(cfg *config.Config, db *database.DB, syncer *sync.Syncer) *Handler {
	return &Handler{
		cfg:    cfg,
		db:     db,
		syncer: syncer,
		now:    time.Now,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// API routes that resemble official WakaTime API
	mux.HandleFunc("GET /api/v1/users/current/durations", h.getDurations)
	mux.HandleFunc("GET /api/v1/users/current/heartbeats", h.getHeartbeats)
	mux.HandleFunc("GET /api/v1/users/current/summaries", h.getSummaries)
	mux.HandleFunc("GET /api/v1/users/current/projects", h.getProjects)

	// Endpoints of the web dashboard
	mux.HandleFunc("GET /api/v1/dashboard", h.getDashboard)
	mux.HandleFunc("GET /api/v1/activity", h.getActivity)

	// Additional convenience endpoints
	mux.HandleFunc("GET /api/v1/stats/daily", h.getDailyStats)
	mux.HandleFunc("GET /api/v1/stats/range", h.getRangeStats)
	mux.HandleFunc("GET /api/v1/stats/years", h.getAvailableYears)
	mux.HandleFunc("GET /api/v1/stats/yearly", h.getYearlyActivity)

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
		dateStr = time.Now().In(h.cfg.GetTimezone()).AddDate(0, 0, -1).Format("2006-01-02")
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
			addAIFields(formatted[i], d.AIStats, parseModels(d.AIModelBreakdown))
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
			addAIFields(formatted[i], d.AIStats, parseModels(d.AIModelBreakdown))
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
		dateStr = time.Now().In(h.cfg.GetTimezone()).AddDate(0, 0, -1).Format("2006-01-02")
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
			"id":                     hb.WakaTimeID,
			"entity":                 hb.Entity,
			"type":                   hb.Type,
			"category":               hb.Category,
			"time":                   hb.Time,
			"project":                hb.Project,
			"project_root_count":     hb.ProjectRootCount,
			"branch":                 hb.Branch,
			"language":               hb.Language,
			"dependencies":           parseDependencies(hb.Dependencies),
			"is_write":               hb.IsWrite,
			"machine_name_id":        hb.MachineID,
			"user_agent_id":          hb.UserAgentID,
			"lines":                  hb.Lines,
			"lineno":                 hb.LineNo,
			"cursorpos":              hb.CursorPos,
			"ai_line_changes":        hb.AILineChanges,
			"human_line_changes":     hb.HumanLineChanges,
			"ai_session":             hb.AISession,
			"ai_input_tokens":        hb.AIInputTokens,
			"ai_cached_input_tokens": hb.AICachedInputTokens,
			"ai_output_tokens":       hb.AIOutputTokens,
			"ai_prompt_length":       hb.AIPromptLength,
			"ai_subscription_plan":   hb.AISubscriptionPlan,
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
		now := time.Now().In(h.cfg.GetTimezone())
		endStr = now.AddDate(0, 0, -1).Format("2006-01-02")
		startStr = now.AddDate(0, 0, -7).Format("2006-01-02")
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

	aiModels, err := h.db.GetDayAIModels(start, end)
	if err != nil {
		slog.Error("failed to get ai models", "error", err)
	}
	modelsByDay := groupModelsByDay(aiModels)

	// Build daily summaries
	summaries := []map[string]interface{}{}
	var cumulativeSeconds float64

	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		dayData := h.buildDaySummary(d, modelsByDay[d.Format("2006-01-02")])
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

// buildDaySummary returns the summary of a day like WakaTime, given the usage of AI models on that day.
func (h *Handler) buildDaySummary(day time.Time, models statModels) map[string]interface{} {
	summary, _ := h.db.GetDaySummary(day)
	totalSeconds := float64(0)
	var ai database.AIStats
	if summary != nil {
		totalSeconds = summary.TotalSeconds
		ai = summary.AIStats
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

	grandTotal := map[string]interface{}{
		"total_seconds": totalSeconds,
		"digital":       formatDigital(totalSeconds),
		"hours":         int(totalSeconds / 3600),
		"minutes":       int(totalSeconds/60) % 60,
		"text":          formatDuration(totalSeconds),
	}
	addAIFields(grandTotal, ai, models[statKey{"total", ""}])

	return map[string]interface{}{
		"grand_total":       grandTotal,
		"categories":        formatStatsItems(categories, totalSeconds, false, nil),
		"languages":         formatStatsItems(languages, totalSeconds, false, nil),
		"editors":           formatStatsItems(editors, totalSeconds, true, models),
		"operating_systems": formatStatsItems(operating_systems, totalSeconds, false, nil),
		"projects":          formatStatsItems(projects, totalSeconds, true, models),
		"dependencies":      formatStatsItems(dependencies, totalSeconds, false, nil),
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

// formatStatsItems formats a breakdown of a day like WakaTime, with the AI coding metrics when
// withAI is set, which WakaTime does for projects and editors.
func formatStatsItems(stats []database.DayStats, totalSeconds float64, withAI bool, models statModels) []map[string]interface{} {
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
		if withAI {
			addAIFields(items[i], s.AIStats, models[statKey{s.Type, s.Name}])
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
		now := time.Now().In(h.cfg.GetTimezone())
		endStr = now.AddDate(0, 0, -1).Format("2006-01-02")
		startStr = now.AddDate(0, 0, -30).Format("2006-01-02")
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
		now := time.Now().In(h.cfg.GetTimezone())
		endStr = now.AddDate(0, 0, -1).Format("2006-01-02")
		startStr = now.AddDate(0, 0, -7).Format("2006-01-02")
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

	// Get AI coding activity
	aiTotals, err := h.db.GetAITotals(start, end)
	if err != nil {
		slog.Error("failed to get ai totals", "error", err)
		aiTotals = &database.AITotals{}
	}
	aiModels, err := h.db.GetAIModelUsage(start, end)
	if err != nil {
		slog.Error("failed to get ai model usage", "error", err)
	}
	aiModelVersions, err := h.db.GetAIModelVersions(start, end)
	if err != nil {
		slog.Error("failed to get ai model versions", "error", err)
	}
	aiModelNames, err := h.db.GetAIModelNames()
	if err != nil {
		slog.Error("failed to get ai model names", "error", err)
	}
	ai := map[string]interface{}{
		"ai_coding_seconds":    aiTotals.AICodingSeconds,
		"active_days":          aiTotals.ActiveDays,
		"ai_sessions":          aiTotals.AISessions,
		"ai_prompt_length_avg": aiTotals.AIPromptLengthAvg,
	}
	addAISums(ai, aiTotals.AIStats)

	// Calculate total
	var totalSeconds float64
	for _, p := range projects {
		totalSeconds += p.TotalSeconds
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"total_seconds":     totalSeconds,
		"text":              formatDuration(totalSeconds),
		"categories":        formatAggStats(categories, totalSeconds, false),
		"languages":         formatAggStats(languages, totalSeconds, false),
		"editors":           formatAggStats(editors, totalSeconds, true),
		"operating_systems": formatAggStats(operating_systems, totalSeconds, false),
		"projects":          formatAggStats(projects, totalSeconds, true),
		"projects_daily":    projectDaily,
		"ai":                ai,
		"ai_models":         nonNil(aiModels),
		"ai_model_versions": nonNil(aiModelVersions),
		"ai_model_names":    nonNil(aiModelNames),
		"start":             startStr,
		"end":               endStr,
	})
}

// formatAggStats formats stats aggregated over a date range, with the AI coding metrics when withAI is set.
func formatAggStats(stats []database.AggregatedStat, totalSeconds float64, withAI bool) []map[string]interface{} {
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
		if withAI {
			addAISums(items[i], s.AIStats)
		}
	}
	return items
}

// --- AI coding stats helpers ---

// modelUsage is the usage of an AI model, as in WakaTime's ai_model_breakdown.
type modelUsage struct {
	Name              string  `json:"name"`
	Lines             int64   `json:"lines"`
	InputTokens       int64   `json:"input_tokens"`
	CachedInputTokens int64   `json:"cached_input_tokens"`
	OutputTokens      int64   `json:"output_tokens"`
	Cost              float64 `json:"cost"`
}

// statKey identifies the total of a day (type "total" and no name), or one of its projects or editors.
type statKey struct {
	Type, Name string
}

// statModels is the usage of AI models on a day, by total, project and editor.
type statModels map[statKey][]modelUsage

// groupModelsByDay groups the usage of AI models by day.
func groupModelsByDay(models []database.DayAIModel) map[string]statModels {
	byDay := make(map[string]statModels)
	for _, m := range models {
		day := m.Day.Format("2006-01-02")
		if byDay[day] == nil {
			byDay[day] = make(statModels)
		}
		key := statKey{m.Type, m.Name}
		byDay[day][key] = append(byDay[day][key], modelUsage{
			Name:              m.Model,
			Lines:             m.Lines,
			InputTokens:       m.InputTokens,
			CachedInputTokens: m.CachedInputTokens,
			OutputTokens:      m.OutputTokens,
			Cost:              m.Cost,
		})
	}
	return byDay
}

// parseModels parses the usage of AI models stored for a duration.
func parseModels(breakdown string) []modelUsage {
	var models []modelUsage
	if breakdown != "" {
		if err := json.Unmarshal([]byte(breakdown), &models); err != nil {
			slog.Warn("invalid stored ai_model_breakdown", "error", err)
		}
	}
	return models
}

// parseDependencies parses stored dependencies, which are a JSON array.
func parseDependencies(dependencies string) []string {
	deps := []string{}
	if dependencies != "" {
		if err := json.Unmarshal([]byte(dependencies), &deps); err != nil || deps == nil {
			deps = []string{}
		}
	}
	return deps
}

// addAISums adds the AI coding metrics that add up over several days or items.
func addAISums(item map[string]interface{}, a database.AIStats) {
	item["ai_additions"] = a.AIAdditions
	item["ai_deletions"] = a.AIDeletions
	item["human_additions"] = a.HumanAdditions
	item["human_deletions"] = a.HumanDeletions
	item["ai_input_tokens"] = a.AIInputTokens
	item["ai_cached_input_tokens"] = a.AICachedInputTokens
	item["ai_output_tokens"] = a.AIOutputTokens
	item["ai_model_total_cost"] = a.AIModelTotalCost
	item["ai_prompt_events_total"] = a.AIPromptEventsTotal
	item["ai_prompt_length_sum"] = a.AIPromptLengthSum
}

// addAIFields adds the AI coding metrics of a day, project, editor or duration like WakaTime, with
// the usage of each AI model as ai_model_breakdown, ai_model_line_changes and ai_model_costs.
func addAIFields(item map[string]interface{}, a database.AIStats, models []modelUsage) {
	addAISums(item, a)
	item["ai_sessions"] = a.AISessions
	item["ai_prompt_length_avg"] = a.AIPromptLengthAvg
	item["ai_prompt_length_avg_per_session"] = a.AIPromptLengthAvgPerSession
	item["ai_prompt_length_median_per_session"] = a.AIPromptLengthMedianPerSession
	item["ai_prompt_events_avg_per_session"] = a.AIPromptEventsAvgPerSession
	item["ai_prompt_events_median_per_session"] = a.AIPromptEventsMedianPerSession

	// Like WakaTime, the maps leave out models without lines or cost
	lineChanges := make(map[string]int64)
	costs := make(map[string]float64)
	for _, m := range models {
		if m.Lines > 0 {
			lineChanges[m.Name] = m.Lines
		}
		if m.Cost > 0 {
			costs[m.Name] = m.Cost
		}
	}
	item["ai_model_breakdown"] = nonNil(models)
	item["ai_model_line_changes"] = lineChanges
	item["ai_model_costs"] = costs
}

// nonNil returns s, or an empty slice instead of nil so that it is encoded as [] rather than null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// triggerSync manually triggers a sync of the last N days, or of a date range.
// By default, stored data is kept when WakaTime returns less of it; force=true overwrites it anyway.
// POST /api/v1/sync?days=7&api_key=xxx
// POST /api/v1/sync?start=2024-01-01&end=2024-01-31&force=true&api_key=xxx
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

	// Dates are kept as UTC midnights, like the ones parsed from the query
	y, m, d := time.Now().In(h.cfg.GetTimezone()).AddDate(0, 0, -1).Date()
	yesterday := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)

	var start, end time.Time
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	if startStr != "" || endStr != "" {
		var err error
		if start, err = parseDate(startStr); err != nil {
			writeError(w, http.StatusBadRequest, "invalid start date format, use YYYY-MM-DD")
			return
		}
		end = yesterday
		if endStr != "" {
			if end, err = parseDate(endStr); err != nil {
				writeError(w, http.StatusBadRequest, "invalid end date format, use YYYY-MM-DD")
				return
			}
		}
		if start.After(end) {
			writeError(w, http.StatusBadRequest, "start date must be before end date")
			return
		}
	} else {
		daysStr := r.URL.Query().Get("days")
		if daysStr == "" {
			daysStr = r.FormValue("day")
		}
		days, err := strconv.Atoi(daysStr)
		if err != nil || days <= 0 {
			days = 1
		}
		end = yesterday
		start = yesterday.AddDate(0, 0, 1-days)
	}

	force, _ := strconv.ParseBool(r.URL.Query().Get("force"))

	// Run sync in background
	if !h.syncer.StartSync(start, end, force) {
		writeError(w, http.StatusConflict, "a sync is already running")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "sync started",
		"days":    int(end.Sub(start).Hours()/24) + 1,
		"start":   start.Format("2006-01-02"),
		"end":     end.Format("2006-01-02"),
		"force":   force,
	})
}

// getSyncStatus returns the last successfully synced day, the days whose last sync failed, and the
// current sync run, or else the last one.
// GET /api/v1/sync/status
func (h *Handler) getSyncStatus(w http.ResponseWriter, r *http.Request) {
	lastSynced, err := h.db.GetLastSyncedDay()
	if err != nil {
		slog.Error("failed to get sync status", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get sync status")
		return
	}
	failed, err := h.db.GetFailedSyncDays()
	if err != nil {
		slog.Error("failed to get sync status", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get sync status")
		return
	}

	lastSyncedDay := ""
	if !lastSynced.IsZero() {
		lastSyncedDay = lastSynced.Format(dayLayout)
	}
	failedDays := make([]string, len(failed))
	for i, d := range failed {
		failedDays[i] = d.Format(dayLayout)
	}
	running, run := h.syncer.Status()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"last_synced_day": lastSyncedDay,
		"failed_days":     failedDays,
		"running":         running,
		"run":             run,
		"today":           h.today().Format(dayLayout),
		"timezone":        h.cfg.GetTimezone().String(),
	})
}

// getAvailableYears returns all years that have activity data
// GET /api/v1/stats/years
func (h *Handler) getAvailableYears(w http.ResponseWriter, r *http.Request) {
	years, err := h.db.GetAvailableYears()
	if err != nil {
		slog.Error("failed to get available years", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get available years")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"years": years,
	})
}

// getYearlyActivity returns daily activity data for an entire year (for heatmap)
// GET /api/v1/stats/yearly?year=2024
func (h *Handler) getYearlyActivity(w http.ResponseWriter, r *http.Request) {
	yearStr := r.URL.Query().Get("year")
	if yearStr == "" {
		yearStr = strconv.Itoa(time.Now().In(h.cfg.GetTimezone()).Year())
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid year format")
		return
	}

	activity, err := h.db.GetYearlyActivity(year)
	if err != nil {
		slog.Error("failed to get yearly activity", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get yearly activity")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"year": year,
		"data": activity,
	})
}

func (h *Handler) healthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
	})
}
