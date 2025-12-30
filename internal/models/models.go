package models

import (
	"time"
)

// Duration represents a coding duration period
type Duration struct {
	ID           int64     `json:"id"`
	Day          time.Time `json:"day"`
	Project      string    `json:"project"`
	StartTime    float64   `json:"time"`     // UNIX timestamp
	Duration     float64   `json:"duration"` // seconds
	Dependencies string    `json:"dependencies,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// ProjectDuration represents detailed duration for a specific project
type ProjectDuration struct {
	ID           int64     `json:"id"`
	Day          time.Time `json:"day"`
	Project      string    `json:"project"`
	Branch       string    `json:"branch,omitempty"`
	Entity       string    `json:"entity,omitempty"`
	Language     string    `json:"language,omitempty"`
	Type         string    `json:"type,omitempty"`
	StartTime    float64   `json:"time"`
	Duration     float64   `json:"duration"`
	Dependencies string    `json:"dependencies,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// HeartBeat represents a heartbeat event
type HeartBeat struct {
	ID        int64     `json:"id"`
	Day       time.Time `json:"day"`
	Entity    string    `json:"entity"`
	Type      string    `json:"type"`
	Category  string    `json:"category,omitempty"`
	Time      float64   `json:"time"` // UNIX timestamp
	Project   string    `json:"project,omitempty"`
	Branch    string    `json:"branch,omitempty"`
	Language  string    `json:"language,omitempty"`
	IsWrite   bool      `json:"is_write"`
	MachineID string    `json:"machine_name_id,omitempty"`
	Lines     int       `json:"lines,omitempty"`
	LineNo    int       `json:"lineno,omitempty"`
	CursorPos int       `json:"cursorpos,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Project represents a WakaTime project
type Project struct {
	ID               int64     `json:"id"`
	UUID             string    `json:"uuid"`
	Name             string    `json:"name"`
	Repository       string    `json:"repository,omitempty"`
	Badge            string    `json:"badge,omitempty"`
	Color            string    `json:"color,omitempty"`
	HasPublicURL     bool      `json:"has_public_url"`
	LastHeartbeatAt  time.Time `json:"last_heartbeat_at,omitempty"`
	FirstHeartbeatAt time.Time `json:"first_heartbeat_at,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// DaySummary represents aggregated statistics for a day
type DaySummary struct {
	ID           int64     `json:"id"`
	Day          time.Time `json:"day"`
	TotalSeconds float64   `json:"total_seconds"`
	CreatedAt    time.Time `json:"created_at"`
}

// DayStats represents statistics breakdown for a specific category type
type DayStats struct {
	ID           int64     `json:"id"`
	Day          time.Time `json:"day"`
	Type         string    `json:"type"` // category, language, editor, os, project, dependency
	Name         string    `json:"name"`
	TotalSeconds float64   `json:"total_seconds"`
	CreatedAt    time.Time `json:"created_at"`
}
