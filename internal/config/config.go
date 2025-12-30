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
	// Start with default config
	cfg := defaultConfig()

	// Load from YAML file if it exists
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	if err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	// Override with environment variables if set
	if envListenAddr := os.Getenv("LISTEN_ADDR"); envListenAddr != "" {
		cfg.ListenAddr = envListenAddr
	}
	if envDatabasePath := os.Getenv("DATABASE_PATH"); envDatabasePath != "" {
		cfg.DatabasePath = envDatabasePath
	}
	if envWakaTimeAPI := os.Getenv("WAKATIME_API_KEY"); envWakaTimeAPI != "" {
		cfg.WakaTimeAPI = envWakaTimeAPI
	}
	if envProxyURL := os.Getenv("PROXY_URL"); envProxyURL != "" {
		cfg.ProxyURL = envProxyURL
	}
	if envStartDate := os.Getenv("START_DATE"); envStartDate != "" {
		cfg.StartDate = envStartDate
	}
	if envSyncSchedule := os.Getenv("SYNC_SCHEDULE"); envSyncSchedule != "" {
		cfg.SyncSchedule = envSyncSchedule
	}
	if envTimezone := os.Getenv("TIMEZONE"); envTimezone != "" {
		cfg.Timezone = envTimezone
	}

	// Apply defaults for any still-missing values
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

	return cfg, nil
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
