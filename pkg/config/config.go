package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config represents the application configuration
type Config struct {
	Alertmanager AlertmanagerConfig
	Jira         JiraConfig
	Sync         SyncConfig
	Metrics      MetricsConfig
}

// AlertmanagerConfig holds Alertmanager-specific configuration
type AlertmanagerConfig struct {
	URL                   string
	AuthType              string // "none", "basic", "bearer"
	Username              string // For basic auth
	Password              string // For basic auth
	BearerToken           string // For bearer token auth
	// Auto-discovery configuration
	AutoDiscover          bool
	DiscoveryServiceName  string   // Service name pattern to match
	DiscoveryServiceLabel string   // Label selector for discovery
	DiscoveryPort         int      // Port to use for discovered services
	DiscoveryNamespaces   []string // Preferred namespaces to search first
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

// MetricsConfig holds metrics publishing configuration
type MetricsConfig struct {
	Enabled               bool
	Backend               string // "pushgateway", "otel", or ""
	URL                   string
	JobName               string // For Pushgateway
	OTelInsecure          bool   // For OTel - use insecure connection
	// Auto-discovery configuration
	AutoDiscover          bool
	DiscoveryServiceName  string   // Service name pattern to match
	DiscoveryServiceLabel string   // Label selector for discovery
	DiscoveryPort         int      // Port to use for discovered services
	DiscoveryNamespaces   []string // Preferred namespaces to search first
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	alertmanagerURL := getEnv("ALERTMANAGER_URL", "")
	autoDiscover := alertmanagerURL == "" || getEnvBool("ALERTMANAGER_AUTO_DISCOVER", alertmanagerURL == "")

	// Metrics configuration
	metricsEnabled := getEnvBool("METRICS_ENABLED", false)
	metricsURL := getEnv("METRICS_URL", "")
	metricsBackend := getEnv("METRICS_BACKEND", "")
	metricsAutoDiscover := metricsURL == "" && metricsEnabled && metricsBackend != ""

	cfg := &Config{
		Alertmanager: AlertmanagerConfig{
			URL:                   alertmanagerURL,
			AuthType:              getEnv("ALERTMANAGER_AUTH_TYPE", "none"),
			Username:              getEnv("ALERTMANAGER_USERNAME", ""),
			Password:              getEnv("ALERTMANAGER_PASSWORD", ""),
			BearerToken:           getEnv("ALERTMANAGER_BEARER_TOKEN", ""),
			AutoDiscover:          autoDiscover,
			DiscoveryServiceName:  getEnv("ALERTMANAGER_DISCOVERY_SERVICE_NAME", "alertmanager"),
			DiscoveryServiceLabel: getEnv("ALERTMANAGER_DISCOVERY_SERVICE_LABEL", "app=alertmanager"),
			DiscoveryPort:         getEnvInt("ALERTMANAGER_DISCOVERY_PORT", 9093),
			DiscoveryNamespaces:   getEnvSlice("ALERTMANAGER_DISCOVERY_NAMESPACES", []string{"monitoring", "default"}),
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
		Metrics: MetricsConfig{
			Enabled:               metricsEnabled,
			Backend:               metricsBackend,
			URL:                   metricsURL,
			JobName:               getEnv("METRICS_PUSHGATEWAY_JOB_NAME", "silence_manager"),
			OTelInsecure:          getEnvBool("METRICS_OTEL_INSECURE", true),
			AutoDiscover:          metricsAutoDiscover,
			DiscoveryServiceName:  getEnv("METRICS_DISCOVERY_SERVICE_NAME", ""),
			DiscoveryServiceLabel: getEnv("METRICS_DISCOVERY_SERVICE_LABEL", ""),
			DiscoveryPort:         getEnvInt("METRICS_DISCOVERY_PORT", 0),
			DiscoveryNamespaces:   getEnvSlice("METRICS_DISCOVERY_NAMESPACES", []string{"monitoring", "default"}),
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

	// Validate metrics configuration
	if cfg.Metrics.Enabled {
		if cfg.Metrics.Backend == "" {
			return nil, fmt.Errorf("METRICS_BACKEND is required when METRICS_ENABLED is true (must be 'pushgateway' or 'otel')")
		}
		if cfg.Metrics.Backend != "pushgateway" && cfg.Metrics.Backend != "otel" {
			return nil, fmt.Errorf("invalid METRICS_BACKEND: %s (must be 'pushgateway' or 'otel')", cfg.Metrics.Backend)
		}
		// URL is not required if auto-discovery is enabled
		if !cfg.Metrics.AutoDiscover && cfg.Metrics.URL == "" {
			return nil, fmt.Errorf("METRICS_URL is required when metrics are enabled and auto-discovery is disabled")
		}
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

func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		// Split by comma and trim spaces
		var result []string
		for _, item := range strings.Split(value, ",") {
			trimmed := strings.TrimSpace(item)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}

