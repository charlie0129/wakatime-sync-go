package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ListenAddr   string `yaml:"listen_addr"`
	DatabasePath string `yaml:"database_path"`
	WakaTimeAPI  string `yaml:"wakatime_api_key"`
	ProxyURL     string `yaml:"proxy_url"`
	StartDate    string `yaml:"start_date"`
	SyncSchedule string `yaml:"sync_schedule"` // cron expression for daily sync
	Timezone     string `yaml:"timezone"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// Return default config if file doesn't exist
		if os.IsNotExist(err) {
			return defaultConfig(), nil
		}
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Apply defaults for missing values
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":3040"
	}
	if cfg.DatabasePath == "" {
		cfg.DatabasePath = "wakatime.db"
	}
	if cfg.StartDate == "" {
		cfg.StartDate = "2016-01-01"
	}
	if cfg.SyncSchedule == "" {
		cfg.SyncSchedule = "0 1 * * *" // 1 AM daily
	}
	if cfg.Timezone == "" {
		cfg.Timezone = "Local"
	}

	return &cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		ListenAddr:   ":3040",
		DatabasePath: "wakatime.db",
		StartDate:    "2016-01-01",
		SyncSchedule: "0 1 * * *",
		Timezone:     "Local",
	}
}

func (c *Config) GetStartDate() time.Time {
	t, err := time.Parse("2006-01-02", c.StartDate)
	if err != nil {
		return time.Date(2016, 1, 1, 0, 0, 0, 0, time.Local)
	}
	return t
}

func (c *Config) GetTimezone() *time.Location {
	if c.Timezone == "" || c.Timezone == "Local" {
		return time.Local
	}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return time.Local
	}
	return loc
}
