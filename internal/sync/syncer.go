package sync

import (
	"log/slog"
	"time"

	"github.com/charlie0129/wakatime-sync-go/internal/config"
	"github.com/charlie0129/wakatime-sync-go/internal/database"
	"github.com/charlie0129/wakatime-sync-go/internal/wakatime"
	"github.com/robfig/cron/v3"
)

type Syncer struct {
	cfg    *config.Config
	db     *database.DB
	client *wakatime.Client
	cron   *cron.Cron
}

func NewSyncer(cfg *config.Config, db *database.DB) *Syncer {
	return &Syncer{
		cfg:    cfg,
		db:     db,
		client: wakatime.NewClient(cfg.WakaTimeAPI, cfg.ProxyURL),
	}
}

func (s *Syncer) StartScheduler() {
	// Sync yesterday's data immediately on startup
	s.SyncYesterday()

	// Set up cron scheduler with configured timezone
	loc := s.cfg.GetTimezone()
	s.cron = cron.New(cron.WithLocation(loc))

	_, err := s.cron.AddFunc(s.cfg.SyncSchedule, func() {
		slog.Info("running scheduled sync", "schedule", s.cfg.SyncSchedule)
		s.SyncYesterday()
	})
	if err != nil {
		slog.Error("failed to add cron job, falling back to 24h ticker", "schedule", s.cfg.SyncSchedule, "error", err)
		// Fallback to simple ticker if cron expression is invalid
		go func() {
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				s.SyncYesterday()
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

func (s *Syncer) SyncYesterday() {
	yesterday := time.Now().AddDate(0, 0, -1)
	if err := s.SyncDay(yesterday); err != nil {
		slog.Error("failed to sync yesterday's data", "date", yesterday.Format("2006-01-02"), "error", err)
	}
}

func (s *Syncer) SyncDays(days int) error {
	end := time.Now().AddDate(0, 0, -1)
	start := time.Now().AddDate(0, 0, -days)

	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if err := s.SyncDay(d); err != nil {
			slog.Error("failed to sync day", "date", d.Format("2006-01-02"), "error", err)
			continue
		}
	}
	return nil
}

func (s *Syncer) SyncDateRange(start, end time.Time) error {
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if err := s.SyncDay(d); err != nil {
			slog.Error("failed to sync day", "date", d.Format("2006-01-02"), "error", err)
			continue
		}
	}
	return nil
}

func (s *Syncer) SyncDay(day time.Time) error {
	dateStr := day.Format("2006-01-02")
	slog.Info("syncing data", "date", dateStr)

	// Sync summaries first (this gives us the grand total and breakdowns)
	totalSeconds, err := s.syncSummary(day)
	if err != nil {
		slog.Error("failed to sync summary", "date", dateStr, "error", err)
		s.db.RecordSync(day, 0, "failed")
		return err
	}

	// Sync durations
	if err := s.syncDurations(day); err != nil {
		slog.Error("failed to sync durations", "date", dateStr, "error", err)
	}

	// Sync heartbeats
	if err := s.syncHeartbeats(day); err != nil {
		slog.Error("failed to sync heartbeats", "date", dateStr, "error", err)
	}

	// Record successful sync
	s.db.RecordSync(day, totalSeconds, "success")
	slog.Info("sync completed", "date", dateStr, "total_seconds", totalSeconds)

	return nil
}

func (s *Syncer) syncSummary(day time.Time) (float64, error) {
	resp, err := s.client.GetSummaries(day, day)
	if err != nil {
		return 0, err
	}

	if len(resp.Data) == 0 {
		slog.Info("no summary data for day", "date", day.Format("2006-01-02"))
		return 0, nil
	}

	summary := resp.Data[0]
	totalSeconds := summary.GrandTotal.TotalSeconds

	// Check if we already have this day with same total
	existing, err := s.db.GetDaySummary(day)
	if err != nil {
		return 0, err
	}
	if existing != nil && existing.TotalSeconds == totalSeconds {
		slog.Info("summary already up to date", "date", day.Format("2006-01-02"))
		return totalSeconds, nil
	}

	// Save grand total
	if err := s.db.UpsertDaySummary(day, totalSeconds); err != nil {
		return 0, err
	}

	// Delete existing stats for this day
	if err := s.db.DeleteDayStatsByDay(day); err != nil {
		return 0, err
	}

	// Collect all stats
	var stats []database.DayStats

	// Categories
	for _, item := range summary.Categories {
		stats = append(stats, database.DayStats{
			Day:          day,
			Type:         "category",
			Name:         item.Name,
			TotalSeconds: item.TotalSeconds,
		})
	}

	// Languages
	for _, item := range summary.Languages {
		stats = append(stats, database.DayStats{
			Day:          day,
			Type:         "language",
			Name:         item.Name,
			TotalSeconds: item.TotalSeconds,
		})
	}

	// Editors
	for _, item := range summary.Editors {
		stats = append(stats, database.DayStats{
			Day:          day,
			Type:         "editor",
			Name:         item.Name,
			TotalSeconds: item.TotalSeconds,
		})
	}

	// Operating Systems
	for _, item := range summary.OperatingSystems {
		stats = append(stats, database.DayStats{
			Day:          day,
			Type:         "os",
			Name:         item.Name,
			TotalSeconds: item.TotalSeconds,
		})
	}

	// Projects
	for _, item := range summary.Projects {
		stats = append(stats, database.DayStats{
			Day:          day,
			Type:         "project",
			Name:         item.Name,
			TotalSeconds: item.TotalSeconds,
		})
	}

	// Dependencies
	for _, item := range summary.Dependencies {
		stats = append(stats, database.DayStats{
			Day:          day,
			Type:         "dependency",
			Name:         item.Name,
			TotalSeconds: item.TotalSeconds,
		})
	}

	// Machines
	for _, item := range summary.Machines {
		stats = append(stats, database.DayStats{
			Day:          day,
			Type:         "machine",
			Name:         item.Name,
			TotalSeconds: item.TotalSeconds,
		})
	}

	if len(stats) > 0 {
		if err := s.db.InsertDayStats(stats); err != nil {
			return 0, err
		}
	}

	slog.Info("synced summary", "date", day.Format("2006-01-02"), "total_seconds", totalSeconds, "stats_count", len(stats))
	return totalSeconds, nil
}

func (s *Syncer) syncDurations(day time.Time) error {
	resp, err := s.client.GetDurations(day)
	if err != nil {
		return err
	}

	if len(resp.Data) == 0 {
		slog.Info("no duration data for day", "date", day.Format("2006-01-02"))
		return nil
	}

	// Check if we already have the same number of durations
	existingCount, err := s.db.CountDurationsByDay(day)
	if err != nil {
		return err
	}
	if existingCount >= len(resp.Data) {
		slog.Info("durations already up to date", "date", day.Format("2006-01-02"))
		return nil
	}

	// Delete existing and insert new
	if err := s.db.DeleteDurationsByDay(day); err != nil {
		return err
	}

	var durations []database.Duration
	for _, d := range resp.Data {
		durations = append(durations, database.Duration{
			Day:          day,
			Project:      d.Project,
			StartTime:    d.Time,
			Duration:     d.Duration,
			Dependencies: d.Dependencies,
		})
	}

	if err := s.db.InsertDurations(durations); err != nil {
		return err
	}

	// Also sync project-level durations for each project
	projects := make(map[string]bool)
	for _, d := range resp.Data {
		if d.Project != "" {
			projects[d.Project] = true
		}
	}

	var projectDurations []database.ProjectDuration
	for project := range projects {
		projResp, err := s.client.GetDurationsWithProject(day, project)
		if err != nil {
			slog.Error("failed to get project durations", "project", project, "error", err)
			continue
		}
		for _, d := range projResp.Data {
			projectDurations = append(projectDurations, database.ProjectDuration{
				Day:          day,
				Project:      project,
				Entity:       d.Entity,
				Language:     d.Language,
				Branch:       d.Branch,
				Type:         d.Type,
				StartTime:    d.Time,
				Duration:     d.Duration,
				Dependencies: d.Dependencies,
			})
		}
	}

	if len(projectDurations) > 0 {
		if err := s.db.DeleteProjectDurationsByDay(day); err != nil {
			return err
		}
		if err := s.db.InsertProjectDurations(projectDurations); err != nil {
			return err
		}
	}

	slog.Info("synced durations", "date", day.Format("2006-01-02"), "count", len(durations), "project_count", len(projectDurations))
	return nil
}

func (s *Syncer) syncHeartbeats(day time.Time) error {
	resp, err := s.client.GetHeartbeats(day)
	if err != nil {
		return err
	}

	if len(resp.Data) == 0 {
		slog.Info("no heartbeat data for day", "date", day.Format("2006-01-02"))
		return nil
	}

	// Check if we already have the same number of heartbeats
	existingCount, err := s.db.CountHeartbeatsByDay(day)
	if err != nil {
		return err
	}
	if existingCount >= len(resp.Data) {
		slog.Info("heartbeats already up to date", "date", day.Format("2006-01-02"))
		return nil
	}

	// Delete existing and insert new
	if err := s.db.DeleteHeartbeatsByDay(day); err != nil {
		return err
	}

	var heartbeats []database.HeartBeat
	for _, h := range resp.Data {
		heartbeats = append(heartbeats, database.HeartBeat{
			Day:       day,
			Entity:    h.Entity,
			Type:      h.Type,
			Category:  h.Category,
			Time:      h.Time,
			Project:   h.Project,
			Branch:    h.Branch,
			Language:  h.Language,
			IsWrite:   h.IsWrite,
			MachineID: h.MachineNameID,
			Lines:     h.Lines,
			LineNo:    h.LineNo,
			CursorPos: h.CursorPos,
		})
	}

	if err := s.db.InsertHeartbeats(heartbeats); err != nil {
		return err
	}

	slog.Info("synced heartbeats", "date", day.Format("2006-01-02"), "count", len(heartbeats))
	return nil
}

func (s *Syncer) SyncProjects() error {
	resp, err := s.client.GetProjects("")
	if err != nil {
		return err
	}

	for _, p := range resp.Data {
		var lastHeartbeat, firstHeartbeat time.Time
		if p.LastHeartbeatAt != "" {
			lastHeartbeat, _ = time.Parse(time.RFC3339, p.LastHeartbeatAt)
		}
		if p.FirstHeartbeatAt != "" {
			firstHeartbeat, _ = time.Parse(time.RFC3339, p.FirstHeartbeatAt)
		}

		if err := s.db.UpsertProject(&database.Project{
			UUID:             p.ID,
			Name:             p.Name,
			Repository:       p.Repository,
			Badge:            p.Badge,
			Color:            p.Color,
			HasPublicURL:     p.HasPublicURL,
			LastHeartbeatAt:  lastHeartbeat,
			FirstHeartbeatAt: firstHeartbeat,
		}); err != nil {
			slog.Error("failed to upsert project", "project", p.Name, "error", err)
		}
	}

	slog.Info("synced projects", "count", len(resp.Data))
	return nil
}
