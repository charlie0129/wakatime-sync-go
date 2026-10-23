package sync

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	gosync "sync"
	"testing"
	"time"

	"github.com/charlie0129/wakatime-sync-go/internal/config"
	"github.com/charlie0129/wakatime-sync-go/internal/database"
)

// fakeWakaTime serves canned WakaTime API responses, keyed by path and query string.
type fakeWakaTime struct {
	mu        gosync.Mutex
	responses map[string]string
	statuses  map[string]int
	requests  map[string]int
}

func newFakeWakaTime() *fakeWakaTime {
	return &fakeWakaTime{responses: map[string]string{}, statuses: map[string]int{}, requests: map[string]int{}}
}

// The api_key parameter is ignored when matching, like the Authorization header was before it.
func (f *fakeWakaTime) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	query := r.URL.Query()
	query.Del("api_key")
	key := r.URL.Path + "?" + query.Encode()
	f.requests[key]++
	if status, ok := f.statuses[key]; ok {
		w.WriteHeader(status)
		return
	}
	body, ok := f.responses[key]
	if !ok {
		http.Error(w, "unexpected request "+key, http.StatusNotFound)
		return
	}
	w.Write([]byte(body))
}

func (f *fakeWakaTime) set(key, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses[key] = body
}

func (f *fakeWakaTime) fail(key string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses[key] = status
}

func (f *fakeWakaTime) count(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[key]
}

func summariesKey(day string) string {
	return "/users/current/summaries?" + url.Values{"start": {day}, "end": {day}}.Encode()
}

func durationsKey(day string) string {
	return "/users/current/durations?" + url.Values{"date": {day}}.Encode()
}

func projectDurationsKey(day, project string) string {
	return "/users/current/durations?" + url.Values{"date": {day}, "project": {project}, "slice_by": {"entity"}}.Encode()
}

func heartbeatsKey(day string) string {
	return "/users/current/heartbeats?" + url.Values{"date": {day}}.Encode()
}

func projectsKey(page int) string {
	return "/users/current/projects?" + url.Values{"page": {fmt.Sprint(page)}}.Encode()
}

// serveDay makes the fake serve a day with one project and the given amount of coding time.
func (f *fakeWakaTime) serveDay(day string, seconds float64) {
	f.set(summariesKey(day), fmt.Sprintf(`{"data": [{
		"grand_total": {"total_seconds": %[1]v},
		"categories": [{"name": "Coding", "total_seconds": %[1]v}],
		"projects": [{"name": "app", "total_seconds": %[1]v}],
		"range": {"date": %[2]q}
	}]}`, seconds, day))
	f.set(durationsKey(day), fmt.Sprintf(`{"data": [{"project": "app", "time": 1792540000, "duration": %v}]}`, seconds))
	f.set(projectDurationsKey(day, "app"), fmt.Sprintf(`{"data": [
		{"project": "app", "entity": "main.go", "language": "Go", "time": 1792540000, "duration": %v}
	]}`, seconds))
	f.set(heartbeatsKey(day), `{"data": [
		{"entity": "main.go", "type": "file", "category": "coding", "time": 1792540000},
		{"entity": "main.go", "type": "file", "category": "coding", "time": 1792540100}
	]}`)
}

func newTestSyncer(t *testing.T, fake *fakeWakaTime) (*Syncer, *database.DB) {
	t.Helper()
	db, err := database.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	cfg := &config.Config{
		WakaTimeAPI:      "test",
		WakaTimeBaseURL:  server.URL,
		StartDate:        "2026-10-01",
		SyncLookbackDays: 3,
		Timezone:         "Asia/Shanghai",
	}
	s := NewSyncer(cfg, db)
	s.retryDelay = 0
	return s, db
}

func formatDays(days []time.Time) []string {
	out := make([]string, len(days))
	for i, d := range days {
		out[i] = d.Format(dayLayout)
	}
	return out
}

func TestDueDays(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	// Just after midnight in the configured timezone, still the previous day in UTC
	now := time.Date(2026, 10, 22, 0, 30, 0, 0, loc)

	day := func(s string) time.Time {
		d, err := time.Parse(dayLayout, s)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}

	tests := []struct {
		name      string
		startDate string
		succeeded []string
		failed    []string
		want      []string
	}{
		{
			name:      "fresh database backfills from start_date",
			startDate: "2026-10-17",
			want:      []string{"2026-10-17", "2026-10-18", "2026-10-19", "2026-10-20", "2026-10-21"},
		},
		{
			name:      "up to date database re-syncs the lookback window",
			startDate: "2026-10-01",
			succeeded: []string{"2026-10-20", "2026-10-21"},
			want:      []string{"2026-10-19", "2026-10-20", "2026-10-21"},
		},
		{
			name:      "catches up after the last successful sync",
			startDate: "2026-10-01",
			succeeded: []string{"2026-10-15"},
			want:      []string{"2026-10-16", "2026-10-17", "2026-10-18", "2026-10-19", "2026-10-20", "2026-10-21"},
		},
		{
			name:      "retries failed days, but not before start_date",
			startDate: "2026-10-01",
			succeeded: []string{"2026-10-21"},
			failed:    []string{"2026-09-30", "2026-10-05"},
			want:      []string{"2026-10-05", "2026-10-19", "2026-10-20", "2026-10-21"},
		},
		{
			name:      "lookback window does not reach before start_date",
			startDate: "2026-10-20",
			succeeded: []string{"2026-10-21"},
			want:      []string{"2026-10-20", "2026-10-21"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, db := newTestSyncer(t, newFakeWakaTime())
			s.cfg.StartDate = tt.startDate
			for _, d := range tt.succeeded {
				if _, err := db.SaveDay(&database.DayData{Day: day(d)}, false); err != nil {
					t.Fatal(err)
				}
			}
			for _, d := range tt.failed {
				if err := db.RecordSyncFailure(day(d)); err != nil {
					t.Fatal(err)
				}
			}

			days, err := s.dueDays(now)
			if err != nil {
				t.Fatal(err)
			}
			if got := formatDays(days); fmt.Sprint(got) != fmt.Sprint(tt.want) {
				t.Errorf("dueDays = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSyncDayFailureKeepsStoredData(t *testing.T) {
	fake := newFakeWakaTime()
	s, db := newTestSyncer(t, fake)
	day := s.date(time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC))
	dayStr := day.Format(dayLayout)

	fake.serveDay(dayStr, 3600)
	if err := s.syncDayWithRetry(day, false); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// The re-sync has more data, but one project's durations fail to load
	fake.serveDay(dayStr, 7200)
	fake.fail(projectDurationsKey(dayStr, "app"), http.StatusInternalServerError)
	s.maxRetries = 1
	if err := s.syncDayWithRetry(day, false); err == nil {
		t.Fatal("sync succeeded, want an error")
	}
	if got := fake.count(projectDurationsKey(dayStr, "app")); got != 3 {
		t.Errorf("project durations requested %d times, want 3 (1 sync, then 1 attempt and 1 retry)", got)
	}

	summary, err := db.GetDaySummary(day)
	if err != nil {
		t.Fatal(err)
	}
	if summary == nil || summary.TotalSeconds != 3600 {
		t.Errorf("stored summary = %+v, want the first sync's 3600 seconds", summary)
	}
	durations, err := db.GetProjectDurationsByDay(day, "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(durations) != 1 || durations[0].Duration != 3600 {
		t.Errorf("stored project durations = %+v, want the first sync's", durations)
	}
	failed, err := db.GetFailedSyncDays()
	if err != nil {
		t.Fatal(err)
	}
	if got := formatDays(failed); fmt.Sprint(got) != fmt.Sprint([]string{dayStr}) {
		t.Errorf("failed days = %v, want [%s]", got, dayStr)
	}
}

func TestSyncDayDoesNotRetryPermanentErrors(t *testing.T) {
	fake := newFakeWakaTime()
	s, _ := newTestSyncer(t, fake)
	day := s.date(time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC))
	dayStr := day.Format(dayLayout)

	fake.fail(summariesKey(dayStr), http.StatusPaymentRequired)
	if err := s.syncDayWithRetry(day, false); err == nil {
		t.Fatal("sync succeeded, want an error")
	}
	if got := fake.count(summariesKey(dayStr)); got != 1 {
		t.Errorf("summaries requested %d times, want 1", got)
	}
}

func TestSyncProjectsFollowsPagination(t *testing.T) {
	fake := newFakeWakaTime()
	s, db := newTestSyncer(t, fake)

	fake.set(projectsKey(1), `{"data": [{"id": "1", "name": "app", "repository": null}], "page": 1, "total_pages": 2, "next_page": 2}`)
	fake.set(projectsKey(2), `{"data": [{"id": "2", "name": "lib", "repository": {"full_name": "me/lib"}}], "page": 2, "total_pages": 2, "next_page": null}`)
	s.syncProjects()

	projects, err := db.GetProjects("")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, p := range projects {
		names[p.Name] = p.Repository
	}
	want := map[string]string{"app": "", "lib": `{"full_name": "me/lib"}`}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Errorf("stored projects (name: repository) = %v, want %v", names, want)
	}
}

func TestSyncDayStoresAIStats(t *testing.T) {
	fake := newFakeWakaTime()
	s, db := newTestSyncer(t, fake)
	day := s.date(time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC))
	dayStr := day.Format(dayLayout)

	aiFields := `"ai_additions": 120, "ai_deletions": 30, "human_additions": 40, "human_deletions": 10,
		"ai_input_tokens": 1000, "ai_cached_input_tokens": 50000, "ai_output_tokens": 200,
		"ai_sessions": 2, "ai_prompt_events_total": 5, "ai_prompt_length_sum": 600, "ai_prompt_length_avg": 120`
	breakdown := `"ai_model_breakdown": [
			{"name": "Opus", "lines": 140, "cost": 3.5, "input_tokens": 900, "output_tokens": 150, "cached_input_tokens": 45000},
			{"name": "GPT", "lines": 10, "cost": 0.5, "input_tokens": 100, "output_tokens": 50, "cached_input_tokens": 5000}
		],
		"ai_model_line_changes": {"Opus": 140, "GPT": 10}, "ai_model_costs": {"Opus": 3.5, "GPT": 0.5}`
	fake.set(summariesKey(dayStr), `{"data": [{
		"grand_total": {"total_seconds": 3600, "ai_model_total_cost": 4, `+aiFields+`, `+breakdown+`},
		"categories": [{"name": "AI Coding", "total_seconds": 3000}, {"name": "Coding", "total_seconds": 600}],
		"projects": [{"name": "app", "total_seconds": 3600, "ai_model_total_cost": 4, `+aiFields+`, `+breakdown+`}],
		"editors": [{"name": "Claude Code", "total_seconds": 3600, "ai_model_total_cost": 4, `+aiFields+`, `+breakdown+`}],
		"range": {"date": "`+dayStr+`"}
	}]}`)
	// Like WakaTime, durations lack ai_model_total_cost, and their breakdown may lack lines and cost
	duration := `{"project": "app", "time": 1792540000, "duration": 3600, ` + aiFields + `,
		"ai_model_breakdown": [{"name": "Opus", "input_tokens": 900, "output_tokens": 150, "cached_input_tokens": 45000}],
		"ai_model_line_changes": {"Opus": 140, "GPT": 10}, "ai_model_costs": {"Opus": 3.5, "GPT": 0.5}}`
	fake.set(durationsKey(dayStr), `{"data": [`+duration+`]}`)
	fake.set(projectDurationsKey(dayStr, "app"), `{"data": [`+duration+`]}`)
	fake.set(heartbeatsKey(dayStr), `{"data": [{
		"id": "hb-1", "entity": "main.go", "type": "file", "category": "ai coding", "time": 1792540000,
		"user_agent_id": "ua-1", "dependencies": ["fmt"], "project_root_count": 4,
		"ai_line_changes": null, "human_line_changes": 3, "ai_session": "session-1",
		"ai_input_tokens": 1000, "ai_cached_input_tokens": 50000, "ai_output_tokens": 200, "ai_prompt_length": 120,
		"ai_subscription_plan": null, "is_write": true
	}]}`)
	fake.set("/users/current/user_agents?page=1", `{"data": [{
		"id": "ua-1", "value": "wakatime/v2 claude-code/2", "editor": "Claude Code",
		"ai_model": "Opus", "ai_model_version": "5-5", "ai_model_complexity": null
	}], "total_pages": 1, "next_page": null}`)

	if err := s.SyncDay(day, false); err != nil {
		t.Fatalf("SyncDay: %v", err)
	}
	s.syncUserAgents()

	queries := []struct {
		query string
		want  string
	}{
		{"SELECT ai_additions, ai_deletions, human_additions, ai_cached_input_tokens, ai_model_total_cost, ai_sessions FROM day_summaries", "120|30|40|50000|4|2"},
		{"SELECT name, ai_additions, ai_model_total_cost FROM day_stats WHERE type IN ('project', 'editor') ORDER BY type", "Claude Code|120|4"},
		{"SELECT COUNT(*) FROM day_ai_models", "6"},
		{"SELECT model, lines, cached_input_tokens, cost FROM day_ai_models WHERE type = 'total' AND model = 'Opus'", "Opus|140|45000|3.5"},
		{"SELECT ai_additions, ai_model_total_cost, json(ai_model_breakdown) FROM durations", `120|4|[{"name":"Opus","lines":140,"input_tokens":900,"cached_input_tokens":45000,"output_tokens":150,"cost":3.5},{"name":"GPT","lines":10,"input_tokens":0,"cached_input_tokens":0,"output_tokens":0,"cost":0.5}]`},
		{"SELECT ai_model_total_cost FROM project_durations", "4"},
		{"SELECT wakatime_id, user_agent_id, project_root_count, json(dependencies), ai_line_changes, human_line_changes, ai_session, ai_cached_input_tokens, ai_prompt_length FROM heartbeats", `hb-1|ua-1|4|["fmt"]|0|3|session-1|50000|120`},
		{"SELECT editor, ai_model, ai_model_version FROM user_agents", "Claude Code|Opus|5-5"},
	}
	for _, q := range queries {
		rows, err := db.Query(q.query)
		if err != nil {
			t.Fatalf("%s: %v", q.query, err)
		}
		var got []string
		cols, _ := rows.Columns()
		for rows.Next() {
			values := make([]interface{}, len(cols))
			dest := make([]interface{}, len(cols))
			for i := range values {
				dest[i] = &values[i]
			}
			if err := rows.Scan(dest...); err != nil {
				t.Fatal(err)
			}
			row := ""
			for i, v := range values {
				if i > 0 {
					row += "|"
				}
				if b, ok := v.([]byte); ok {
					v = string(b)
				}
				row += fmt.Sprint(v)
			}
			got = append(got, row)
		}
		rows.Close()
		if len(got) == 0 || got[0] != q.want {
			t.Errorf("%s\n got: %v\nwant: %s", q.query, got, q.want)
		}
	}
}

func TestRunStatus(t *testing.T) {
	fake := newFakeWakaTime()
	s, _ := newTestSyncer(t, fake)
	fake.serveDay("2026-10-20", 3600)
	fake.fail(summariesKey("2026-10-21"), http.StatusPaymentRequired)
	fake.set(projectsKey(1), `{"data": [], "total_pages": 1, "next_page": null}`)
	fake.set("/users/current/user_agents?page=1", `{"data": [], "total_pages": 1, "next_page": null}`)

	if running, run := s.Status(); running || run != nil {
		t.Fatalf("before any sync: running = %v, run = %+v", running, run)
	}

	start := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	if !s.StartSync(start, start.AddDate(0, 0, 1), false) {
		t.Fatal("StartSync refused to start")
	}
	if s.StartSync(start, start, false) {
		t.Error("a second sync started while the first one was running")
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		running, run := s.Status()
		if !running && run != nil && run.FinishedAt != nil {
			if run.Days != 2 || run.Done != 2 || run.Failed != 1 {
				t.Errorf("run = %+v, want 2 days done, 1 failed", run)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("sync did not finish, run = %+v", run)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
