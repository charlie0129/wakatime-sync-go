package sync

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	gosync "sync"
	"time"

	"github.com/charlie0129/wakatime-sync-go/internal/config"
	"github.com/charlie0129/wakatime-sync-go/internal/database"
	"github.com/charlie0129/wakatime-sync-go/internal/wakatime"
	"github.com/robfig/cron/v3"
)

const dayLayout = "2006-01-02"

type Syncer struct {
	cfg        *config.Config
	db         *database.DB
	client     *wakatime.Client
	cron       *cron.Cron
	maxRetries int
	// retryDelay is the base delay for retries, which will be multiplied by the attempt number (exponential backoff)
	retryDelay time.Duration
	// mu ensures that only one sync runs at a time
	mu gosync.Mutex

	// statusMu guards running and run, the current sync run or else the last one
	statusMu gosync.Mutex
	running  bool
	run      *RunStatus
}

// RunStatus describes a sync run.
type RunStatus struct {
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	// Days is the number of days to sync; Done of them were processed so far, of which Failed failed
	Days   int `json:"days"`
	Done   int `json:"done"`
	Failed int `json:"failed"`
}

// Status returns whether a sync is running, and the current sync run, or else the last one, if any.
func (s *Syncer) Status() (bool, *RunStatus) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	if s.run == nil {
		return s.running, nil
	}
	run := *s.run
	return s.running, &run
}

// updateRun changes the status of the current sync run, if any.
func (s *Syncer) updateRun(update func(run *RunStatus)) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	if s.run != nil {
		update(s.run)
	}
}

// runSync syncs the given days, then refreshes projects and user agents, keeping track of its status.
// It must be called with mu held.
func (s *Syncer) runSync(days []time.Time, force bool) {
	s.statusMu.Lock()
	s.running = true
	s.run = &RunStatus{StartedAt: time.Now(), Days: len(days)}
	s.statusMu.Unlock()

	defer func() {
		s.statusMu.Lock()
		defer s.statusMu.Unlock()
		finished := time.Now()
		s.run.FinishedAt = &finished
		s.running = false
	}()

	s.syncDays(days, force)
	s.syncMetadata()
}

func NewSyncer(cfg *config.Config, db *database.DB) *Syncer {
	return &Syncer{
		cfg:        cfg,
		db:         db,
		client:     wakatime.NewClientWithBaseURL(cfg.WakaTimeAPI, cfg.ProxyURL, cfg.WakaTimeBaseURL),
		maxRetries: 3,
		retryDelay: 5 * time.Second,
	}
}

// dependenciesToString converts a dependencies array to a JSON string for storage
func dependenciesToString(deps []string) string {
	// Always return valid JSON, even for empty arrays
	b, err := json.Marshal(deps)
	if err != nil {
		// Return empty JSON array on error
		return "[]"
	}
	return string(b)
}

// aiStats converts AI coding metrics for storage.
func aiStats(a wakatime.AIStats) database.AIStats {
	return database.AIStats{
		AIAdditions:                    int64(a.AIAdditions),
		AIDeletions:                    int64(a.AIDeletions),
		HumanAdditions:                 int64(a.HumanAdditions),
		HumanDeletions:                 int64(a.HumanDeletions),
		AIInputTokens:                  a.AIInputTokens,
		AICachedInputTokens:            a.AICachedInputTokens,
		AIOutputTokens:                 a.AIOutputTokens,
		AIModelTotalCost:               a.TotalCost(),
		AISessions:                     int64(a.AISessions),
		AIPromptEventsTotal:            int64(a.AIPromptEventsTotal),
		AIPromptLengthSum:              a.AIPromptLengthSum,
		AIPromptLengthAvg:              a.AIPromptLengthAvg,
		AIPromptLengthAvgPerSession:    a.AIPromptLengthAvgPerSession,
		AIPromptLengthMedianPerSession: a.AIPromptLengthMedianPerSession,
		AIPromptEventsAvgPerSession:    a.AIPromptEventsAvgPerSession,
		AIPromptEventsMedianPerSession: a.AIPromptEventsMedianPerSession,
	}
}

// aiModels converts the usage of each AI model on a day for storage.
func aiModels(day time.Time, statType, name string, a wakatime.AIStats) []database.DayAIModel {
	var models []database.DayAIModel
	for _, m := range a.Models() {
		models = append(models, database.DayAIModel{
			Day:               day,
			Type:              statType,
			Name:              name,
			Model:             m.Name,
			Lines:             int64(m.Lines),
			InputTokens:       m.InputTokens,
			CachedInputTokens: m.CachedInputTokens,
			OutputTokens:      m.OutputTokens,
			Cost:              m.Cost,
		})
	}
	return models
}

// aiModelsToString converts the usage of each AI model to a JSON string for storage, or an empty
// string when no AI model was used.
func aiModelsToString(a wakatime.AIStats) string {
	models := a.Models()
	if len(models) == 0 {
		return ""
	}
	b, err := json.Marshal(models)
	if err != nil {
		return ""
	}
	return string(b)
}

func (s *Syncer) StartScheduler() {
	// Catch up on missed days immediately on startup
	if v := os.Getenv("SKIP_INITIAL_SYNC"); v == "1" || v == "true" {
		slog.Info("skipping initial sync due to SKIP_INITIAL_SYNC env var")
	} else {
		slog.Info("performing initial sync")
		s.RunScheduledSync()
	}

	// Set up cron scheduler with configured timezone
	loc := s.cfg.GetTimezone()
	s.cron = cron.New(cron.WithLocation(loc))

	_, err := s.cron.AddFunc(s.cfg.SyncSchedule, func() {
		slog.Info("running scheduled sync", "schedule", s.cfg.SyncSchedule)
		s.RunScheduledSync()
	})
	if err != nil {
		slog.Error("failed to add cron job, falling back to 24h ticker", "schedule", s.cfg.SyncSchedule, "error", err)
		// Fallback to simple ticker if cron expression is invalid
		go func() {
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				s.RunScheduledSync()
			}
		}()
		return
	}

	slog.Info("scheduled daily sync", "schedule", s.cfg.SyncSchedule, "timezone", loc.String())
	s.cron.Start()
}

func (s *Syncer) Stop() {
	if s.cron != nil {
		s.cron.Stop()
	}
}

// RunScheduledSync syncs every day that is due (see dueDays), then refreshes projects and user agents.
// It waits for a sync that is already running to finish.
func (s *Syncer) RunScheduledSync() {
	s.mu.Lock()
	defer s.mu.Unlock()

	days, err := s.dueDays(time.Now())
	if err != nil {
		slog.Error("failed to determine which days to sync", "error", err)
		return
	}
	s.runSync(days, false)
}

// StartSync syncs every day from start to end in the background, then refreshes projects and user agents.
// It returns false without doing anything if another sync is running.
func (s *Syncer) StartSync(start, end time.Time, force bool) bool {
	if !s.mu.TryLock() {
		return false
	}

	go func() {
		defer s.mu.Unlock()

		var days []time.Time
		for d := s.date(start); !d.After(s.date(end)); d = d.AddDate(0, 0, 1) {
			days = append(days, d)
		}
		s.runSync(days, force)
	}()
	return true
}

// dueDays returns the days a scheduled sync must cover, oldest first. On a fresh database, that is
// every day since start_date. Otherwise, it is every day after the last successful sync, the last
// SyncLookbackDays days (WakaTime keeps receiving heartbeats for past days, e.g. from plugins that
// were offline), and every day whose last sync failed. Days before start_date are never included.
func (s *Syncer) dueDays(now time.Time) ([]time.Time, error) {
	yesterday := s.date(now.In(s.cfg.GetTimezone())).AddDate(0, 0, -1)
	startDate := s.date(s.cfg.GetStartDate())

	lastSynced, err := s.db.GetLastSyncedDay()
	if err != nil {
		return nil, err
	}
	failed, err := s.db.GetFailedSyncDays()
	if err != nil {
		return nil, err
	}

	from := startDate
	if !lastSynced.IsZero() {
		from = s.date(lastSynced).AddDate(0, 0, 1)
		if lookback := yesterday.AddDate(0, 0, 1-s.cfg.SyncLookbackDays); lookback.Before(from) {
			from = lookback
		}
		if from.Before(startDate) {
			from = startDate
		}
	}

	due := make(map[string]time.Time)
	for d := from; !d.After(yesterday); d = d.AddDate(0, 0, 1) {
		due[d.Format(dayLayout)] = d
	}
	for _, d := range failed {
		if d = s.date(d); !d.Before(startDate) && !d.After(yesterday) {
			due[d.Format(dayLayout)] = d
		}
	}

	days := make([]time.Time, 0, len(due))
	for _, d := range due {
		days = append(days, d)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })
	return days, nil
}

// date returns midnight of t's calendar date in the configured timezone.
func (s *Syncer) date(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, s.cfg.GetTimezone())
}

// syncDays syncs the given days one after another.
func (s *Syncer) syncDays(days []time.Time, force bool) {
	if len(days) == 0 {
		return
	}

	slog.Info("syncing days", "from", days[0].Format(dayLayout), "to", days[len(days)-1].Format(dayLayout), "count", len(days), "force", force)
	failed := 0
	for _, day := range days {
		err := s.syncDayWithRetry(day, force)
		if err != nil {
			failed++
		}
		s.updateRun(func(run *RunStatus) {
			run.Done++
			if err != nil {
				run.Failed++
			}
		})
	}
	slog.Info("finished syncing days", "count", len(days), "failed", failed)
}

// syncDayWithRetry syncs the day, retrying transient failures. When all attempts fail, the day is
// recorded as failed so that the next scheduled sync tries again.
func (s *Syncer) syncDayWithRetry(day time.Time, force bool) error {
	dateStr := day.Format(dayLayout)

	var err error
	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		if attempt > 0 {
			delay := s.retryDelay * time.Duration(1<<(attempt-1)) // exponential backoff
			slog.Info("retrying sync", "date", dateStr, "attempt", attempt, "delay", delay)
			time.Sleep(delay)
		}

		if err = s.SyncDay(day, force); err == nil {
			return nil
		}
		slog.Warn("sync attempt failed", "date", dateStr, "attempt", attempt, "error", err)
		if !isRetryable(err) {
			break
		}
	}

	slog.Error("failed to sync day", "date", dateStr, "error", err)
	if recordErr := s.db.RecordSyncFailure(day); recordErr != nil {
		slog.Error("failed to record sync failure", "date", dateStr, "error", recordErr)
	}
	return err
}

// isRetryable reports whether a failed sync may succeed if tried again.
func isRetryable(err error) bool {
	var apiErr *wakatime.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Retryable()
	}
	// Responses that cannot be decoded will not decode any better next time
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	return !errors.As(err, &syntaxErr) && !errors.As(err, &typeErr)
}

// SyncDay fetches everything WakaTime has for the day, then saves it in a single transaction.
// Unless force is set, stored data is kept when the new data is smaller (see database.SaveDay).
func (s *Syncer) SyncDay(day time.Time, force bool) error {
	dateStr := day.Format(dayLayout)
	slog.Info("syncing data", "date", dateStr)

	data := &database.DayData{Day: day}
	if err := s.fetchSummary(data); err != nil {
		return fmt.Errorf("failed to fetch summary: %w", err)
	}
	if err := s.fetchDurations(data); err != nil {
		return fmt.Errorf("failed to fetch durations: %w", err)
	}
	if err := s.fetchHeartbeats(data); err != nil {
		return fmt.Errorf("failed to fetch heartbeats: %w", err)
	}

	result, err := s.db.SaveDay(data, force)
	if err != nil {
		return fmt.Errorf("failed to save data: %w", err)
	}
	for _, part := range result.Kept {
		slog.Warn("kept stored data because the new data is smaller, sync with force to overwrite", "date", dateStr, "part", part)
	}

	slog.Info("sync completed", "date", dateStr, "total_seconds", result.TotalSeconds,
		"durations", len(data.Durations), "project_durations", len(data.ProjectDurations), "heartbeats", len(data.Heartbeats))
	return nil
}

// fetchSummary fetches the day's grand total and its breakdowns.
func (s *Syncer) fetchSummary(data *database.DayData) error {
	resp, err := s.client.GetSummaries(data.Day, data.Day)
	if err != nil {
		return err
	}

	if len(resp.Data) == 0 {
		slog.Info("no summary data for day", "date", data.Day.Format(dayLayout))
		return nil
	}

	summary := resp.Data[0]
	data.Summary = &database.DaySummary{
		Day:          data.Day,
		TotalSeconds: summary.GrandTotal.TotalSeconds,
		AIStats:      aiStats(summary.GrandTotal.AIStats),
	}
	data.AIModels = aiModels(data.Day, "total", "", summary.GrandTotal.AIStats)

	// AI stats are only set for projects and editors
	addStats := func(statType string, items []wakatime.SummaryItem) {
		for _, item := range items {
			data.Stats = append(data.Stats, database.DayStats{
				Day:          data.Day,
				Type:         statType,
				Name:         item.Name,
				TotalSeconds: item.TotalSeconds,
				AIStats:      aiStats(item.AIStats),
			})
			data.AIModels = append(data.AIModels, aiModels(data.Day, statType, item.Name, item.AIStats)...)
		}
	}
	addStats("category", summary.Categories)
	addStats("language", summary.Languages)
	addStats("editor", summary.Editors)
	addStats("os", summary.OperatingSystems)
	addStats("project", summary.Projects)
	addStats("dependency", summary.Dependencies)
	for _, item := range summary.Machines {
		data.Stats = append(data.Stats, database.DayStats{
			Day:          data.Day,
			Type:         "machine",
			Name:         item.Name,
			TotalSeconds: item.TotalSeconds,
		})
	}

	return nil
}

// fetchDurations fetches the day's durations, and the detailed durations of each of its projects.
func (s *Syncer) fetchDurations(data *database.DayData) error {
	resp, err := s.client.GetDurations(data.Day)
	if err != nil {
		return err
	}

	var projects []string
	seen := make(map[string]bool)
	for _, d := range resp.Data {
		data.Durations = append(data.Durations, database.Duration{
			Day:              data.Day,
			Project:          d.Project,
			StartTime:        d.Time,
			Duration:         d.Duration,
			Dependencies:     dependenciesToString(d.Dependencies),
			AIStats:          aiStats(d.AIStats),
			AIModelBreakdown: aiModelsToString(d.AIStats),
		})
		if d.Project != "" && !seen[d.Project] {
			seen[d.Project] = true
			projects = append(projects, d.Project)
		}
	}

	// Any failure fails the whole day, so that stored project durations are never replaced by an incomplete set
	for _, project := range projects {
		projResp, err := s.client.GetDurationsWithProject(data.Day, project)
		if err != nil {
			return fmt.Errorf("project %q: %w", project, err)
		}
		for _, d := range projResp.Data {
			data.ProjectDurations = append(data.ProjectDurations, database.ProjectDuration{
				Day:              data.Day,
				Project:          project,
				Entity:           d.Entity,
				Language:         d.Language,
				Branch:           d.Branch,
				Type:             d.Type,
				StartTime:        d.Time,
				Duration:         d.Duration,
				Dependencies:     dependenciesToString(d.Dependencies),
				AIStats:          aiStats(d.AIStats),
				AIModelBreakdown: aiModelsToString(d.AIStats),
			})
		}
	}

	return nil
}

// fetchHeartbeats fetches the day's heartbeats.
func (s *Syncer) fetchHeartbeats(data *database.DayData) error {
	resp, err := s.client.GetHeartbeats(data.Day)
	if err != nil {
		return err
	}

	for _, h := range resp.Data {
		data.Heartbeats = append(data.Heartbeats, database.HeartBeat{
			WakaTimeID:          h.ID,
			Day:                 data.Day,
			Entity:              h.Entity,
			Type:                h.Type,
			Category:            h.Category,
			Time:                h.Time,
			Project:             h.Project,
			ProjectRootCount:    h.ProjectRootCount,
			Branch:              h.Branch,
			Language:            h.Language,
			Dependencies:        dependenciesToString(h.Dependencies),
			IsWrite:             h.IsWrite,
			MachineID:           h.MachineNameID,
			UserAgentID:         h.UserAgentID,
			Lines:               h.Lines,
			LineNo:              h.LineNo,
			CursorPos:           h.CursorPos,
			AILineChanges:       h.AILineChanges,
			HumanLineChanges:    h.HumanLineChanges,
			AISession:           h.AISession,
			AIInputTokens:       h.AIInputTokens,
			AICachedInputTokens: h.AICachedInputTokens,
			AIOutputTokens:      h.AIOutputTokens,
			AIPromptLength:      h.AIPromptLength,
			AISubscriptionPlan:  h.AISubscriptionPlan,
		})
	}

	return nil
}

// syncMetadata refreshes the stored projects and user agents.
func (s *Syncer) syncMetadata() {
	s.syncProjects()
	s.syncUserAgents()
}

// syncProjects refreshes the stored list of projects.
func (s *Syncer) syncProjects() {
	projects, err := s.client.GetProjects("")
	if err != nil {
		slog.Error("failed to sync projects", "error", err)
		return
	}

	stored := make([]database.Project, 0, len(projects))
	for _, p := range projects {
		var lastHeartbeat, firstHeartbeat time.Time
		if p.LastHeartbeatAt != "" {
			lastHeartbeat, _ = time.Parse(time.RFC3339, p.LastHeartbeatAt)
		}
		if p.FirstHeartbeatAt != "" {
			firstHeartbeat, _ = time.Parse(time.RFC3339, p.FirstHeartbeatAt)
		}

		stored = append(stored, database.Project{
			UUID:             p.ID,
			Name:             p.Name,
			Repository:       string(p.Repository),
			Badge:            string(p.Badge),
			Color:            p.Color,
			HasPublicURL:     p.HasPublicURL,
			LastHeartbeatAt:  lastHeartbeat,
			FirstHeartbeatAt: firstHeartbeat,
		})
	}

	if err := s.db.UpsertProjects(stored); err != nil {
		slog.Error("failed to store projects", "error", err)
		return
	}
	slog.Info("synced projects", "count", len(stored))
}

// syncUserAgents refreshes the stored user agents, which tell the AI model versions behind heartbeats.
func (s *Syncer) syncUserAgents() {
	agents, err := s.client.GetUserAgents()
	if err != nil {
		slog.Error("failed to sync user agents", "error", err)
		return
	}

	stored := make([]database.UserAgent, 0, len(agents))
	for _, a := range agents {
		stored = append(stored, database.UserAgent{
			ID:                 a.ID,
			Value:              a.Value,
			Editor:             a.Editor,
			OS:                 a.OS,
			Version:            a.Version,
			AIModel:            a.AIModel,
			AIModelVersion:     a.AIModelVersion,
			AIModelComplexity:  a.AIModelComplexity,
			IsBrowserExtension: a.IsBrowserExtension,
			IsDesktopApp:       a.IsDesktopApp,
			FirstSeenAt:        a.CreatedAt,
			LastSeenAt:         a.LastSeenAt,
		})
	}

	if err := s.db.UpsertUserAgents(stored); err != nil {
		slog.Error("failed to store user agents", "error", err)
		return
	}
	slog.Info("synced user agents", "count", len(stored))
}
