package alertmanager

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewPrometheusAlertManager(t *testing.T) {
	am := NewPrometheusAlertManager("http://localhost:9093")

	if am.baseURL != "http://localhost:9093" {
		t.Errorf("Expected baseURL to be 'http://localhost:9093', got '%s'", am.baseURL)
	}
	if am.authType != "none" {
		t.Errorf("Expected authType to be 'none', got '%s'", am.authType)
	}
	if am.annotationPrefix != "silence-manager" {
		t.Errorf("Expected annotationPrefix to be 'silence-manager', got '%s'", am.annotationPrefix)
	}
}

func TestNewPrometheusAlertManagerWithConfig(t *testing.T) {
	config := AlertManagerConfig{
		BaseURL:          "http://localhost:9093",
		AuthType:         "basic",
		Username:         "admin",
		Password:         "secret",
		AnnotationPrefix: "custom-prefix",
	}

	am := NewPrometheusAlertManagerWithConfig(config)

	if am.baseURL != "http://localhost:9093" {
		t.Errorf("Expected baseURL to be 'http://localhost:9093', got '%s'", am.baseURL)
	}
	if am.authType != "basic" {
		t.Errorf("Expected authType to be 'basic', got '%s'", am.authType)
	}
	if am.username != "admin" {
		t.Errorf("Expected username to be 'admin', got '%s'", am.username)
	}
	if am.password != "secret" {
		t.Errorf("Expected password to be 'secret', got '%s'", am.password)
	}
	if am.annotationPrefix != "custom-prefix" {
		t.Errorf("Expected annotationPrefix to be 'custom-prefix', got '%s'", am.annotationPrefix)
	}
}

func TestNewPrometheusAlertManagerWithConfig_DefaultPrefix(t *testing.T) {
	config := AlertManagerConfig{
		BaseURL:          "http://localhost:9093",
		AnnotationPrefix: "",
	}

	am := NewPrometheusAlertManagerWithConfig(config)

	if am.annotationPrefix != "silence-manager" {
		t.Errorf("Expected default annotationPrefix to be 'silence-manager', got '%s'", am.annotationPrefix)
	}
}

func TestGetSilence_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/silence/test-id" {
			t.Errorf("Expected path '/api/v2/silence/test-id', got '%s'", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET method, got '%s'", r.Method)
		}

		response := promSilence{
			ID:        "test-id",
			CreatedBy: "test-user",
			Comment:   "# silence-manager: PROJ-123\nTest comment",
			StartsAt:  time.Now(),
			EndsAt:    time.Now().Add(24 * time.Hour),
			Matchers: []promMatcher{
				{Name: "alertname", Value: "TestAlert", IsRegex: false, IsEqual: true},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	am := NewPrometheusAlertManager(server.URL)
	silence, err := am.GetSilence("test-id")

	if err != nil {
		t.Fatalf("GetSilence() failed: %v", err)
	}
	if silence.ID != "test-id" {
		t.Errorf("Expected silence ID to be 'test-id', got '%s'", silence.ID)
	}
	if silence.CreatedBy != "test-user" {
		t.Errorf("Expected CreatedBy to be 'test-user', got '%s'", silence.CreatedBy)
	}
	if silence.TicketRef != "PROJ-123" {
		t.Errorf("Expected TicketRef to be 'PROJ-123', got '%s'", silence.TicketRef)
	}
	if len(silence.Matchers) != 1 {
		t.Errorf("Expected 1 matcher, got %d", len(silence.Matchers))
	}
}

func TestGetSilence_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	am := NewPrometheusAlertManager(server.URL)
	_, err := am.GetSilence("nonexistent")

	if err == nil {
		t.Error("Expected error for nonexistent silence")
	}
}

func TestListSilences_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/silences" {
			t.Errorf("Expected path '/api/v2/silences', got '%s'", r.URL.Path)
		}

		response := []promSilence{
			{
				ID:        "silence-1",
				CreatedBy: "user1",
				Comment:   "Test 1",
				StartsAt:  time.Now(),
				EndsAt:    time.Now().Add(24 * time.Hour),
				Status:    &silenceStatus{State: "active"},
			},
			{
				ID:        "silence-2",
				CreatedBy: "user2",
				Comment:   "Test 2",
				StartsAt:  time.Now(),
				EndsAt:    time.Now().Add(24 * time.Hour),
				Status:    &silenceStatus{State: "pending"},
			},
			{
				ID:        "silence-3",
				CreatedBy: "user3",
				Comment:   "Test 3",
				StartsAt:  time.Now(),
				EndsAt:    time.Now().Add(24 * time.Hour),
				Status:    &silenceStatus{State: "expired"},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	am := NewPrometheusAlertManager(server.URL)
	silences, err := am.ListSilences()

	if err != nil {
		t.Fatalf("ListSilences() failed: %v", err)
	}
	// Should only return active and pending silences
	if len(silences) != 2 {
		t.Errorf("Expected 2 silences (active and pending), got %d", len(silences))
	}
}

func TestCreateSilence_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/silences" {
			t.Errorf("Expected path '/api/v2/silences', got '%s'", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got '%s'", r.Method)
		}

		var ps promSilence
		if err := json.NewDecoder(r.Body).Decode(&ps); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		// Verify ticket ref is embedded in comment
		expectedComment := "# silence-manager: PROJ-123\nTest silence"
		if ps.Comment != expectedComment {
			t.Errorf("Expected comment '%s', got '%s'", expectedComment, ps.Comment)
		}

		response := struct {
			SilenceID string `json:"silenceID"`
		}{
			SilenceID: "new-silence-id",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	am := NewPrometheusAlertManager(server.URL)
	silence := &Silence{
		CreatedBy: "test-user",
		Comment:   "Test silence",
		StartsAt:  time.Now(),
		EndsAt:    time.Now().Add(24 * time.Hour),
		TicketRef: "PROJ-123",
		Matchers: []Matcher{
			{Name: "alertname", Value: "TestAlert", IsRegex: false, IsEqual: true},
		},
	}

	id, err := am.CreateSilence(silence)

	if err != nil {
		t.Fatalf("CreateSilence() failed: %v", err)
	}
	if id != "new-silence-id" {
		t.Errorf("Expected silence ID to be 'new-silence-id', got '%s'", id)
	}
}

func TestUpdateSilence_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got '%s'", r.Method)
		}

		var ps promSilence
		if err := json.NewDecoder(r.Body).Decode(&ps); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		// Verify ID is included
		if ps.ID != "existing-id" {
			t.Errorf("Expected ID 'existing-id', got '%s'", ps.ID)
		}

		response := struct {
			SilenceID string `json:"silenceID"`
		}{
			SilenceID: "existing-id",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	am := NewPrometheusAlertManager(server.URL)
	silence := &Silence{
		ID:        "existing-id",
		CreatedBy: "test-user",
		Comment:   "Updated silence",
		StartsAt:  time.Now(),
		EndsAt:    time.Now().Add(48 * time.Hour),
		Matchers: []Matcher{
			{Name: "alertname", Value: "TestAlert", IsRegex: false, IsEqual: true},
		},
	}

	err := am.UpdateSilence(silence)

	if err != nil {
		t.Fatalf("UpdateSilence() failed: %v", err)
	}
}

func TestDeleteSilence_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/silence/test-id" {
			t.Errorf("Expected path '/api/v2/silence/test-id', got '%s'", r.URL.Path)
		}
		if r.Method != http.MethodDelete {
			t.Errorf("Expected DELETE method, got '%s'", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	am := NewPrometheusAlertManager(server.URL)
	err := am.DeleteSilence("test-id")

	if err != nil {
		t.Fatalf("DeleteSilence() failed: %v", err)
	}
}

func TestExtendSilence_Success(t *testing.T) {
	getCount := 0
	updateCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			getCount++
			response := promSilence{
				ID:        "test-id",
				CreatedBy: "test-user",
				Comment:   "Test silence",
				StartsAt:  time.Now(),
				EndsAt:    time.Now().Add(24 * time.Hour),
				Matchers: []promMatcher{
					{Name: "alertname", Value: "TestAlert", IsRegex: false, IsEqual: true},
				},
			}
			json.NewEncoder(w).Encode(response)
		} else if r.Method == http.MethodPost {
			updateCount++
			response := struct {
				SilenceID string `json:"silenceID"`
			}{
				SilenceID: "test-id",
			}
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	am := NewPrometheusAlertManager(server.URL)
	newEndTime := time.Now().Add(72 * time.Hour)
	err := am.ExtendSilence("test-id", newEndTime)

	if err != nil {
		t.Fatalf("ExtendSilence() failed: %v", err)
	}
	if getCount != 1 {
		t.Errorf("Expected 1 GET request, got %d", getCount)
	}
	if updateCount != 1 {
		t.Errorf("Expected 1 POST request, got %d", updateCount)
	}
}

func TestGetAlerts_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/alerts" {
			t.Errorf("Expected path '/api/v2/alerts', got '%s'", r.URL.Path)
		}

		response := []promAlert{
			{
				Labels:      map[string]string{"alertname": "TestAlert", "severity": "critical"},
				Annotations: map[string]string{"summary": "Test alert"},
				StartsAt:    time.Now(),
				EndsAt:      time.Now().Add(1 * time.Hour),
				Status:      struct{ State string `json:"state"` }{State: "active"},
			},
			{
				Labels:      map[string]string{"alertname": "OtherAlert", "severity": "warning"},
				Annotations: map[string]string{"summary": "Other alert"},
				StartsAt:    time.Now(),
				EndsAt:      time.Now().Add(1 * time.Hour),
				Status:      struct{ State string `json:"state"` }{State: "active"},
			},
			{
				Labels:      map[string]string{"alertname": "InactiveAlert"},
				Annotations: map[string]string{"summary": "Inactive alert"},
				StartsAt:    time.Now(),
				EndsAt:      time.Now().Add(1 * time.Hour),
				Status:      struct{ State string `json:"state"` }{State: "resolved"},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	am := NewPrometheusAlertManager(server.URL)
	matchers := []Matcher{
		{Name: "alertname", Value: "TestAlert", IsRegex: false, IsEqual: true},
	}
	alerts, err := am.GetAlerts(matchers)

	if err != nil {
		t.Fatalf("GetAlerts() failed: %v", err)
	}
	// Should only return active alerts that match the matchers
	if len(alerts) != 1 {
		t.Errorf("Expected 1 matching alert, got %d", len(alerts))
	}
	if alerts[0].Labels["alertname"] != "TestAlert" {
		t.Errorf("Expected alertname 'TestAlert', got '%s'", alerts[0].Labels["alertname"])
	}
}

func TestGetAlerts_NoMatchers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := []promAlert{
			{
				Labels:   map[string]string{"alertname": "Alert1"},
				StartsAt: time.Now(),
				EndsAt:   time.Now().Add(1 * time.Hour),
				Status:   struct{ State string `json:"state"` }{State: "active"},
			},
			{
				Labels:   map[string]string{"alertname": "Alert2"},
				StartsAt: time.Now(),
				EndsAt:   time.Now().Add(1 * time.Hour),
				Status:   struct{ State string `json:"state"` }{State: "active"},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	am := NewPrometheusAlertManager(server.URL)
	alerts, err := am.GetAlerts(nil)

	if err != nil {
		t.Fatalf("GetAlerts() failed: %v", err)
	}
	// Should return all active alerts when no matchers specified
	if len(alerts) != 2 {
		t.Errorf("Expected 2 alerts, got %d", len(alerts))
	}
}

func TestExtractTicketRef(t *testing.T) {
	am := NewPrometheusAlertManager("http://localhost:9093")

	tests := []struct {
		name     string
		comment  string
		expected string
	}{
		{
			name:     "Valid ticket ref",
			comment:  "# silence-manager: PROJ-123\nTest comment",
			expected: "PROJ-123",
		},
		{
			name:     "Valid ticket ref without newline",
			comment:  "# silence-manager: PROJ-456",
			expected: "PROJ-456",
		},
		{
			name:     "No ticket ref",
			comment:  "Just a comment",
			expected: "",
		},
		{
			name:     "Wrong prefix",
			comment:  "# other-prefix: PROJ-789\nComment",
			expected: "",
		},
		{
			name:     "Empty comment",
			comment:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := am.extractTicketRef(tt.comment)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestExtractTicketRef_CustomPrefix(t *testing.T) {
	config := AlertManagerConfig{
		BaseURL:          "http://localhost:9093",
		AnnotationPrefix: "custom-prefix",
	}
	am := NewPrometheusAlertManagerWithConfig(config)

	comment := "# custom-prefix: PROJ-999\nTest"
	result := am.extractTicketRef(comment)

	if result != "PROJ-999" {
		t.Errorf("Expected 'PROJ-999', got '%s'", result)
	}
}

func TestMatchesMatchers(t *testing.T) {
	am := NewPrometheusAlertManager("http://localhost:9093")

	alert := &Alert{
		Labels: map[string]string{
			"alertname": "TestAlert",
			"severity":  "critical",
			"instance":  "server1",
		},
	}

	tests := []struct {
		name     string
		matchers []Matcher
		expected bool
	}{
		{
			name: "Single matching matcher",
			matchers: []Matcher{
				{Name: "alertname", Value: "TestAlert", IsRegex: false, IsEqual: true},
			},
			expected: true,
		},
		{
			name: "Multiple matching matchers",
			matchers: []Matcher{
				{Name: "alertname", Value: "TestAlert", IsRegex: false, IsEqual: true},
				{Name: "severity", Value: "critical", IsRegex: false, IsEqual: true},
			},
			expected: true,
		},
		{
			name: "Non-matching matcher",
			matchers: []Matcher{
				{Name: "alertname", Value: "OtherAlert", IsRegex: false, IsEqual: true},
			},
			expected: false,
		},
		{
			name: "Negative matcher (!=) matching",
			matchers: []Matcher{
				{Name: "severity", Value: "warning", IsRegex: false, IsEqual: false},
			},
			expected: true,
		},
		{
			name: "Negative matcher (!=) not matching",
			matchers: []Matcher{
				{Name: "severity", Value: "critical", IsRegex: false, IsEqual: false},
			},
			expected: false,
		},
		{
			name:     "No matchers",
			matchers: []Matcher{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := am.matchesMatchers(alert, tt.matchers)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestAddAuth(t *testing.T) {
	tests := []struct {
		name        string
		authType    string
		username    string
		password    string
		bearerToken string
		checkFunc   func(*testing.T, *http.Request)
	}{
		{
			name:     "No auth",
			authType: "none",
			checkFunc: func(t *testing.T, r *http.Request) {
				if r.Header.Get("Authorization") != "" {
					t.Error("Expected no Authorization header")
				}
			},
		},
		{
			name:     "Basic auth",
			authType: "basic",
			username: "admin",
			password: "secret",
			checkFunc: func(t *testing.T, r *http.Request) {
				user, pass, ok := r.BasicAuth()
				if !ok {
					t.Error("Expected basic auth to be set")
				}
				if user != "admin" || pass != "secret" {
					t.Errorf("Expected basic auth admin:secret, got %s:%s", user, pass)
				}
			},
		},
		{
			name:        "Bearer auth",
			authType:    "bearer",
			bearerToken: "my-token",
			checkFunc: func(t *testing.T, r *http.Request) {
				auth := r.Header.Get("Authorization")
				expected := "Bearer my-token"
				if auth != expected {
					t.Errorf("Expected Authorization header '%s', got '%s'", expected, auth)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := AlertManagerConfig{
				BaseURL:     "http://localhost:9093",
				AuthType:    tt.authType,
				Username:    tt.username,
				Password:    tt.password,
				BearerToken: tt.bearerToken,
			}
			am := NewPrometheusAlertManagerWithConfig(config)

			req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
			am.addAuth(req)

			tt.checkFunc(t, req)
		})
	}
}
