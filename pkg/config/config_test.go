package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadConfig_Success(t *testing.T) {
	// Set up required environment variables
	os.Setenv("JIRA_URL", "https://test.atlassian.net")
	os.Setenv("JIRA_USERNAME", "test@example.com")
	os.Setenv("JIRA_API_TOKEN", "test-token")
	os.Setenv("JIRA_PROJECT_KEY", "TEST")
	defer cleanEnv()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	if cfg.Jira.URL != "https://test.atlassian.net" {
		t.Errorf("Expected Jira URL to be 'https://test.atlassian.net', got '%s'", cfg.Jira.URL)
	}
	if cfg.Jira.Username != "test@example.com" {
		t.Errorf("Expected Jira username to be 'test@example.com', got '%s'", cfg.Jira.Username)
	}
	if cfg.Jira.APIToken != "test-token" {
		t.Errorf("Expected Jira API token to be 'test-token', got '%s'", cfg.Jira.APIToken)
	}
	if cfg.Jira.ProjectKey != "TEST" {
		t.Errorf("Expected Jira project key to be 'TEST', got '%s'", cfg.Jira.ProjectKey)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	// Set up required environment variables with defaults
	os.Setenv("JIRA_URL", "https://test.atlassian.net")
	os.Setenv("JIRA_USERNAME", "test@example.com")
	os.Setenv("JIRA_API_TOKEN", "test-token")
	os.Setenv("JIRA_PROJECT_KEY", "TEST")
	defer cleanEnv()

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	// Check defaults
	if cfg.Alertmanager.AuthType != "none" {
		t.Errorf("Expected auth type to default to 'none', got '%s'", cfg.Alertmanager.AuthType)
	}
	if cfg.Alertmanager.DiscoveryServiceName != "alertmanager" {
		t.Errorf("Expected discovery service name to default to 'alertmanager', got '%s'", cfg.Alertmanager.DiscoveryServiceName)
	}
	if cfg.Alertmanager.DiscoveryServiceLabel != "app=alertmanager" {
		t.Errorf("Expected discovery service label to default to 'app=alertmanager', got '%s'", cfg.Alertmanager.DiscoveryServiceLabel)
	}
	if cfg.Alertmanager.DiscoveryPort != 9093 {
		t.Errorf("Expected discovery port to default to 9093, got %d", cfg.Alertmanager.DiscoveryPort)
	}
	if cfg.Sync.ExpiryThresholdHours != 24 {
		t.Errorf("Expected expiry threshold to default to 24, got %d", cfg.Sync.ExpiryThresholdHours)
	}
	if cfg.Sync.ExtensionDurationHours != 168 {
		t.Errorf("Expected extension duration to default to 168, got %d", cfg.Sync.ExtensionDurationHours)
	}
	if cfg.Sync.DefaultSilenceDurationHours != 168 {
		t.Errorf("Expected default silence duration to default to 168, got %d", cfg.Sync.DefaultSilenceDurationHours)
	}
	if !cfg.Sync.CheckAlerts {
		t.Error("Expected check alerts to default to true")
	}
	if cfg.Sync.AnnotationPrefix != "silence-manager" {
		t.Errorf("Expected annotation prefix to default to 'silence-manager', got '%s'", cfg.Sync.AnnotationPrefix)
	}
}

func TestLoadConfig_AutoDiscovery(t *testing.T) {
	tests := []struct {
		name              string
		alertmanagerURL   string
		autoDiscoverEnv   string
		expectedAutoDisc  bool
	}{
		{
			name:             "Auto-discover when URL empty",
			alertmanagerURL:  "",
			autoDiscoverEnv:  "",
			expectedAutoDisc: true,
		},
		{
			name:             "No auto-discover when URL set",
			alertmanagerURL:  "http://alertmanager:9093",
			autoDiscoverEnv:  "",
			expectedAutoDisc: false,
		},
		{
			name:             "Explicit auto-discover true with URL set",
			alertmanagerURL:  "http://alertmanager:9093",
			autoDiscoverEnv:  "true",
			expectedAutoDisc: true,
		},
		{
			name:             "Auto-discover when URL empty (env var ignored due to OR logic)",
			alertmanagerURL:  "",
			autoDiscoverEnv:  "false",
			expectedAutoDisc: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanEnv()
			os.Setenv("JIRA_URL", "https://test.atlassian.net")
			os.Setenv("JIRA_USERNAME", "test@example.com")
			os.Setenv("JIRA_API_TOKEN", "test-token")
			os.Setenv("JIRA_PROJECT_KEY", "TEST")

			if tt.alertmanagerURL != "" {
				os.Setenv("ALERTMANAGER_URL", tt.alertmanagerURL)
			}
			if tt.autoDiscoverEnv != "" {
				os.Setenv("ALERTMANAGER_AUTO_DISCOVER", tt.autoDiscoverEnv)
			}

			cfg, err := LoadConfig()
			if err != nil {
				t.Fatalf("LoadConfig() failed: %v", err)
			}

			if cfg.Alertmanager.AutoDiscover != tt.expectedAutoDisc {
				t.Errorf("Expected auto-discover to be %v, got %v", tt.expectedAutoDisc, cfg.Alertmanager.AutoDiscover)
			}
		})
	}
}

func TestLoadConfig_MissingJiraURL(t *testing.T) {
	cleanEnv()
	os.Setenv("JIRA_USERNAME", "test@example.com")
	os.Setenv("JIRA_API_TOKEN", "test-token")
	os.Setenv("JIRA_PROJECT_KEY", "TEST")

	_, err := LoadConfig()
	if err == nil {
		t.Error("Expected error when JIRA_URL is missing")
	}
}

func TestLoadConfig_MissingJiraUsername(t *testing.T) {
	cleanEnv()
	os.Setenv("JIRA_URL", "https://test.atlassian.net")
	os.Setenv("JIRA_API_TOKEN", "test-token")
	os.Setenv("JIRA_PROJECT_KEY", "TEST")

	_, err := LoadConfig()
	if err == nil {
		t.Error("Expected error when JIRA_USERNAME is missing")
	}
}

func TestLoadConfig_MissingJiraAPIToken(t *testing.T) {
	cleanEnv()
	os.Setenv("JIRA_URL", "https://test.atlassian.net")
	os.Setenv("JIRA_USERNAME", "test@example.com")
	os.Setenv("JIRA_PROJECT_KEY", "TEST")

	_, err := LoadConfig()
	if err == nil {
		t.Error("Expected error when JIRA_API_TOKEN is missing")
	}
}

func TestLoadConfig_MissingJiraProjectKey(t *testing.T) {
	cleanEnv()
	os.Setenv("JIRA_URL", "https://test.atlassian.net")
	os.Setenv("JIRA_USERNAME", "test@example.com")
	os.Setenv("JIRA_API_TOKEN", "test-token")

	_, err := LoadConfig()
	if err == nil {
		t.Error("Expected error when JIRA_PROJECT_KEY is missing")
	}
}

func TestLoadConfig_BasicAuth(t *testing.T) {
	cleanEnv()
	os.Setenv("JIRA_URL", "https://test.atlassian.net")
	os.Setenv("JIRA_USERNAME", "test@example.com")
	os.Setenv("JIRA_API_TOKEN", "test-token")
	os.Setenv("JIRA_PROJECT_KEY", "TEST")
	os.Setenv("ALERTMANAGER_AUTH_TYPE", "basic")
	os.Setenv("ALERTMANAGER_USERNAME", "admin")
	os.Setenv("ALERTMANAGER_PASSWORD", "secret")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	if cfg.Alertmanager.AuthType != "basic" {
		t.Errorf("Expected auth type to be 'basic', got '%s'", cfg.Alertmanager.AuthType)
	}
	if cfg.Alertmanager.Username != "admin" {
		t.Errorf("Expected alertmanager username to be 'admin', got '%s'", cfg.Alertmanager.Username)
	}
	if cfg.Alertmanager.Password != "secret" {
		t.Errorf("Expected alertmanager password to be 'secret', got '%s'", cfg.Alertmanager.Password)
	}
}

func TestLoadConfig_BasicAuthMissingCredentials(t *testing.T) {
	cleanEnv()
	os.Setenv("JIRA_URL", "https://test.atlassian.net")
	os.Setenv("JIRA_USERNAME", "test@example.com")
	os.Setenv("JIRA_API_TOKEN", "test-token")
	os.Setenv("JIRA_PROJECT_KEY", "TEST")
	os.Setenv("ALERTMANAGER_AUTH_TYPE", "basic")

	_, err := LoadConfig()
	if err == nil {
		t.Error("Expected error when basic auth is set but credentials are missing")
	}
}

func TestLoadConfig_BearerAuth(t *testing.T) {
	cleanEnv()
	os.Setenv("JIRA_URL", "https://test.atlassian.net")
	os.Setenv("JIRA_USERNAME", "test@example.com")
	os.Setenv("JIRA_API_TOKEN", "test-token")
	os.Setenv("JIRA_PROJECT_KEY", "TEST")
	os.Setenv("ALERTMANAGER_AUTH_TYPE", "bearer")
	os.Setenv("ALERTMANAGER_BEARER_TOKEN", "my-bearer-token")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	if cfg.Alertmanager.AuthType != "bearer" {
		t.Errorf("Expected auth type to be 'bearer', got '%s'", cfg.Alertmanager.AuthType)
	}
	if cfg.Alertmanager.BearerToken != "my-bearer-token" {
		t.Errorf("Expected bearer token to be 'my-bearer-token', got '%s'", cfg.Alertmanager.BearerToken)
	}
}

func TestLoadConfig_BearerAuthMissingToken(t *testing.T) {
	cleanEnv()
	os.Setenv("JIRA_URL", "https://test.atlassian.net")
	os.Setenv("JIRA_USERNAME", "test@example.com")
	os.Setenv("JIRA_API_TOKEN", "test-token")
	os.Setenv("JIRA_PROJECT_KEY", "TEST")
	os.Setenv("ALERTMANAGER_AUTH_TYPE", "bearer")

	_, err := LoadConfig()
	if err == nil {
		t.Error("Expected error when bearer auth is set but token is missing")
	}
}

func TestLoadConfig_InvalidAuthType(t *testing.T) {
	cleanEnv()
	os.Setenv("JIRA_URL", "https://test.atlassian.net")
	os.Setenv("JIRA_USERNAME", "test@example.com")
	os.Setenv("JIRA_API_TOKEN", "test-token")
	os.Setenv("JIRA_PROJECT_KEY", "TEST")
	os.Setenv("ALERTMANAGER_AUTH_TYPE", "invalid")

	_, err := LoadConfig()
	if err == nil {
		t.Error("Expected error when auth type is invalid")
	}
}

func TestLoadConfig_CustomValues(t *testing.T) {
	cleanEnv()
	os.Setenv("JIRA_URL", "https://test.atlassian.net")
	os.Setenv("JIRA_USERNAME", "test@example.com")
	os.Setenv("JIRA_API_TOKEN", "test-token")
	os.Setenv("JIRA_PROJECT_KEY", "TEST")
	os.Setenv("ALERTMANAGER_DISCOVERY_SERVICE_NAME", "custom-alertmanager")
	os.Setenv("ALERTMANAGER_DISCOVERY_SERVICE_LABEL", "app=custom")
	os.Setenv("ALERTMANAGER_DISCOVERY_PORT", "8080")
	os.Setenv("ALERTMANAGER_DISCOVERY_NAMESPACES", "ns1,ns2,ns3")
	os.Setenv("SYNC_EXPIRY_THRESHOLD_HOURS", "12")
	os.Setenv("SYNC_EXTENSION_DURATION_HOURS", "48")
	os.Setenv("SYNC_DEFAULT_SILENCE_DURATION_HOURS", "72")
	os.Setenv("SYNC_CHECK_ALERTS", "false")
	os.Setenv("SYNC_ANNOTATION_PREFIX", "custom-prefix")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() failed: %v", err)
	}

	if cfg.Alertmanager.DiscoveryServiceName != "custom-alertmanager" {
		t.Errorf("Expected discovery service name to be 'custom-alertmanager', got '%s'", cfg.Alertmanager.DiscoveryServiceName)
	}
	if cfg.Alertmanager.DiscoveryServiceLabel != "app=custom" {
		t.Errorf("Expected discovery service label to be 'app=custom', got '%s'", cfg.Alertmanager.DiscoveryServiceLabel)
	}
	if cfg.Alertmanager.DiscoveryPort != 8080 {
		t.Errorf("Expected discovery port to be 8080, got %d", cfg.Alertmanager.DiscoveryPort)
	}
	if len(cfg.Alertmanager.DiscoveryNamespaces) != 3 {
		t.Errorf("Expected 3 discovery namespaces, got %d", len(cfg.Alertmanager.DiscoveryNamespaces))
	}
	if cfg.Alertmanager.DiscoveryNamespaces[0] != "ns1" || cfg.Alertmanager.DiscoveryNamespaces[1] != "ns2" || cfg.Alertmanager.DiscoveryNamespaces[2] != "ns3" {
		t.Errorf("Expected discovery namespaces to be ['ns1', 'ns2', 'ns3'], got %v", cfg.Alertmanager.DiscoveryNamespaces)
	}
	if cfg.Sync.ExpiryThresholdHours != 12 {
		t.Errorf("Expected expiry threshold to be 12, got %d", cfg.Sync.ExpiryThresholdHours)
	}
	if cfg.Sync.ExtensionDurationHours != 48 {
		t.Errorf("Expected extension duration to be 48, got %d", cfg.Sync.ExtensionDurationHours)
	}
	if cfg.Sync.DefaultSilenceDurationHours != 72 {
		t.Errorf("Expected default silence duration to be 72, got %d", cfg.Sync.DefaultSilenceDurationHours)
	}
	if cfg.Sync.CheckAlerts {
		t.Error("Expected check alerts to be false")
	}
	if cfg.Sync.AnnotationPrefix != "custom-prefix" {
		t.Errorf("Expected annotation prefix to be 'custom-prefix', got '%s'", cfg.Sync.AnnotationPrefix)
	}
}

func TestGetSyncDurations(t *testing.T) {
	cfg := &Config{
		Sync: SyncConfig{
			ExpiryThresholdHours:        24,
			ExtensionDurationHours:      168,
			DefaultSilenceDurationHours: 72,
		},
	}

	expiry, extension, defaultDuration := cfg.GetSyncDurations()

	if expiry != 24*time.Hour {
		t.Errorf("Expected expiry threshold to be 24h, got %v", expiry)
	}
	if extension != 168*time.Hour {
		t.Errorf("Expected extension duration to be 168h, got %v", extension)
	}
	if defaultDuration != 72*time.Hour {
		t.Errorf("Expected default silence duration to be 72h, got %v", defaultDuration)
	}
}

func TestGetEnvSlice_WithSpaces(t *testing.T) {
	os.Setenv("TEST_SLICE", " ns1 , ns2 , ns3 ")
	defer os.Unsetenv("TEST_SLICE")

	result := getEnvSlice("TEST_SLICE", []string{"default"})

	if len(result) != 3 {
		t.Errorf("Expected 3 items, got %d", len(result))
	}
	if result[0] != "ns1" || result[1] != "ns2" || result[2] != "ns3" {
		t.Errorf("Expected trimmed values, got %v", result)
	}
}

func TestGetEnvSlice_EmptyItems(t *testing.T) {
	os.Setenv("TEST_SLICE", "ns1,,ns2,,,ns3")
	defer os.Unsetenv("TEST_SLICE")

	result := getEnvSlice("TEST_SLICE", []string{"default"})

	if len(result) != 3 {
		t.Errorf("Expected 3 items (empty items removed), got %d", len(result))
	}
}

func TestGetEnvSlice_Default(t *testing.T) {
	result := getEnvSlice("NONEXISTENT_VAR", []string{"default1", "default2"})

	if len(result) != 2 {
		t.Errorf("Expected 2 default items, got %d", len(result))
	}
	if result[0] != "default1" || result[1] != "default2" {
		t.Errorf("Expected default values, got %v", result)
	}
}

func TestGetEnvInt_InvalidValue(t *testing.T) {
	os.Setenv("TEST_INT", "not-a-number")
	defer os.Unsetenv("TEST_INT")

	result := getEnvInt("TEST_INT", 42)

	if result != 42 {
		t.Errorf("Expected default value 42 for invalid int, got %d", result)
	}
}

func TestGetEnvBool_InvalidValue(t *testing.T) {
	os.Setenv("TEST_BOOL", "not-a-bool")
	defer os.Unsetenv("TEST_BOOL")

	result := getEnvBool("TEST_BOOL", true)

	if !result {
		t.Error("Expected default value true for invalid bool")
	}
}

// Helper function to clean environment variables
func cleanEnv() {
	vars := []string{
		"JIRA_URL", "JIRA_USERNAME", "JIRA_API_TOKEN", "JIRA_PROJECT_KEY",
		"ALERTMANAGER_URL", "ALERTMANAGER_AUTO_DISCOVER", "ALERTMANAGER_AUTH_TYPE",
		"ALERTMANAGER_USERNAME", "ALERTMANAGER_PASSWORD", "ALERTMANAGER_BEARER_TOKEN",
		"ALERTMANAGER_DISCOVERY_SERVICE_NAME", "ALERTMANAGER_DISCOVERY_SERVICE_LABEL",
		"ALERTMANAGER_DISCOVERY_PORT", "ALERTMANAGER_DISCOVERY_NAMESPACES",
		"SYNC_EXPIRY_THRESHOLD_HOURS", "SYNC_EXTENSION_DURATION_HOURS",
		"SYNC_DEFAULT_SILENCE_DURATION_HOURS", "SYNC_CHECK_ALERTS", "SYNC_ANNOTATION_PREFIX",
	}
	for _, v := range vars {
		os.Unsetenv(v)
	}
}
