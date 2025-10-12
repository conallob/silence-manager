package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config represents the application configuration
type Config struct {
	Alertmanager AlertmanagerConfig
	Jira         JiraConfig
	Sync         SyncConfig
}

// AlertmanagerConfig holds Alertmanager-specific configuration
type AlertmanagerConfig struct {
	URL         string
	AuthType    string // "none", "basic", "bearer"
	Username    string // For basic auth
	Password    string // For basic auth
	BearerToken string // For bearer token auth
}

// JiraConfig holds Jira-specific configuration
type JiraConfig struct {
	URL        string
	Username   string
	APIToken   string
	ProjectKey string
}

// SyncConfig holds synchronization configuration
type SyncConfig struct {
	ExpiryThresholdHours        int
	ExtensionDurationHours      int
	DefaultSilenceDurationHours int
	CheckAlerts                 bool
	AnnotationPrefix            string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Alertmanager: AlertmanagerConfig{
			URL:         getEnv("ALERTMANAGER_URL", "http://alertmanager:9093"),
			AuthType:    getEnv("ALERTMANAGER_AUTH_TYPE", "none"),
			Username:    getEnv("ALERTMANAGER_USERNAME", ""),
			Password:    getEnv("ALERTMANAGER_PASSWORD", ""),
			BearerToken: getEnv("ALERTMANAGER_BEARER_TOKEN", ""),
		},
		Jira: JiraConfig{
			URL:        getEnv("JIRA_URL", ""),
			Username:   getEnv("JIRA_USERNAME", ""),
			APIToken:   getEnv("JIRA_API_TOKEN", ""),
			ProjectKey: getEnv("JIRA_PROJECT_KEY", ""),
		},
		Sync: SyncConfig{
			ExpiryThresholdHours:        getEnvInt("SYNC_EXPIRY_THRESHOLD_HOURS", 24),
			ExtensionDurationHours:      getEnvInt("SYNC_EXTENSION_DURATION_HOURS", 168), // 7 days
			DefaultSilenceDurationHours: getEnvInt("SYNC_DEFAULT_SILENCE_DURATION_HOURS", 168), // 7 days
			CheckAlerts:                 getEnvBool("SYNC_CHECK_ALERTS", true),
			AnnotationPrefix:            getEnv("SYNC_ANNOTATION_PREFIX", "silence-manager"),
		},
	}

	// Validate required fields
	if cfg.Jira.URL == "" {
		return nil, fmt.Errorf("JIRA_URL is required")
	}
	if cfg.Jira.Username == "" {
		return nil, fmt.Errorf("JIRA_USERNAME is required")
	}
	if cfg.Jira.APIToken == "" {
		return nil, fmt.Errorf("JIRA_API_TOKEN is required")
	}
	if cfg.Jira.ProjectKey == "" {
		return nil, fmt.Errorf("JIRA_PROJECT_KEY is required")
	}

	// Validate alertmanager auth configuration
	switch cfg.Alertmanager.AuthType {
	case "basic":
		if cfg.Alertmanager.Username == "" || cfg.Alertmanager.Password == "" {
			return nil, fmt.Errorf("ALERTMANAGER_USERNAME and ALERTMANAGER_PASSWORD are required when ALERTMANAGER_AUTH_TYPE is 'basic'")
		}
	case "bearer":
		if cfg.Alertmanager.BearerToken == "" {
			return nil, fmt.Errorf("ALERTMANAGER_BEARER_TOKEN is required when ALERTMANAGER_AUTH_TYPE is 'bearer'")
		}
	case "none":
		// No validation needed
	default:
		return nil, fmt.Errorf("invalid ALERTMANAGER_AUTH_TYPE: %s (must be 'none', 'basic', or 'bearer')", cfg.Alertmanager.AuthType)
	}

	return cfg, nil
}

// GetSyncDurations converts hour-based configuration to time.Duration
func (c *Config) GetSyncDurations() (expiryThreshold, extensionDuration, defaultSilenceDuration time.Duration) {
	expiryThreshold = time.Duration(c.Sync.ExpiryThresholdHours) * time.Hour
	extensionDuration = time.Duration(c.Sync.ExtensionDurationHours) * time.Hour
	defaultSilenceDuration = time.Duration(c.Sync.DefaultSilenceDurationHours) * time.Hour
	return
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}
