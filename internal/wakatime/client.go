package wakatime

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"
)

const BaseURL = "https://wakatime.com/api/v1"

// minRequestInterval spaces out requests. WakaTime documents a rate limit of 10 requests per
// second averaged over 5 minutes, but it rate limits well below that in practice, possibly because
// the heartbeats sent by the user's plugins count towards it too.
const minRequestInterval = 500 * time.Millisecond

// rateLimitRetries is how many times a rate limited request is retried, after waiting for
// Retry-After or for rateLimitWait doubled at each attempt, up to maxRateLimitWait.
const rateLimitRetries = 6

var (
	rateLimitWait    = 30 * time.Second
	maxRateLimitWait = 5 * time.Minute
)

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client

	mu          sync.Mutex
	lastRequest time.Time
}

// APIError is returned when the WakaTime API responds with a non-200 status code.
type APIError struct {
	StatusCode int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("wakatime api returned status %d", e.StatusCode)
}

// Retryable reports whether the request may succeed later. Rate limiting and server errors are
// transient, while other client errors (e.g. 402 for premium-only data) keep failing.
func (e *APIError) Retryable() bool {
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}

func NewClient(apiKey string, proxyURL string) *Client {
	return NewClientWithBaseURL(apiKey, proxyURL, BaseURL)
}

func NewClientWithBaseURL(apiKey string, proxyURL string, baseURL string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()

	if proxyURL != "" && proxyURL != "false" {
		if proxyParsed, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxyParsed)
		}
	}

	if baseURL == "" {
		baseURL = BaseURL
	}

	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

func (c *Client) doRequest(endpoint string, params map[string]string) ([]byte, error) {
	reqURL, err := url.Parse(c.baseURL + endpoint)
	if err != nil {
		return nil, err
	}

	q := reqURL.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	reqURL.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Basic "+c.apiKey)
	req.Header.Set("Accept", "application/json")

	for attempt := 0; ; attempt++ {
		body, status, retryAfter, err := c.send(req)
		if err != nil {
			return nil, err
		}

		// Wait out rate limiting, so that a long sync slows down instead of failing
		if status == http.StatusTooManyRequests && attempt < rateLimitRetries {
			wait := retryAfter
			if wait <= 0 {
				wait = min(rateLimitWait<<attempt, maxRateLimitWait)
			}
			slog.Warn("rate limited by wakatime, waiting before retrying", "endpoint", endpoint, "attempt", attempt, "wait", wait)
			time.Sleep(wait)
			continue
		}

		if status != http.StatusOK {
			slog.Error("wakatime api error", "status", status, "body", string(body))
			return nil, &APIError{StatusCode: status}
		}
		return body, nil
	}
}

// send sends the request once, returning the response body and status code, and how long to
// wait before retrying according to the Retry-After header, if any.
func (c *Client) send(req *http.Request) ([]byte, int, time.Duration, error) {
	c.throttle()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, 0, err
	}

	var retryAfter time.Duration
	if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && seconds > 0 {
		retryAfter = time.Duration(seconds) * time.Second
	}
	return body, resp.StatusCode, retryAfter, nil
}

// throttle waits until at least minRequestInterval has passed since the previous request.
func (c *Client) throttle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if wait := time.Until(c.lastRequest.Add(minRequestInterval)); wait > 0 {
		time.Sleep(wait)
	}
	c.lastRequest = time.Now()
}

type pagedResponse[T any] struct {
	Data       []T  `json:"data"`
	TotalPages int  `json:"total_pages"`
	NextPage   *int `json:"next_page"`
}

// getAllPages fetches every page of a paginated endpoint.
func getAllPages[T any](c *Client, endpoint string, params map[string]string) ([]T, error) {
	var all []T
	for page := 1; ; page++ {
		pageParams := map[string]string{"page": strconv.Itoa(page)}
		for k, v := range params {
			pageParams[k] = v
		}
		body, err := c.doRequest(endpoint, pageParams)
		if err != nil {
			return nil, err
		}

		var resp pagedResponse[T]
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Data...)

		if resp.NextPage == nil || page >= resp.TotalPages || len(resp.Data) == 0 {
			return all, nil
		}
	}
}

// LooseString decodes a JSON string as is, and any other non-null value as its raw JSON text.
// WakaTime documents some fields as strings that may be objects, e.g. the repository of a
// connected project; decoding them strictly would fail the whole response.
type LooseString string

func (s *LooseString) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*s = ""
		return nil
	}
	var str string
	if err := json.Unmarshal(b, &str); err == nil {
		*s = LooseString(str)
		return nil
	}
	*s = LooseString(b)
	return nil
}

// --- API Response Types ---

type DurationResponse struct {
	Data     []DurationData `json:"data"`
	Start    string         `json:"start"`
	End      string         `json:"end"`
	Timezone string         `json:"timezone"`
}

// AIStats holds the AI coding metrics WakaTime adds to durations, summary items and grand totals.
type AIStats struct {
	AIAdditions                    int     `json:"ai_additions,omitempty"`
	AIDeletions                    int     `json:"ai_deletions,omitempty"`
	HumanAdditions                 int     `json:"human_additions,omitempty"`
	HumanDeletions                 int     `json:"human_deletions,omitempty"`
	AIInputTokens                  int64   `json:"ai_input_tokens,omitempty"`
	AICachedInputTokens            int64   `json:"ai_cached_input_tokens,omitempty"`
	AIOutputTokens                 int64   `json:"ai_output_tokens,omitempty"`
	AIModelTotalCost               float64 `json:"ai_model_total_cost,omitempty"`
	AISessions                     int     `json:"ai_sessions,omitempty"`
	AIPromptEventsTotal            int     `json:"ai_prompt_events_total,omitempty"`
	AIPromptLengthSum              int64   `json:"ai_prompt_length_sum,omitempty"`
	AIPromptLengthAvg              float64 `json:"ai_prompt_length_avg,omitempty"`
	AIPromptLengthAvgPerSession    float64 `json:"ai_prompt_length_avg_per_session,omitempty"`
	AIPromptLengthMedianPerSession float64 `json:"ai_prompt_length_median_per_session,omitempty"`
	AIPromptEventsAvgPerSession    float64 `json:"ai_prompt_events_avg_per_session,omitempty"`
	AIPromptEventsMedianPerSession float64 `json:"ai_prompt_events_median_per_session,omitempty"`

	AIModelBreakdown   []AIModelUsage     `json:"ai_model_breakdown,omitempty"`
	AIModelLineChanges map[string]int     `json:"ai_model_line_changes,omitempty"`
	AIModelCosts       map[string]float64 `json:"ai_model_costs,omitempty"`
}

// AIModelUsage is the usage of an AI model, as in ai_model_breakdown.
type AIModelUsage struct {
	Name              string  `json:"name"`
	Lines             int     `json:"lines"`
	InputTokens       int64   `json:"input_tokens"`
	CachedInputTokens int64   `json:"cached_input_tokens"`
	OutputTokens      int64   `json:"output_tokens"`
	Cost              float64 `json:"cost"`
}

// Models returns the usage of each AI model. It merges ai_model_breakdown with ai_model_line_changes
// and ai_model_costs, since breakdown entries of durations often lack the lines and cost that the
// maps have.
func (a *AIStats) Models() []AIModelUsage {
	models := make([]AIModelUsage, 0, len(a.AIModelBreakdown))
	index := make(map[string]int)
	for _, m := range a.AIModelBreakdown {
		index[m.Name] = len(models)
		models = append(models, m)
	}

	// Models missing from the breakdown, in a stable order
	missing := make(map[string]bool)
	for name := range a.AIModelLineChanges {
		missing[name] = true
	}
	for name := range a.AIModelCosts {
		missing[name] = true
	}
	names := make([]string, 0, len(missing))
	for name := range missing {
		if _, ok := index[name]; !ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		index[name] = len(models)
		models = append(models, AIModelUsage{Name: name})
	}

	for name, lines := range a.AIModelLineChanges {
		models[index[name]].Lines = lines
	}
	for name, cost := range a.AIModelCosts {
		models[index[name]].Cost = cost
	}
	return models
}

// TotalCost returns the estimated cost of all AI models. Durations lack ai_model_total_cost, so it
// falls back to the sum of the models' costs.
func (a *AIStats) TotalCost() float64 {
	if a.AIModelTotalCost != 0 {
		return a.AIModelTotalCost
	}
	var total float64
	for _, m := range a.Models() {
		total += m.Cost
	}
	return total
}

type DurationData struct {
	Project      string   `json:"project"`
	Time         float64  `json:"time"`
	Duration     float64  `json:"duration"`
	Branch       string   `json:"branch,omitempty"`
	Entity       string   `json:"entity,omitempty"`
	Language     string   `json:"language,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
	Type         string   `json:"type,omitempty"`
	AIStats
}

type HeartbeatResponse struct {
	Data     []HeartbeatData `json:"data"`
	Start    string          `json:"start"`
	End      string          `json:"end"`
	Timezone string          `json:"timezone"`
}

type HeartbeatData struct {
	ID                  string   `json:"id"`
	Entity              string   `json:"entity"`
	Type                string   `json:"type"`
	Category            string   `json:"category"`
	Time                float64  `json:"time"`
	Project             string   `json:"project,omitempty"`
	ProjectRootCount    int      `json:"project_root_count,omitempty"`
	Branch              string   `json:"branch,omitempty"`
	Language            string   `json:"language,omitempty"`
	Dependencies        []string `json:"dependencies,omitempty"`
	MachineNameID       string   `json:"machine_name_id,omitempty"`
	UserAgentID         string   `json:"user_agent_id,omitempty"`
	AILineChanges       int      `json:"ai_line_changes,omitempty"`
	HumanLineChanges    int      `json:"human_line_changes,omitempty"`
	AISession           string   `json:"ai_session,omitempty"`
	AIInputTokens       int64    `json:"ai_input_tokens,omitempty"`
	AICachedInputTokens int64    `json:"ai_cached_input_tokens,omitempty"`
	AIOutputTokens      int64    `json:"ai_output_tokens,omitempty"`
	AIPromptLength      int      `json:"ai_prompt_length,omitempty"`
	AISubscriptionPlan  string   `json:"ai_subscription_plan,omitempty"`
	Lines               int      `json:"lines,omitempty"`
	LineNo              int      `json:"lineno,omitempty"`
	CursorPos           int      `json:"cursorpos,omitempty"`
	IsWrite             bool     `json:"is_write"`
}

type ProjectData struct {
	ID                            string      `json:"id"`
	Name                          string      `json:"name"`
	Repository                    LooseString `json:"repository,omitempty"`
	Badge                         LooseString `json:"badge,omitempty"`
	Color                         string      `json:"color,omitempty"`
	HasPublicURL                  bool        `json:"has_public_url"`
	HumanReadableLastHeartbeatAt  string      `json:"human_readable_last_heartbeat_at,omitempty"`
	LastHeartbeatAt               string      `json:"last_heartbeat_at,omitempty"`
	HumanReadableFirstHeartbeatAt string      `json:"human_readable_first_heartbeat_at,omitempty"`
	FirstHeartbeatAt              string      `json:"first_heartbeat_at,omitempty"`
	URL                           string      `json:"url,omitempty"`
	URLEncodedName                string      `json:"urlencoded_name,omitempty"`
	CreatedAt                     string      `json:"created_at,omitempty"`
}

type SummaryResponse struct {
	Data            []SummaryDay    `json:"data"`
	CumulativeTotal CumulativeTotal `json:"cumulative_total"`
	DailyAverage    DailyAverage    `json:"daily_average"`
	Start           string          `json:"start"`
	End             string          `json:"end"`
}

type SummaryDay struct {
	GrandTotal       GrandTotal    `json:"grand_total"`
	Categories       []SummaryItem `json:"categories"`
	Projects         []SummaryItem `json:"projects"`
	Languages        []SummaryItem `json:"languages"`
	Editors          []SummaryItem `json:"editors"`
	OperatingSystems []SummaryItem `json:"operating_systems"`
	Dependencies     []SummaryItem `json:"dependencies"`
	Machines         []MachineItem `json:"machines"`
	Branches         []SummaryItem `json:"branches,omitempty"`
	Entities         []SummaryItem `json:"entities,omitempty"`
	Range            SummaryRange  `json:"range"`
}

type GrandTotal struct {
	Digital      string  `json:"digital"`
	Hours        int     `json:"hours"`
	Minutes      int     `json:"minutes"`
	Text         string  `json:"text"`
	TotalSeconds float64 `json:"total_seconds"`
	AIStats
}

type SummaryItem struct {
	Name         string  `json:"name"`
	TotalSeconds float64 `json:"total_seconds"`
	Percent      float64 `json:"percent"`
	Digital      string  `json:"digital"`
	Text         string  `json:"text"`
	Hours        int     `json:"hours"`
	Minutes      int     `json:"minutes"`
	Seconds      int     `json:"seconds,omitempty"`
	AIStats
}

type MachineItem struct {
	Name          string  `json:"name"`
	MachineNameID string  `json:"machine_name_id"`
	TotalSeconds  float64 `json:"total_seconds"`
	Percent       float64 `json:"percent"`
	Digital       string  `json:"digital"`
	Text          string  `json:"text"`
	Hours         int     `json:"hours"`
	Minutes       int     `json:"minutes"`
	Seconds       int     `json:"seconds,omitempty"`
}

type SummaryRange struct {
	Date     string `json:"date"`
	Start    string `json:"start"`
	End      string `json:"end"`
	Text     string `json:"text"`
	Timezone string `json:"timezone"`
}

type CumulativeTotal struct {
	Seconds float64 `json:"seconds"`
	Text    string  `json:"text"`
	Decimal string  `json:"decimal"`
	Digital string  `json:"digital"`
}

type DailyAverage struct {
	Holidays                      int     `json:"holidays"`
	DaysIncludingHolidays         int     `json:"days_including_holidays"`
	DaysMinusHolidays             int     `json:"days_minus_holidays"`
	Seconds                       float64 `json:"seconds"`
	Text                          string  `json:"text"`
	SecondsIncludingOtherLanguage float64 `json:"seconds_including_other_language"`
	TextIncludingOtherLanguage    string  `json:"text_including_other_language"`
}

// UserAgentData is a plugin which has sent data for the user, and the AI model it reported.
type UserAgentData struct {
	ID                 string `json:"id"`
	Value              string `json:"value"`
	Editor             string `json:"editor,omitempty"`
	OS                 string `json:"os,omitempty"`
	Version            string `json:"version,omitempty"`
	AIModel            string `json:"ai_model,omitempty"`
	AIModelVersion     string `json:"ai_model_version,omitempty"`
	AIModelComplexity  string `json:"ai_model_complexity,omitempty"`
	IsBrowserExtension bool   `json:"is_browser_extension"`
	IsDesktopApp       bool   `json:"is_desktop_app"`
	CreatedAt          string `json:"created_at,omitempty"`
	LastSeenAt         string `json:"last_seen_at,omitempty"`
}

type UserResponse struct {
	Data UserData `json:"data"`
}

type UserData struct {
	ID                 string `json:"id"`
	DisplayName        string `json:"display_name"`
	FullName           string `json:"full_name"`
	Email              string `json:"email"`
	Photo              string `json:"photo"`
	Timezone           string `json:"timezone"`
	LastHeartbeatAt    string `json:"last_heartbeat_at"`
	LastPlugin         string `json:"last_plugin"`
	LastPluginName     string `json:"last_plugin_name"`
	LastProject        string `json:"last_project"`
	Username           string `json:"username"`
	CreatedAt          string `json:"created_at"`
	HasPremiumFeatures bool   `json:"has_premium_features"`
}

// --- API Methods ---

func (c *Client) GetDurations(date time.Time) (*DurationResponse, error) {
	params := map[string]string{
		"date": date.Format("2006-01-02"),
	}
	body, err := c.doRequest("/users/current/durations", params)
	if err != nil {
		return nil, err
	}

	var resp DurationResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetDurationsWithProject(date time.Time, project string) (*DurationResponse, error) {
	params := map[string]string{
		"date":     date.Format("2006-01-02"),
		"project":  project,
		"slice_by": "entity",
	}
	body, err := c.doRequest("/users/current/durations", params)
	if err != nil {
		return nil, err
	}

	var resp DurationResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetHeartbeats(date time.Time) (*HeartbeatResponse, error) {
	params := map[string]string{
		"date": date.Format("2006-01-02"),
	}
	body, err := c.doRequest("/users/current/heartbeats", params)
	if err != nil {
		return nil, err
	}

	var resp HeartbeatResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetProjects returns all of the user's projects, following pagination.
func (c *Client) GetProjects(query string) ([]ProjectData, error) {
	params := map[string]string{}
	if query != "" {
		params["q"] = query
	}
	return getAllPages[ProjectData](c, "/users/current/projects", params)
}

func (c *Client) GetSummaries(start, end time.Time) (*SummaryResponse, error) {
	params := map[string]string{
		"start": start.Format("2006-01-02"),
		"end":   end.Format("2006-01-02"),
	}
	body, err := c.doRequest("/users/current/summaries", params)
	if err != nil {
		return nil, err
	}

	var resp SummaryResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetUserAgents returns all plugins which have sent data for the user, following pagination.
func (c *Client) GetUserAgents() ([]UserAgentData, error) {
	return getAllPages[UserAgentData](c, "/users/current/user_agents", nil)
}

func (c *Client) GetUser() (*UserResponse, error) {
	body, err := c.doRequest("/users/current", nil)
	if err != nil {
		return nil, err
	}

	var resp UserResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
