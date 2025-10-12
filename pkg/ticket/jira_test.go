package ticket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewJiraTicketSystem(t *testing.T) {
	jira := NewJiraTicketSystem("https://test.atlassian.net", "user@test.com", "token", "PROJ", "custom-prefix")

	if jira.baseURL != "https://test.atlassian.net" {
		t.Errorf("Expected baseURL to be 'https://test.atlassian.net', got '%s'", jira.baseURL)
	}
	if jira.username != "user@test.com" {
		t.Errorf("Expected username to be 'user@test.com', got '%s'", jira.username)
	}
	if jira.apiToken != "token" {
		t.Errorf("Expected apiToken to be 'token', got '%s'", jira.apiToken)
	}
	if jira.projectKey != "PROJ" {
		t.Errorf("Expected projectKey to be 'PROJ', got '%s'", jira.projectKey)
	}
	if jira.annotationPrefix != "custom-prefix" {
		t.Errorf("Expected annotationPrefix to be 'custom-prefix', got '%s'", jira.annotationPrefix)
	}
}

func TestNewJiraTicketSystem_DefaultPrefix(t *testing.T) {
	jira := NewJiraTicketSystem("https://test.atlassian.net", "user@test.com", "token", "PROJ", "")

	if jira.annotationPrefix != "silence-manager" {
		t.Errorf("Expected default annotationPrefix to be 'silence-manager', got '%s'", jira.annotationPrefix)
	}
}

func TestNewJiraTicketSystem_TrimSlash(t *testing.T) {
	jira := NewJiraTicketSystem("https://test.atlassian.net/", "user@test.com", "token", "PROJ", "")

	if jira.baseURL != "https://test.atlassian.net" {
		t.Errorf("Expected trailing slash to be trimmed, got '%s'", jira.baseURL)
	}
}

func TestGetTicket_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/PROJ-123" {
			t.Errorf("Expected path '/rest/api/3/issue/PROJ-123', got '%s'", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET method, got '%s'", r.Method)
		}

		// Check auth
		user, pass, ok := r.BasicAuth()
		if !ok || user != "user@test.com" || pass != "token" {
			t.Error("Expected basic auth to be set correctly")
		}

		response := jiraIssue{
			ID:  "10001",
			Key: "PROJ-123",
			Fields: jiraFields{
				Summary: "Test issue",
				Description: &jiraDescription{
					Type:    "doc",
					Version: 1,
					Content: []jiraDescriptionContent{
						{
							Type: "paragraph",
							Content: []jiraDescriptionParagraph{
								{Type: "text", Text: "silence-manager: silence-id-123"},
								{Type: "text", Text: "Test description"},
							},
						},
					},
				},
				Status:  &jiraStatus{Name: "Open"},
				Created: time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
				Updated: time.Now().Format(time.RFC3339),
				Labels:  []string{"label1", "label2"},
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	jira := NewJiraTicketSystem(server.URL, "user@test.com", "token", "PROJ", "silence-manager")
	ticket, err := jira.GetTicket("PROJ-123")

	if err != nil {
		t.Fatalf("GetTicket() failed: %v", err)
	}
	if ticket.Key != "PROJ-123" {
		t.Errorf("Expected ticket key to be 'PROJ-123', got '%s'", ticket.Key)
	}
	if ticket.Summary != "Test issue" {
		t.Errorf("Expected summary to be 'Test issue', got '%s'", ticket.Summary)
	}
	if ticket.SilenceRef != "silence-id-123" {
		t.Errorf("Expected silence ref to be 'silence-id-123', got '%s'", ticket.SilenceRef)
	}
	if ticket.Status != StatusOpen {
		t.Errorf("Expected status to be StatusOpen, got %v", ticket.Status)
	}
	if len(ticket.Labels) != 2 {
		t.Errorf("Expected 2 labels, got %d", len(ticket.Labels))
	}
}

func TestGetTicket_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	jira := NewJiraTicketSystem(server.URL, "user@test.com", "token", "PROJ", "")
	_, err := jira.GetTicket("NONEXISTENT")

	if err == nil {
		t.Error("Expected error for nonexistent ticket")
	}
}

func TestCreateTicket_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue" {
			t.Errorf("Expected path '/rest/api/3/issue', got '%s'", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got '%s'", r.Method)
		}

		var ji jiraIssue
		if err := json.NewDecoder(r.Body).Decode(&ji); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		// Verify fields
		if ji.Fields.Summary != "Test ticket" {
			t.Errorf("Expected summary 'Test ticket', got '%s'", ji.Fields.Summary)
		}
		if ji.Fields.Project == nil || ji.Fields.Project.Key != "PROJ" {
			t.Error("Expected project key to be set")
		}
		if ji.Fields.IssueType == nil || ji.Fields.IssueType.Name != "Task" {
			t.Error("Expected issue type to be 'Task'")
		}

		w.WriteHeader(http.StatusCreated)
		response := jiraIssue{
			Key: "PROJ-999",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	jira := NewJiraTicketSystem(server.URL, "user@test.com", "token", "PROJ", "silence-manager")
	ticket := &Ticket{
		Summary:     "Test ticket",
		Description: "Test description",
		SilenceRef:  "silence-123",
		Labels:      []string{"test"},
	}

	key, err := jira.CreateTicket(ticket)

	if err != nil {
		t.Fatalf("CreateTicket() failed: %v", err)
	}
	if key != "PROJ-999" {
		t.Errorf("Expected ticket key to be 'PROJ-999', got '%s'", key)
	}
}

func TestUpdateTicket_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/PROJ-123" {
			t.Errorf("Expected path '/rest/api/3/issue/PROJ-123', got '%s'", r.URL.Path)
		}
		if r.Method != http.MethodPut {
			t.Errorf("Expected PUT method, got '%s'", r.Method)
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	jira := NewJiraTicketSystem(server.URL, "user@test.com", "token", "PROJ", "")
	ticket := &Ticket{
		Key:         "PROJ-123",
		Summary:     "Updated summary",
		Description: "Updated description",
	}

	err := jira.UpdateTicket(ticket)

	if err != nil {
		t.Fatalf("UpdateTicket() failed: %v", err)
	}
}

func TestAddComment_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/PROJ-123/comment" {
			t.Errorf("Expected path '/rest/api/3/issue/PROJ-123/comment', got '%s'", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got '%s'", r.Method)
		}

		var commentBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&commentBody); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		// Verify comment structure
		body, ok := commentBody["body"].(map[string]interface{})
		if !ok {
			t.Error("Expected body field")
		}
		if body["type"] != "doc" {
			t.Error("Expected doc type")
		}

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	jira := NewJiraTicketSystem(server.URL, "user@test.com", "token", "PROJ", "")
	err := jira.AddComment("PROJ-123", "Test comment")

	if err != nil {
		t.Fatalf("AddComment() failed: %v", err)
	}
}

func TestReopenTicket_Success(t *testing.T) {
	callOrder := []string{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/issue/PROJ-123/comment" && r.Method == http.MethodPost {
			callOrder = append(callOrder, "comment")
			w.WriteHeader(http.StatusCreated)
		} else if r.URL.Path == "/rest/api/3/issue/PROJ-123/transitions" && r.Method == http.MethodGet {
			callOrder = append(callOrder, "get-transitions")
			response := jiraTransitionsResponse{
				Transitions: []jiraTransition{
					{ID: "1", Name: "Reopen", To: struct{ Name string `json:"name"` }{Name: "Open"}},
					{ID: "2", Name: "Close", To: struct{ Name string `json:"name"` }{Name: "Closed"}},
				},
			}
			json.NewEncoder(w).Encode(response)
		} else if r.URL.Path == "/rest/api/3/issue/PROJ-123/transitions" && r.Method == http.MethodPost {
			callOrder = append(callOrder, "do-transition")
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	jira := NewJiraTicketSystem(server.URL, "user@test.com", "token", "PROJ", "")
	err := jira.ReopenTicket("PROJ-123", "Reopening ticket")

	if err != nil {
		t.Fatalf("ReopenTicket() failed: %v", err)
	}

	// Verify call order
	if len(callOrder) != 3 {
		t.Errorf("Expected 3 API calls, got %d", len(callOrder))
	}
	if callOrder[0] != "comment" || callOrder[1] != "get-transitions" || callOrder[2] != "do-transition" {
		t.Errorf("Unexpected call order: %v", callOrder)
	}
}

func TestReopenTicket_NoTransitionAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/issue/PROJ-123/transitions" && r.Method == http.MethodGet {
			response := jiraTransitionsResponse{
				Transitions: []jiraTransition{
					{ID: "1", Name: "Close", To: struct{ Name string `json:"name"` }{Name: "Closed"}},
				},
			}
			json.NewEncoder(w).Encode(response)
		}
	}))
	defer server.Close()

	jira := NewJiraTicketSystem(server.URL, "user@test.com", "token", "PROJ", "")
	err := jira.ReopenTicket("PROJ-123", "")

	if err == nil {
		t.Error("Expected error when no reopen transition is available")
	}
}

func TestCloseTicket_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/issue/PROJ-123/comment" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		} else if r.URL.Path == "/rest/api/3/issue/PROJ-123/transitions" && r.Method == http.MethodGet {
			response := jiraTransitionsResponse{
				Transitions: []jiraTransition{
					{ID: "1", Name: "Close", To: struct{ Name string `json:"name"` }{Name: "Closed"}},
				},
			}
			json.NewEncoder(w).Encode(response)
		} else if r.URL.Path == "/rest/api/3/issue/PROJ-123/transitions" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	jira := NewJiraTicketSystem(server.URL, "user@test.com", "token", "PROJ", "")
	err := jira.CloseTicket("PROJ-123", "Closing ticket")

	if err != nil {
		t.Fatalf("CloseTicket() failed: %v", err)
	}
}

func TestIsResolved(t *testing.T) {
	jira := NewJiraTicketSystem("http://test.com", "user", "token", "PROJ", "")

	tests := []struct {
		name     string
		status   TicketStatus
		expected bool
	}{
		{"Resolved status", StatusResolved, true},
		{"Open status", StatusOpen, false},
		{"Closed status", StatusClosed, false},
		{"In progress status", StatusInProgress, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ticket := &Ticket{Status: tt.status}
			result := jira.IsResolved(ticket)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsClosed(t *testing.T) {
	jira := NewJiraTicketSystem("http://test.com", "user", "token", "PROJ", "")

	tests := []struct {
		name     string
		status   TicketStatus
		expected bool
	}{
		{"Closed status", StatusClosed, true},
		{"Resolved status", StatusResolved, true},
		{"Open status", StatusOpen, false},
		{"In progress status", StatusInProgress, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ticket := &Ticket{Status: tt.status}
			result := jira.IsClosed(ticket)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsOpen(t *testing.T) {
	jira := NewJiraTicketSystem("http://test.com", "user", "token", "PROJ", "")

	tests := []struct {
		name     string
		status   TicketStatus
		expected bool
	}{
		{"Open status", StatusOpen, true},
		{"In progress status", StatusInProgress, true},
		{"Closed status", StatusClosed, false},
		{"Resolved status", StatusResolved, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ticket := &Ticket{Status: tt.status}
			result := jira.IsOpen(ticket)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestMapJiraStatus(t *testing.T) {
	jira := NewJiraTicketSystem("http://test.com", "user", "token", "PROJ", "")

	tests := []struct {
		name           string
		jiraStatus     string
		expectedStatus TicketStatus
	}{
		{"Open", "Open", StatusOpen},
		{"To Do", "To Do", StatusOpen},
		{"to do", "to do", StatusOpen},
		{"In Progress", "In Progress", StatusInProgress},
		{"In Review", "In Review", StatusInProgress},
		{"Resolved", "Resolved", StatusResolved},
		{"Done", "Done", StatusResolved},
		{"Closed", "Closed", StatusClosed},
		{"Reopened", "Reopened", StatusOpen}, // "reopened" contains "open", matches first
		{"Unknown", "Some Other Status", StatusOpen}, // defaults to open
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := jira.mapJiraStatus(tt.jiraStatus)
			if result != tt.expectedStatus {
				t.Errorf("Expected %v for status '%s', got %v", tt.expectedStatus, tt.jiraStatus, result)
			}
		})
	}
}

func TestExtractSilenceRef(t *testing.T) {
	jira := NewJiraTicketSystem("http://test.com", "user", "token", "PROJ", "silence-manager")

	tests := []struct {
		name        string
		description string
		expected    string
	}{
		{
			name:        "Valid silence ref",
			description: "silence-manager: silence-123\nRest of description",
			expected:    "silence-123",
		},
		{
			name:        "Valid silence ref without newline",
			description: "silence-manager: silence-456",
			expected:    "silence-456",
		},
		{
			name:        "No silence ref",
			description: "Just a description",
			expected:    "",
		},
		{
			name:        "Wrong prefix",
			description: "other-prefix: silence-789",
			expected:    "",
		},
		{
			name:        "Empty description",
			description: "",
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := jira.extractSilenceRef(tt.description)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestExtractSilenceRef_CustomPrefix(t *testing.T) {
	jira := NewJiraTicketSystem("http://test.com", "user", "token", "PROJ", "custom-prefix")

	description := "custom-prefix: silence-999\nDescription"
	result := jira.extractSilenceRef(description)

	if result != "silence-999" {
		t.Errorf("Expected 'silence-999', got '%s'", result)
	}
}

func TestExtractDescriptionText(t *testing.T) {
	jira := NewJiraTicketSystem("http://test.com", "user", "token", "PROJ", "")

	desc := &jiraDescription{
		Type:    "doc",
		Version: 1,
		Content: []jiraDescriptionContent{
			{
				Type: "paragraph",
				Content: []jiraDescriptionParagraph{
					{Type: "text", Text: "First line"},
					{Type: "text", Text: "Second line"},
				},
			},
			{
				Type: "paragraph",
				Content: []jiraDescriptionParagraph{
					{Type: "text", Text: "Third line"},
				},
			},
		},
	}

	result := jira.extractDescriptionText(desc)
	expected := "First line\nSecond line\nThird line"

	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestCreateJiraDescription(t *testing.T) {
	jira := NewJiraTicketSystem("http://test.com", "user", "token", "PROJ", "")

	text := "Test description"
	desc := jira.createJiraDescription(text)

	if desc.Type != "doc" {
		t.Errorf("Expected type 'doc', got '%s'", desc.Type)
	}
	if desc.Version != 1 {
		t.Errorf("Expected version 1, got %d", desc.Version)
	}
	if len(desc.Content) != 1 {
		t.Errorf("Expected 1 content item, got %d", len(desc.Content))
	}
	if len(desc.Content[0].Content) != 1 {
		t.Errorf("Expected 1 paragraph, got %d", len(desc.Content[0].Content))
	}
	if desc.Content[0].Content[0].Text != text {
		t.Errorf("Expected text '%s', got '%s'", text, desc.Content[0].Content[0].Text)
	}
}

func TestConvertToJiraIssue_WithSilenceRef(t *testing.T) {
	jira := NewJiraTicketSystem("http://test.com", "user", "token", "PROJ", "silence-manager")

	ticket := &Ticket{
		Summary:     "Test ticket",
		Description: "Original description",
		SilenceRef:  "silence-123",
		Labels:      []string{"label1"},
	}

	ji := jira.convertToJiraIssue(ticket)

	if ji.Fields.Summary != "Test ticket" {
		t.Errorf("Expected summary 'Test ticket', got '%s'", ji.Fields.Summary)
	}

	descText := jira.extractDescriptionText(ji.Fields.Description)
	expectedDesc := "silence-manager: silence-123\n\nOriginal description"
	if descText != expectedDesc {
		t.Errorf("Expected description '%s', got '%s'", expectedDesc, descText)
	}

	if len(ji.Fields.Labels) != 1 || ji.Fields.Labels[0] != "label1" {
		t.Errorf("Expected labels ['label1'], got %v", ji.Fields.Labels)
	}
}

func TestConvertToJiraIssue_WithoutSilenceRef(t *testing.T) {
	jira := NewJiraTicketSystem("http://test.com", "user", "token", "PROJ", "silence-manager")

	ticket := &Ticket{
		Summary:     "Test ticket",
		Description: "Original description",
		SilenceRef:  "",
		Labels:      []string{},
	}

	ji := jira.convertToJiraIssue(ticket)

	descText := jira.extractDescriptionText(ji.Fields.Description)
	if descText != "Original description" {
		t.Errorf("Expected description 'Original description', got '%s'", descText)
	}
}
