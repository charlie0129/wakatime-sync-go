package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/charlie0129/wakatime-sync-go/internal/database"
)

const dayLayout = "2006-01-02"

// rangePresets are the named date ranges of the dashboard, given today's date. Ranges end
// yesterday, the last day that can have been synced, except when that would end them before they
// start. Weeks start on Monday.
var rangePresets = map[string]func(today time.Time) (time.Time, time.Time){
	"yesterday": lastDays(1),
	"7d":        lastDays(7),
	"30d":       lastDays(30),
	"90d":       lastDays(90),
	"12m":       lastDays(365),
	"this-week": func(today time.Time) (time.Time, time.Time) {
		return untilYesterday(startOfWeek(today), today)
	},
	"last-week": func(today time.Time) (time.Time, time.Time) {
		start := startOfWeek(today).AddDate(0, 0, -7)
		return start, start.AddDate(0, 0, 6)
	},
	"this-month": func(today time.Time) (time.Time, time.Time) {
		return untilYesterday(time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC), today)
	},
	"last-month": func(today time.Time) (time.Time, time.Time) {
		thisMonth := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
		return thisMonth.AddDate(0, -1, 0), thisMonth.AddDate(0, 0, -1)
	},
	"this-year": func(today time.Time) (time.Time, time.Time) {
		return untilYesterday(time.Date(today.Year(), 1, 1, 0, 0, 0, 0, time.UTC), today)
	},
	"last-year": func(today time.Time) (time.Time, time.Time) {
		return time.Date(today.Year()-1, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(today.Year()-1, 12, 31, 0, 0, 0, 0, time.UTC)
	},
}

// lastDays returns a preset of the n days until yesterday.
func lastDays(n int) func(time.Time) (time.Time, time.Time) {
	return func(today time.Time) (time.Time, time.Time) {
		end := today.AddDate(0, 0, -1)
		return end.AddDate(0, 0, 1-n), end
	}
}

// untilYesterday returns the range from start to yesterday, or to today if yesterday is before start.
func untilYesterday(start, today time.Time) (time.Time, time.Time) {
	if end := today.AddDate(0, 0, -1); !end.Before(start) {
		return start, end
	}
	return start, today
}

func startOfWeek(day time.Time) time.Time {
	return day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
}

// today returns the current date in the configured timezone, as a UTC midnight like parsed dates.
func (h *Handler) today() time.Time {
	y, m, d := h.now().In(h.cfg.GetTimezone()).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// resolveRange returns the date range of a request, given as a preset with range=, or as start=
// and end= dates. Dates are UTC midnights.
func (h *Handler) resolveRange(r *http.Request) (preset string, start, end time.Time, err error) {
	q := r.URL.Query()
	if preset = q.Get("range"); preset != "" {
		resolve, ok := rangePresets[preset]
		if !ok {
			return "", start, end, fmt.Errorf("unknown range %q", preset)
		}
		start, end = resolve(h.today())
		return preset, start, end, nil
	}

	if start, err = parseDate(q.Get("start")); err != nil {
		return "", start, end, errors.New("invalid start date, use YYYY-MM-DD")
	}
	if end, err = parseDate(q.Get("end")); err != nil {
		return "", start, end, errors.New("invalid end date, use YYYY-MM-DD")
	}
	if start.After(end) {
		return "", start, end, errors.New("the start date is after the end date")
	}
	return "", start, end, nil
}

// --- Dashboard ---

// breakdownItem is the time spent in a project, editor, language, etc. over a date range, with the
// AI coding metrics of projects and editors.
type breakdownItem struct {
	Name         string  `json:"name"`
	TotalSeconds float64 `json:"total_seconds"`
	AILines      int64   `json:"ai_lines"`
	HumanLines   int64   `json:"human_lines"`
	AITokens     int64   `json:"ai_tokens"`
	AICost       float64 `json:"ai_cost"`
}

func breakdown(stats []database.AggregatedStat) []breakdownItem {
	items := make([]breakdownItem, len(stats))
	for i, s := range stats {
		items[i] = breakdownItem{
			Name:         s.Name,
			TotalSeconds: s.TotalSeconds,
			AILines:      s.AIAdditions + s.AIDeletions,
			HumanLines:   s.HumanAdditions + s.HumanDeletions,
			AITokens:     s.Tokens(),
			AICost:       s.AIModelTotalCost,
		}
	}
	return items
}

// aiSummary is the AI coding activity over a date range.
type aiSummary struct {
	CodingSeconds     float64 `json:"coding_seconds"`
	ActiveDays        int64   `json:"active_days"`
	Lines             int64   `json:"lines"`
	HumanLines        int64   `json:"human_lines"`
	InputTokens       int64   `json:"input_tokens"`
	CachedInputTokens int64   `json:"cached_input_tokens"`
	OutputTokens      int64   `json:"output_tokens"`
	Cost              float64 `json:"cost"`
	Sessions          int64   `json:"sessions"`
	Prompts           int64   `json:"prompts"`
	PromptLengthAvg   float64 `json:"prompt_length_avg"`
}

func summarizeAI(t *database.AITotals) aiSummary {
	return aiSummary{
		CodingSeconds:     t.AICodingSeconds,
		ActiveDays:        t.ActiveDays,
		Lines:             t.AIAdditions + t.AIDeletions,
		HumanLines:        t.HumanAdditions + t.HumanDeletions,
		InputTokens:       t.AIInputTokens,
		CachedInputTokens: t.AICachedInputTokens,
		OutputTokens:      t.AIOutputTokens,
		Cost:              t.AIModelTotalCost,
		Sessions:          t.AISessions,
		Prompts:           t.AIPromptEventsTotal,
		PromptLengthAvg:   t.AIPromptLengthAvg,
	}
}

// periodTotals is the activity over the period before the requested date range, to compare with.
type periodTotals struct {
	Start        string    `json:"start"`
	End          string    `json:"end"`
	TotalSeconds float64   `json:"total_seconds"`
	ActiveDays   int64     `json:"active_days"`
	AI           aiSummary `json:"ai"`
}

// getDashboard returns everything the web dashboard shows for a date range, in one response. The
// previous period has as many days as the range, and ends the day before it starts.
// GET /api/v1/dashboard?range=7d
// GET /api/v1/dashboard?start=2024-01-01&end=2024-01-31
func (h *Handler) getDashboard(w http.ResponseWriter, r *http.Request) {
	preset, start, end, err := h.resolveRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	prevEnd := start.AddDate(0, 0, -1)
	prevStart := prevEnd.Add(-end.Sub(start))

	resp, err := h.dashboard(start, end, prevStart, prevEnd)
	if err != nil {
		slog.Error("failed to get dashboard", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get dashboard")
		return
	}
	resp["range"] = preset
	resp["today"] = h.today().Format(dayLayout)
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) dashboard(start, end, prevStart, prevEnd time.Time) (map[string]interface{}, error) {
	days, err := h.db.GetDailyActivity(start, end)
	if err != nil {
		return nil, err
	}
	totalSeconds, activeDays, err := h.db.GetRangeTotals(start, end)
	if err != nil {
		return nil, err
	}
	aiTotals, err := h.db.GetAITotals(start, end)
	if err != nil {
		return nil, err
	}

	prevSeconds, prevActiveDays, err := h.db.GetRangeTotals(prevStart, prevEnd)
	if err != nil {
		return nil, err
	}
	prevAI, err := h.db.GetAITotals(prevStart, prevEnd)
	if err != nil {
		return nil, err
	}

	resp := map[string]interface{}{
		"start":         start.Format(dayLayout),
		"end":           end.Format(dayLayout),
		"total_seconds": totalSeconds,
		"active_days":   activeDays,
		"days":          days,
		"ai":            summarizeAI(aiTotals),
		"previous": periodTotals{
			Start:        prevStart.Format(dayLayout),
			End:          prevEnd.Format(dayLayout),
			TotalSeconds: prevSeconds,
			ActiveDays:   prevActiveDays,
			AI:           summarizeAI(prevAI),
		},
	}

	for key, statType := range map[string]string{
		"categories":        "category",
		"languages":         "language",
		"editors":           "editor",
		"operating_systems": "os",
		"projects":          "project",
		"machines":          "machine",
	} {
		stats, err := h.db.GetAggregatedStats(start, end, statType)
		if err != nil {
			return nil, err
		}
		resp[key] = breakdown(stats)
	}

	models, err := h.db.GetAIModelUsage(start, end)
	if err != nil {
		return nil, err
	}
	versions, err := h.db.GetAIModelVersions(start, end)
	if err != nil {
		return nil, err
	}
	names, err := h.db.GetAIModelNames()
	if err != nil {
		return nil, err
	}
	resp["ai_models"] = nonNil(models)
	resp["ai_model_versions"] = nonNil(versions)
	resp["ai_model_names"] = nonNil(names)

	return resp, nil
}

// --- Activity ---

// spanMergeGap is the longest pause between two spans of the same kind that are drawn as one.
const spanMergeGap = 60

// daySpans is when coding happened on a day: spans of [start, end, ai] seconds since midnight,
// where ai is 1 when AI coding tools were used during the span.
type daySpans struct {
	Date  string   `json:"date"`
	Spans [][3]int `json:"spans"`
}

// groupSpans groups spans by day, from start to end in the given timezone, merging spans of the
// same kind that are close to each other.
func groupSpans(spans []database.DurationSpan, start, end time.Time, loc *time.Location) []daySpans {
	var days []daySpans
	index := make(map[string]int)
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		date := d.Format(dayLayout)
		index[date] = len(days)
		days = append(days, daySpans{Date: date, Spans: [][3]int{}})
	}

	for _, span := range spans {
		i, ok := index[span.Day.Format(dayLayout)]
		if !ok {
			continue
		}
		y, m, d := span.Day.Date()
		midnight := float64(time.Date(y, m, d, 0, 0, 0, 0, loc).Unix())
		from := int(max(span.StartTime-midnight, 0))
		to := int(min(span.StartTime+span.Duration-midnight, 24*60*60))
		if to <= from {
			continue
		}
		ai := 0
		if span.AI {
			ai = 1
		}

		day := &days[i]
		if n := len(day.Spans); n > 0 {
			last := &day.Spans[n-1]
			if last[2] == ai && from <= last[1]+spanMergeGap {
				last[1] = max(last[1], to)
				continue
			}
		}
		day.Spans = append(day.Spans, [3]int{from, to, ai})
	}
	return days
}

// getActivity returns when coding happened on each day of a date range, for the rhythm plot of the
// dashboard.
// GET /api/v1/activity?range=7d
// GET /api/v1/activity?start=2024-01-01&end=2024-01-31
func (h *Handler) getActivity(w http.ResponseWriter, r *http.Request) {
	preset, start, end, err := h.resolveRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	spans, err := h.db.GetDurationSpans(start, end)
	if err != nil {
		slog.Error("failed to get activity", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get activity")
		return
	}

	loc := h.cfg.GetTimezone()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"range":    preset,
		"start":    start.Format(dayLayout),
		"end":      end.Format(dayLayout),
		"timezone": loc.String(),
		"days":     groupSpans(spans, start, end, loc),
	})
}
