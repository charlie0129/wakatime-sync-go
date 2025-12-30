package wakatime

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

const BaseURL = "https://wakatime.com/api/v1"

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		slog.Error("wakatime api error", "status", resp.StatusCode, "body", string(body))
		return nil, fmt.Errorf("wakatime api returned status %d", resp.StatusCode)
	}

	return body, nil
}

// --- API Response Types ---

type DurationResponse struct {
	Data     []DurationData `json:"data"`
	Start    string         `json:"start"`
	End      string         `json:"end"`
	Timezone string         `json:"timezone"`
}

type DurationData struct {
	Project        string   `json:"project"`
	Time           float64  `json:"time"`
	Duration       float64  `json:"duration"`
	Branch         string   `json:"branch,omitempty"`
	Entity         string   `json:"entity,omitempty"`
	Language       string   `json:"language,omitempty"`
	Dependencies   []string `json:"dependencies,omitempty"`
	Type           string   `json:"type,omitempty"`
	AIAdditions    int      `json:"ai_additions,omitempty"`
	AIDeletions    int      `json:"ai_deletions,omitempty"`
	HumanAdditions int      `json:"human_additions,omitempty"`
	HumanDeletions int      `json:"human_deletions,omitempty"`
}

type HeartbeatResponse struct {
	Data     []HeartbeatData `json:"data"`
	Start    string          `json:"start"`
	End      string          `json:"end"`
	Timezone string          `json:"timezone"`
}

type HeartbeatData struct {
	Entity           string   `json:"entity"`
	Type             string   `json:"type"`
	Category         string   `json:"category"`
	Time             float64  `json:"time"`
	Project          string   `json:"project,omitempty"`
	ProjectRootCount int      `json:"project_root_count,omitempty"`
	Branch           string   `json:"branch,omitempty"`
	Language         string   `json:"language,omitempty"`
	Dependencies     []string `json:"dependencies,omitempty"`
	MachineNameID    string   `json:"machine_name_id,omitempty"`
	Lines            int      `json:"lines,omitempty"`
	LineNo           int      `json:"lineno,omitempty"`
	CursorPos        int      `json:"cursorpos,omitempty"`
	IsWrite          bool     `json:"is_write"`
}

type ProjectResponse struct {
	Data []ProjectData `json:"data"`
}

type ProjectData struct {
	ID                            string `json:"id"`
	Name                          string `json:"name"`
	Repository                    string `json:"repository,omitempty"`
	Badge                         string `json:"badge,omitempty"`
	Color                         string `json:"color,omitempty"`
	HasPublicURL                  bool   `json:"has_public_url"`
	HumanReadableLastHeartbeatAt  string `json:"human_readable_last_heartbeat_at,omitempty"`
	LastHeartbeatAt               string `json:"last_heartbeat_at,omitempty"`
	HumanReadableFirstHeartbeatAt string `json:"human_readable_first_heartbeat_at,omitempty"`
	FirstHeartbeatAt              string `json:"first_heartbeat_at,omitempty"`
	URL                           string `json:"url,omitempty"`
	URLEncodedName                string `json:"urlencoded_name,omitempty"`
	CreatedAt                     string `json:"created_at,omitempty"`
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
	Digital        string  `json:"digital"`
	Hours          int     `json:"hours"`
	Minutes        int     `json:"minutes"`
	Text           string  `json:"text"`
	TotalSeconds   float64 `json:"total_seconds"`
	AIAdditions    int     `json:"ai_additions,omitempty"`
	AIDeletions    int     `json:"ai_deletions,omitempty"`
	HumanAdditions int     `json:"human_additions,omitempty"`
	HumanDeletions int     `json:"human_deletions,omitempty"`
}

type SummaryItem struct {
	Name           string  `json:"name"`
	TotalSeconds   float64 `json:"total_seconds"`
	Percent        float64 `json:"percent"`
	Digital        string  `json:"digital"`
	Text           string  `json:"text"`
	Hours          int     `json:"hours"`
	Minutes        int     `json:"minutes"`
	Seconds        int     `json:"seconds,omitempty"`
	AIAdditions    int     `json:"ai_additions,omitempty"`
	AIDeletions    int     `json:"ai_deletions,omitempty"`
	HumanAdditions int     `json:"human_additions,omitempty"`
	HumanDeletions int     `json:"human_deletions,omitempty"`
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

func (c *Client) GetProjects(query string) (*ProjectResponse, error) {
	params := map[string]string{}
	if query != "" {
		params["q"] = query
	}
	body, err := c.doRequest("/users/current/projects", params)
	if err != nil {
		return nil, err
	}

	var resp ProjectResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
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
