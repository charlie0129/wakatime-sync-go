package wakatime

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func TestAIStatsModels(t *testing.T) {
	// Breakdown entries of durations may lack lines and cost, which the maps have
	var stats AIStats
	err := json.Unmarshal([]byte(`{
		"ai_model_breakdown": [
			{"name": "Opus", "input_tokens": 100, "output_tokens": 10, "cached_input_tokens": 1000},
			{"name": "Vscode", "lines": 5, "cost": 0, "input_tokens": 0, "output_tokens": 0, "cached_input_tokens": 0}
		],
		"ai_model_line_changes": {"Opus": 12, "Vscode": 5, "GPT": 3},
		"ai_model_costs": {"Opus": 1.5, "GLM": 0.25}
	}`), &stats)
	if err != nil {
		t.Fatal(err)
	}

	want := []AIModelUsage{
		{Name: "Opus", Lines: 12, InputTokens: 100, CachedInputTokens: 1000, OutputTokens: 10, Cost: 1.5},
		{Name: "Vscode", Lines: 5},
		{Name: "GLM", Cost: 0.25},
		{Name: "GPT", Lines: 3},
	}
	if got := stats.Models(); !reflect.DeepEqual(got, want) {
		t.Errorf("Models() = %+v, want %+v", got, want)
	}

	if got := stats.TotalCost(); got != 1.75 {
		t.Errorf("TotalCost() = %v, want the sum of model costs 1.75", got)
	}
	stats.AIModelTotalCost = 2
	if got := stats.TotalCost(); got != 2 {
		t.Errorf("TotalCost() = %v, want ai_model_total_cost 2", got)
	}
}

func TestLooseString(t *testing.T) {
	var p ProjectData
	if err := json.Unmarshal([]byte(`{"repository": {"full_name": "me/app"}, "badge": null, "color": null}`), &p); err != nil {
		t.Fatal(err)
	}
	if p.Repository != `{"full_name": "me/app"}` || p.Badge != "" {
		t.Errorf("repository = %q, badge = %q", p.Repository, p.Badge)
	}

	if err := json.Unmarshal([]byte(`{"repository": "https://github.com/me/app"}`), &p); err != nil {
		t.Fatal(err)
	}
	if p.Repository != "https://github.com/me/app" {
		t.Errorf("repository = %q", p.Repository)
	}
}

func TestAPIKeySentAsParameter(t *testing.T) {
	// Legacy API keys and secret OAuth tokens ("sec_...") both authenticate via
	// the api_key query parameter, and only that way.
	for _, key := range []string{
		"7621f438-81ad-4c1b-8692-89d616d02a93",
		"sec_0123456789abcdef",
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("api_key"); got != key {
				t.Errorf("api_key = %q, want %q", got, key)
			}
			if got := r.Header.Get("Authorization"); got != "" {
				t.Errorf("Authorization header = %q, want it unset", got)
			}
			w.Write([]byte(`{"data": {"id": "me"}}`))
		}))
		defer server.Close()

		if _, err := NewClientWithBaseURL(key, "", server.URL).GetUser(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRateLimitedRequestsAreRetried(t *testing.T) {
	rateLimitWait = time.Millisecond
	defer func() { rateLimitWait = 30 * time.Second }()

	tests := []struct {
		name        string
		rateLimited int32
		wantErr     bool
		wantCalls   int32
	}{
		{"recovers after waiting", 2, false, 3},
		{"gives up eventually", rateLimitRetries + 1, true, rateLimitRetries + 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) <= tt.rateLimited {
					w.WriteHeader(http.StatusTooManyRequests)
					return
				}
				w.Write([]byte(`{"data": {"id": "me"}}`))
			}))
			defer server.Close()

			user, err := NewClientWithBaseURL("key", "", server.URL).GetUser()
			if tt.wantErr {
				var apiErr *APIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTooManyRequests {
					t.Errorf("GetUser error = %v, want status 429", err)
				}
			} else if err != nil || user.Data.ID != "me" {
				t.Errorf("GetUser = %+v, %v", user, err)
			}
			if got := calls.Load(); got != tt.wantCalls {
				t.Errorf("requests = %d, want %d", got, tt.wantCalls)
			}
		})
	}
}
