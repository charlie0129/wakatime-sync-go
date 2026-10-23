package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/charlie0129/wakatime-sync-go/internal/config"
	"github.com/charlie0129/wakatime-sync-go/internal/database"
	"github.com/charlie0129/wakatime-sync-go/internal/sync"
)

// newTestHandler returns a handler on an empty database, at 00:30 on now in Asia/Shanghai.
func newTestHandler(t *testing.T, now string) (*Handler, *database.DB) {
	t.Helper()
	db, err := database.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	cfg := &config.Config{Timezone: "Asia/Shanghai", StartDate: "2026-01-01", SyncLookbackDays: 3, WakaTimeBaseURL: "http://127.0.0.1:0"}
	h := NewHandler(cfg, db, sync.NewSyncer(cfg, db))
	day, err := time.ParseInLocation(dayLayout, now, cfg.GetTimezone())
	if err != nil {
		t.Fatal(err)
	}
	// Just after midnight in the configured timezone, still the previous day in UTC
	h.now = func() time.Time { return day.Add(30 * time.Minute) }
	return h, db
}

func get(t *testing.T, h http.HandlerFunc, url string, dest interface{}) int {
	t.Helper()
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, url, nil))
	if dest != nil && rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
			t.Fatalf("decoding %s: %v", url, err)
		}
	}
	return rec.Code
}

func TestRangePresets(t *testing.T) {
	tests := []struct {
		today, preset, start, end string
	}{
		// 2026-10-22 is a Thursday
		{"2026-10-22", "yesterday", "2026-10-21", "2026-10-21"},
		{"2026-10-22", "7d", "2026-10-15", "2026-10-21"},
		{"2026-10-22", "30d", "2026-09-22", "2026-10-21"},
		{"2026-10-22", "90d", "2026-07-24", "2026-10-21"},
		{"2026-10-22", "12m", "2025-10-22", "2026-10-21"},
		{"2026-10-22", "this-week", "2026-10-19", "2026-10-21"},
		{"2026-10-22", "last-week", "2026-10-12", "2026-10-18"},
		{"2026-10-22", "this-month", "2026-10-01", "2026-10-21"},
		{"2026-10-22", "last-month", "2026-09-01", "2026-09-30"},
		{"2026-10-22", "this-year", "2026-01-01", "2026-10-21"},
		{"2026-10-22", "last-year", "2025-01-01", "2025-12-31"},
		// On a Monday, this week has no complete day yet
		{"2026-10-19", "this-week", "2026-10-19", "2026-10-19"},
		{"2026-10-01", "this-month", "2026-10-01", "2026-10-01"},
		{"2026-03-15", "last-month", "2026-02-01", "2026-02-28"},
	}
	for _, tt := range tests {
		t.Run(tt.today+" "+tt.preset, func(t *testing.T) {
			h, _ := newTestHandler(t, tt.today)
			preset, start, end, err := h.resolveRange(httptest.NewRequest(http.MethodGet, "/?range="+tt.preset, nil))
			if err != nil {
				t.Fatal(err)
			}
			if preset != tt.preset || start.Format(dayLayout) != tt.start || end.Format(dayLayout) != tt.end {
				t.Errorf("got %s from %s to %s, want %s to %s", preset, start.Format(dayLayout), end.Format(dayLayout), tt.start, tt.end)
			}
		})
	}
}

func TestResolveRangeErrors(t *testing.T) {
	h, _ := newTestHandler(t, "2026-10-22")
	for _, query := range []string{
		"range=fortnight",
		"start=2026-10-01",
		"start=2026-10-01&end=2026-13-01",
		"start=2026-10-21&end=2026-10-01",
	} {
		if _, _, _, err := h.resolveRange(httptest.NewRequest(http.MethodGet, "/?"+query, nil)); err == nil {
			t.Errorf("%s: no error", query)
		}
	}
}

func TestGroupSpans(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	day := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	midnight := float64(time.Date(2026, 10, 21, 0, 0, 0, 0, loc).Unix())
	span := func(from, duration float64, ai bool) database.DurationSpan {
		return database.DurationSpan{Day: day, StartTime: midnight + from, Duration: duration, AI: ai}
	}

	days := groupSpans([]database.DurationSpan{
		span(-600, 1200, false),  // starts the day before
		span(3600, 600, true),    // merged with the next one, less than a minute apart
		span(4230, 300, true),    //
		span(4600, 100, false),   // a different kind is not merged
		span(86000, 1000, false), // ends the next day
	}, day, day.AddDate(0, 0, 1), loc)

	if len(days) != 2 || days[0].Date != "2026-10-21" || days[1].Date != "2026-10-22" || len(days[1].Spans) != 0 {
		t.Fatalf("days = %+v", days)
	}
	want := [][3]int{{0, 600, 0}, {3600, 4530, 1}, {4600, 4700, 0}, {86000, 86400, 0}}
	if got := days[0].Spans; len(got) != len(want) {
		t.Fatalf("spans = %v, want %v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("spans = %v, want %v", got, want)
				break
			}
		}
	}
}

func TestDashboard(t *testing.T) {
	h, db := newTestHandler(t, "2026-10-22")
	save := func(date string, seconds float64, ai database.AIStats, models ...database.DayAIModel) {
		t.Helper()
		day, _ := time.Parse(dayLayout, date)
		for i := range models {
			models[i].Day = day
		}
		data := &database.DayData{
			Day:     day,
			Summary: &database.DaySummary{Day: day, TotalSeconds: seconds, AIStats: ai},
			Stats: []database.DayStats{
				{Day: day, Type: "project", Name: "app", TotalSeconds: seconds, AIStats: ai},
				{Day: day, Type: "category", Name: "AI Coding", TotalSeconds: seconds / 2},
			},
			AIModels: models,
		}
		if _, err := db.SaveDay(data, false); err != nil {
			t.Fatal(err)
		}
	}
	save("2026-10-20", 3600, database.AIStats{AIAdditions: 30, HumanAdditions: 10, AIInputTokens: 100, AIModelTotalCost: 2},
		database.DayAIModel{Type: "total", Model: "Opus", Lines: 30, InputTokens: 100, Cost: 2})
	save("2026-10-21", 7200, database.AIStats{})
	// In the previous period
	save("2026-10-13", 1800, database.AIStats{AIAdditions: 5, AIModelTotalCost: 1})

	var resp struct {
		Range        string  `json:"range"`
		Start        string  `json:"start"`
		End          string  `json:"end"`
		Today        string  `json:"today"`
		TotalSeconds float64 `json:"total_seconds"`
		ActiveDays   int     `json:"active_days"`
		Days         []database.DayActivity
		AI           aiSummary
		Previous     periodTotals
		Projects     []breakdownItem
		Categories   []breakdownItem
		AIModelNames []string `json:"ai_model_names"`
	}
	if code := get(t, h.getDashboard, "/api/v1/dashboard?range=7d", &resp); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}

	if resp.Range != "7d" || resp.Start != "2026-10-15" || resp.End != "2026-10-21" || resp.Today != "2026-10-22" {
		t.Errorf("range = %s from %s to %s, today %s", resp.Range, resp.Start, resp.End, resp.Today)
	}
	if resp.TotalSeconds != 10800 || resp.ActiveDays != 2 || len(resp.Days) != 7 {
		t.Errorf("total = %v over %d active days, %d days", resp.TotalSeconds, resp.ActiveDays, len(resp.Days))
	}
	if d := resp.Days[5]; d.Date != "2026-10-20" || d.AILines != 30 || d.HumanLines != 10 || d.AICodingSeconds != 1800 || d.ModelCosts["Opus"] != 2 {
		t.Errorf("2026-10-20 = %+v", d)
	}
	if resp.AI.Lines != 30 || resp.AI.Cost != 2 || resp.AI.CodingSeconds != 5400 {
		t.Errorf("ai = %+v", resp.AI)
	}
	if p := resp.Previous; p.Start != "2026-10-08" || p.End != "2026-10-14" || p.TotalSeconds != 1800 || p.AI.Lines != 5 {
		t.Errorf("previous = %+v", p)
	}
	if len(resp.Projects) != 1 || resp.Projects[0].AILines != 30 || resp.Projects[0].AICost != 2 {
		t.Errorf("projects = %+v", resp.Projects)
	}
	if len(resp.AIModelNames) != 1 || resp.AIModelNames[0] != "Opus" {
		t.Errorf("ai_model_names = %v", resp.AIModelNames)
	}

	if code := get(t, h.getDashboard, "/api/v1/dashboard?start=2026-10-21&end=2026-10-01", nil); code != http.StatusBadRequest {
		t.Errorf("reversed range: status %d, want 400", code)
	}
}

func TestSyncStatus(t *testing.T) {
	h, db := newTestHandler(t, "2026-10-22")
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	if _, err := db.SaveDay(&database.DayData{Day: day}, false); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSyncFailure(day.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}

	var resp struct {
		LastSyncedDay string   `json:"last_synced_day"`
		FailedDays    []string `json:"failed_days"`
		Running       bool     `json:"running"`
		Today         string   `json:"today"`
		Timezone      string   `json:"timezone"`
	}
	if code := get(t, h.getSyncStatus, "/api/v1/sync/status", &resp); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if resp.LastSyncedDay != "2026-10-20" || len(resp.FailedDays) != 1 || resp.FailedDays[0] != "2026-10-21" ||
		resp.Running || resp.Today != "2026-10-22" || resp.Timezone != "Asia/Shanghai" {
		t.Errorf("status = %+v", resp)
	}
}
