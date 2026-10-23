package database

import (
	"database/sql"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

var testDay = time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)

// testDayData returns data for testDay whose size grows with scale.
func testDayData(scale float64) *DayData {
	data := &DayData{
		Day:     testDay,
		Summary: &DaySummary{Day: testDay, TotalSeconds: 100 * scale},
		Stats: []DayStats{
			{Day: testDay, Type: "project", Name: "app", TotalSeconds: 100 * scale},
		},
	}
	for i := 0; i < int(scale*2); i++ {
		data.Durations = append(data.Durations, Duration{Day: testDay, Project: "app", StartTime: float64(i * 100), Duration: 50, Dependencies: "[]"})
		data.ProjectDurations = append(data.ProjectDurations, ProjectDuration{Day: testDay, Project: "app", Entity: "main.go", StartTime: float64(i * 100), Duration: 50, Dependencies: "[]"})
		data.Heartbeats = append(data.Heartbeats, HeartBeat{Day: testDay, Entity: "main.go", Type: "file", Time: float64(i * 100)})
	}
	return data
}

type storedSizes struct {
	summarySeconds, statSeconds, durationSeconds, projectDurationSeconds float64
	heartbeats                                                           int
	syncStatus                                                           string
}

func readSizes(t *testing.T, db *DB) storedSizes {
	t.Helper()
	day := testDay.Format("2006-01-02")
	var s storedSizes
	queries := []struct {
		query string
		dest  interface{}
	}{
		{"SELECT COALESCE(SUM(total_seconds), 0) FROM day_summaries WHERE day = ?", &s.summarySeconds},
		{"SELECT COALESCE(SUM(total_seconds), 0) FROM day_stats WHERE day = ?", &s.statSeconds},
		{"SELECT COALESCE(SUM(duration), 0) FROM durations WHERE day = ?", &s.durationSeconds},
		{"SELECT COALESCE(SUM(duration), 0) FROM project_durations WHERE day = ?", &s.projectDurationSeconds},
		{"SELECT COUNT(*) FROM heartbeats WHERE day = ?", &s.heartbeats},
		{"SELECT COALESCE(MAX(status), '') FROM sync_log WHERE day = ?", &s.syncStatus},
	}
	for _, q := range queries {
		if err := db.QueryRow(q.query, day).Scan(q.dest); err != nil {
			t.Fatalf("%s: %v", q.query, err)
		}
	}
	return s
}

func TestSaveDay(t *testing.T) {
	full := storedSizes{summarySeconds: 200, statSeconds: 200, durationSeconds: 200, projectDurationSeconds: 200, heartbeats: 4, syncStatus: "success"}
	half := storedSizes{summarySeconds: 100, statSeconds: 100, durationSeconds: 100, projectDurationSeconds: 100, heartbeats: 2, syncStatus: "success"}

	tests := []struct {
		name     string
		resync   *DayData
		force    bool
		wantKept []string
		want     storedSizes
	}{
		{
			name:     "smaller data keeps stored data",
			resync:   testDayData(1),
			wantKept: []string{"summary", "durations", "project_durations", "heartbeats"},
			want:     full,
		},
		{
			name:   "smaller data with force overwrites",
			resync: testDayData(1),
			force:  true,
			want:   half,
		},
		{
			name:   "equal data is replaced",
			resync: testDayData(2),
			want:   full,
		},
		{
			name:   "larger data is replaced",
			resync: testDayData(3),
			want:   storedSizes{summarySeconds: 300, statSeconds: 300, durationSeconds: 300, projectDurationSeconds: 300, heartbeats: 6, syncStatus: "success"},
		},
		{
			name:     "missing summary leaves stored summary untouched",
			resync:   &DayData{Day: testDay},
			wantKept: []string{"durations", "project_durations", "heartbeats"},
			want:     full,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			if _, err := db.SaveDay(testDayData(2), false); err != nil {
				t.Fatalf("initial SaveDay: %v", err)
			}

			result, err := db.SaveDay(tt.resync, tt.force)
			if err != nil {
				t.Fatalf("SaveDay: %v", err)
			}
			if !slices.Equal(result.Kept, tt.wantKept) {
				t.Errorf("Kept = %v, want %v", result.Kept, tt.wantKept)
			}
			if got := readSizes(t, db); got != tt.want {
				t.Errorf("stored sizes = %+v, want %+v", got, tt.want)
			}
			if result.TotalSeconds != tt.want.summarySeconds {
				t.Errorf("TotalSeconds = %v, want %v", result.TotalSeconds, tt.want.summarySeconds)
			}
		})
	}
}

func TestSaveDayIsAtomic(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.SaveDay(testDayData(2), false); err != nil {
		t.Fatalf("initial SaveDay: %v", err)
	}
	before := readSizes(t, db)

	// The last part fails to insert, after the earlier ones were already replaced
	if _, err := db.Exec("CREATE TRIGGER fail_heartbeats BEFORE INSERT ON heartbeats BEGIN SELECT RAISE(ABORT, 'boom'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveDay(testDayData(3), false); err == nil {
		t.Fatal("SaveDay succeeded, want an error")
	}

	if after := readSizes(t, db); after != before {
		t.Errorf("stored sizes changed after a failed save: got %+v, want %+v", after, before)
	}
}

func TestSaveDaySumsStatsWithTheSameName(t *testing.T) {
	db := newTestDB(t)
	data := testDayData(1)
	data.Stats = []DayStats{
		{Day: testDay, Type: "machine", Name: "laptop.local", TotalSeconds: 60},
		{Day: testDay, Type: "machine", Name: "laptop.local", TotalSeconds: 40},
	}
	if _, err := db.SaveDay(data, false); err != nil {
		t.Fatalf("SaveDay: %v", err)
	}

	stats, err := db.GetDayStatsByDayAndType(testDay, "machine")
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 || stats[0].TotalSeconds != 100 {
		t.Errorf("machine stats = %+v, want one entry with 100 seconds", stats)
	}
}

func TestShrinks(t *testing.T) {
	tests := []struct {
		name            string
		newSize, stored []float64
		want            bool
	}{
		{"smaller", []float64{90}, []float64{100}, true},
		{"within tolerance", []float64{99.5}, []float64{100}, false},
		{"equal", []float64{100}, []float64{100}, false},
		{"larger", []float64{110}, []float64{100}, false},
		{"tie broken by smaller second value", []float64{100, 5}, []float64{100, 6}, true},
		{"tie broken by larger second value", []float64{100, 7}, []float64{100, 6}, false},
		{"larger first value wins over smaller second value", []float64{110, 0}, []float64{100, 6}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shrinks(tt.newSize, tt.stored, shrinkTolerance); got != tt.want {
				t.Errorf("shrinks(%v, %v) = %v, want %v", tt.newSize, tt.stored, got, tt.want)
			}
		})
	}
}

func TestSyncLog(t *testing.T) {
	db := newTestDB(t)
	day1 := testDay.AddDate(0, 0, -1)

	if _, err := db.SaveDay(testDayData(1), false); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordSyncFailure(day1); err != nil {
		t.Fatal(err)
	}

	last, err := db.GetLastSyncedDay()
	if err != nil {
		t.Fatal(err)
	}
	if !last.Equal(testDay) {
		t.Errorf("GetLastSyncedDay = %v, want %v", last, testDay)
	}

	failed, err := db.GetFailedSyncDays()
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 || !failed[0].Equal(day1) {
		t.Errorf("GetFailedSyncDays = %v, want [%v]", failed, day1)
	}

	// A failed re-sync keeps the stored total
	if err := db.RecordSyncFailure(testDay); err != nil {
		t.Fatal(err)
	}
	var total float64
	if err := db.QueryRow("SELECT total_seconds FROM sync_log WHERE day = ?", testDay.Format("2006-01-02")).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 100 {
		t.Errorf("sync_log total_seconds = %v after a failed re-sync, want 100", total)
	}
}

func TestSaveDayKeepsAIData(t *testing.T) {
	db := newTestDB(t)
	withAI := testDayData(2)
	withAI.Summary.AIStats = AIStats{AIAdditions: 10, AIInputTokens: 1000}
	withAI.AIModels = []DayAIModel{{Day: testDay, Type: "total", Model: "Opus", Lines: 10, InputTokens: 1000, Cost: 1.5}}
	if _, err := db.SaveDay(withAI, false); err != nil {
		t.Fatal(err)
	}

	// Same time, but WakaTime no longer returns AI data
	result, err := db.SaveDay(testDayData(2), false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Kept, []string{"summary"}) {
		t.Errorf("Kept = %v, want [summary]", result.Kept)
	}

	var tokens, models int64
	if err := db.QueryRow("SELECT ai_input_tokens FROM day_summaries").Scan(&tokens); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM day_ai_models").Scan(&models); err != nil {
		t.Fatal(err)
	}
	if tokens != 1000 || models != 1 {
		t.Errorf("stored ai_input_tokens = %d and %d AI models, want 1000 and 1", tokens, models)
	}
}

func TestMigrateFromBaseSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	// A database created before versioned migrations, with existing data
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range baseSchema {
		if _, err := old.Exec(m); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := old.Exec(`INSERT INTO day_summaries (day, total_seconds) VALUES ('2026-10-21', 100)`); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`INSERT INTO heartbeats (day, entity, type, time) VALUES ('2026-10-21', 'main.go', 'file', 1)`); err != nil {
		t.Fatal(err)
	}
	old.Close()

	db, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != len(schemaMigrations) {
		t.Errorf("user_version = %d, want %d", version, len(schemaMigrations))
	}

	// Existing data is kept, without AI stats
	if got := readSizes(t, db); got.summarySeconds != 100 || got.heartbeats != 1 {
		t.Errorf("stored sizes after migration = %+v", got)
	}
	var aiAdditions sql.NullInt64
	if err := db.QueryRow("SELECT ai_additions FROM day_summaries").Scan(&aiAdditions); err != nil {
		t.Fatal(err)
	}
	if aiAdditions.Valid {
		t.Errorf("ai_additions = %v for existing data, want NULL", aiAdditions.Int64)
	}

	// New data can be saved over it
	if _, err := db.SaveDay(testDayData(2), false); err != nil {
		t.Fatalf("SaveDay after migration: %v", err)
	}

	// Opening an up to date database does not migrate again
	db.Close()
	reopened, err := New(path)
	if err != nil {
		t.Fatalf("reopening: %v", err)
	}
	reopened.Close()
}

func TestReadsHandleDataMigratedFromMySQL(t *testing.T) {
	db := newTestDB(t)
	// The migration script only fills a few columns
	if _, err := db.Exec(`INSERT INTO heartbeats (day, entity, type, time, created_at) VALUES ('2026-10-21', 'main.go', 'file', 1, '2026-10-22T00:05:02')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO project_durations (day, project, entity, start_time, duration, dependencies, created_at)
		VALUES ('2026-10-21', 'app', 'main.go', 1, 60, jsonb('[]'), '2026-10-22T00:05:02')`); err != nil {
		t.Fatal(err)
	}

	heartbeats, err := db.GetHeartbeatsByDay(testDay)
	if err != nil || len(heartbeats) != 1 {
		t.Errorf("GetHeartbeatsByDay = %+v, %v", heartbeats, err)
	}
	durations, err := db.GetProjectDurationsByDay(testDay, "app")
	if err != nil || len(durations) != 1 || durations[0].Dependencies != "[]" {
		t.Errorf("GetProjectDurationsByDay = %+v, %v", durations, err)
	}
}

func TestGetAITotals(t *testing.T) {
	db := newTestDB(t)
	day := func(offset int) time.Time { return testDay.AddDate(0, 0, offset) }

	save := func(d time.Time, sessions int64, aiCodingSeconds float64, heartbeatSessions ...string) {
		t.Helper()
		data := &DayData{
			Day: d,
			Summary: &DaySummary{Day: d, TotalSeconds: 3600, AIStats: AIStats{
				AIAdditions: 10, HumanAdditions: 5, AIInputTokens: 100, AIModelTotalCost: 1.5,
				AISessions: sessions, AIPromptEventsTotal: 2, AIPromptLengthSum: 300,
			}},
			Stats: []DayStats{{Day: d, Type: "category", Name: "AI Coding", TotalSeconds: aiCodingSeconds}},
		}
		for i, session := range heartbeatSessions {
			data.Heartbeats = append(data.Heartbeats, HeartBeat{Day: d, Entity: "main.go", Time: float64(i), AISession: session})
		}
		if _, err := db.SaveDay(data, false); err != nil {
			t.Fatal(err)
		}
	}
	// Synced before AI stats were, so its heartbeats have no sessions
	save(day(-2), 4, 0, "")
	// Session b spans midnight
	save(day(-1), 2, 100, "a", "b")
	save(day(0), 2, 50, "b", "c")

	totals, err := db.GetAITotals(day(-2), day(0))
	if err != nil {
		t.Fatal(err)
	}
	want := AITotals{
		AIStats: AIStats{
			AIAdditions: 30, HumanAdditions: 15, AIInputTokens: 300, AIModelTotalCost: 4.5,
			AISessions: 3 + 4, AIPromptEventsTotal: 6, AIPromptLengthSum: 900, AIPromptLengthAvg: 150,
		},
		AICodingSeconds: 150,
		ActiveDays:      3,
	}
	if *totals != want {
		t.Errorf("GetAITotals = %+v\nwant %+v", *totals, want)
	}
}
