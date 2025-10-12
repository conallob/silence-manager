package ticket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// JiraTicketSystem implements the TicketSystem interface for Atlassian Jira
type JiraTicketSystem struct {
	baseURL          string
	username         string
	apiToken         string
	projectKey       string
	httpClient       *http.Client
	annotationPrefix string
}

// NewJiraTicketSystem creates a new Jira ticket system client
func NewJiraTicketSystem(baseURL, username, apiToken, projectKey, annotationPrefix string) *JiraTicketSystem {
	prefix := annotationPrefix
	if prefix == "" {
		prefix = "silence-manager"
	}
	return &JiraTicketSystem{
		baseURL:          strings.TrimSuffix(baseURL, "/"),
		username:         username,
		apiToken:         apiToken,
		projectKey:       projectKey,
		annotationPrefix: prefix,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Jira API structures
type jiraIssue struct {
	ID     string     `json:"id,omitempty"`
	Key    string     `json:"key,omitempty"`
	Fields jiraFields `json:"fields"`
}

type jiraFields struct {
	Summary     string           `json:"summary,omitempty"`
	Description *jiraDescription `json:"description,omitempty"`
	Status      *jiraStatus      `json:"status,omitempty"`
	Created     string           `json:"created,omitempty"`
	Updated     string           `json:"updated,omitempty"`
	Labels      []string         `json:"labels,omitempty"`
	Assignee    *jiraUser        `json:"assignee,omitempty"`
	Project     *jiraProject     `json:"project,omitempty"`
	IssueType   *jiraIssueType   `json:"issuetype,omitempty"`
}

type jiraDescription struct {
	Type    string                   `json:"type"`
	Version int                      `json:"version"`
	Content []jiraDescriptionContent `json:"content"`
}

type jiraDescriptionContent struct {
	Type    string                     `json:"type"`
	Content []jiraDescriptionParagraph `json:"content,omitempty"`
}

type jiraDescriptionParagraph struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type jiraStatus struct {
	Name string `json:"name"`
}

type jiraUser struct {
	AccountID string `json:"accountId,omitempty"`
	Name      string `json:"name,omitempty"`
}

type jiraProject struct {
	Key string `json:"key"`
}

type jiraIssueType struct {
	Name string `json:"name"`
}

type jiraComment struct {
	Body string `json:"body"`
}

type jiraTransition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   struct {
		Name string `json:"name"`
	} `json:"to"`
}

type jiraTransitionsResponse struct {
	Transitions []jiraTransition `json:"transitions"`
}

// GetTicket retrieves a ticket by its key
func (j *JiraTicketSystem) GetTicket(key string) (*Ticket, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s", j.baseURL, key)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(j.username, j.apiToken)
	req.Header.Set("Accept", "application/json")

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get ticket: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("ticket not found: %s", key)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var ji jiraIssue
	if err := json.NewDecoder(resp.Body).Decode(&ji); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return j.convertFromJiraIssue(&ji), nil
}

// CreateTicket creates a new ticket and returns its key
func (j *JiraTicketSystem) CreateTicket(ticket *Ticket) (string, error) {
	ji := j.convertToJiraIssue(ticket)
	ji.Fields.Project = &jiraProject{Key: j.projectKey}
	ji.Fields.IssueType = &jiraIssueType{Name: "Task"}

	body, err := json.Marshal(ji)
	if err != nil {
		return "", fmt.Errorf("failed to marshal ticket: %w", err)
	}

	url := fmt.Sprintf("%s/rest/api/3/issue", j.baseURL)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(j.username, j.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to create ticket: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		responseBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(responseBody))
	}

	var result jiraIssue
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Key, nil
}

// UpdateTicket updates an existing ticket
func (j *JiraTicketSystem) UpdateTicket(ticket *Ticket) error {
	ji := j.convertToJiraIssue(ticket)

	body, err := json.Marshal(ji)
	if err != nil {
		return fmt.Errorf("failed to marshal ticket: %w", err)
	}

	url := fmt.Sprintf("%s/rest/api/3/issue/%s", j.baseURL, ticket.Key)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(j.username, j.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update ticket: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(responseBody))
	}

	return nil
}

// ReopenTicket reopens a closed/resolved ticket
func (j *JiraTicketSystem) ReopenTicket(key string, comment string) error {
	// First add a comment
	if comment != "" {
		if err := j.AddComment(key, comment); err != nil {
			return fmt.Errorf("failed to add comment: %w", err)
		}
	}

	// Get available transitions
	transitions, err := j.getTransitions(key)
	if err != nil {
		return fmt.Errorf("failed to get transitions: %w", err)
	}

	// Find "Reopen" or similar transition
	var transitionID string
	for _, t := range transitions {
		if strings.EqualFold(t.Name, "reopen") || strings.EqualFold(t.To.Name, "open") ||
			strings.EqualFold(t.To.Name, "reopened") || strings.EqualFold(t.To.Name, "to do") {
			transitionID = t.ID
			break
		}
	}

	if transitionID == "" {
		return fmt.Errorf("no reopen transition found for ticket %s", key)
	}

	return j.doTransition(key, transitionID)
}

// CloseTicket marks a ticket as closed
func (j *JiraTicketSystem) CloseTicket(key string, comment string) error {
	// First add a comment
	if comment != "" {
		if err := j.AddComment(key, comment); err != nil {
			return fmt.Errorf("failed to add comment: %w", err)
		}
	}

	// Get available transitions
	transitions, err := j.getTransitions(key)
	if err != nil {
		return fmt.Errorf("failed to get transitions: %w", err)
	}

	// Find "Close" or "Done" transition
	var transitionID string
	for _, t := range transitions {
		if strings.EqualFold(t.Name, "close") || strings.EqualFold(t.Name, "done") ||
			strings.EqualFold(t.To.Name, "closed") || strings.EqualFold(t.To.Name, "done") {
			transitionID = t.ID
			break
		}
	}

	if transitionID == "" {
		return fmt.Errorf("no close transition found for ticket %s", key)
	}

	return j.doTransition(key, transitionID)
}

// AddComment adds a comment to a ticket
func (j *JiraTicketSystem) AddComment(key string, comment string) error {
	commentBody := map[string]interface{}{
		"body": map[string]interface{}{
			"type":    "doc",
			"version": 1,
			"content": []map[string]interface{}{
				{
					"type": "paragraph",
					"content": []map[string]interface{}{
						{
							"type": "text",
							"text": comment,
						},
					},
				},
			},
		},
	}

	body, err := json.Marshal(commentBody)
	if err != nil {
		return fmt.Errorf("failed to marshal comment: %w", err)
	}

	url := fmt.Sprintf("%s/rest/api/3/issue/%s/comment", j.baseURL, key)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(j.username, j.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to add comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(responseBody))
	}

	return nil
}

// IsResolved checks if a ticket is in a resolved state
func (j *JiraTicketSystem) IsResolved(ticket *Ticket) bool {
	return ticket.Status == StatusResolved
}

// IsClosed checks if a ticket is in a closed state
func (j *JiraTicketSystem) IsClosed(ticket *Ticket) bool {
	return ticket.Status == StatusClosed || ticket.Status == StatusResolved
}

// IsOpen checks if a ticket is in an open state
func (j *JiraTicketSystem) IsOpen(ticket *Ticket) bool {
	return ticket.Status == StatusOpen || ticket.Status == StatusInProgress
}

// Helper functions
func (j *JiraTicketSystem) getTransitions(key string) ([]jiraTransition, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s/transitions", j.baseURL, key)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(j.username, j.apiToken)
	req.Header.Set("Accept", "application/json")

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get transitions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var result jiraTransitionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Transitions, nil
}

func (j *JiraTicketSystem) doTransition(key string, transitionID string) error {
	transitionBody := map[string]interface{}{
		"transition": map[string]string{
			"id": transitionID,
		},
	}

	body, err := json.Marshal(transitionBody)
	if err != nil {
		return fmt.Errorf("failed to marshal transition: %w", err)
	}

	url := fmt.Sprintf("%s/rest/api/3/issue/%s/transitions", j.baseURL, key)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(j.username, j.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to do transition: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(responseBody))
	}

	return nil
}

func (j *JiraTicketSystem) convertFromJiraIssue(ji *jiraIssue) *Ticket {
	ticket := &Ticket{
		ID:     ji.ID,
		Key:    ji.Key,
		Labels: ji.Fields.Labels,
	}

	if ji.Fields.Summary != "" {
		ticket.Summary = ji.Fields.Summary
	}

	if ji.Fields.Description != nil {
		ticket.Description = j.extractDescriptionText(ji.Fields.Description)
		// Extract silence reference from description if present
		ticket.SilenceRef = j.extractSilenceRef(ticket.Description)
	}

	if ji.Fields.Status != nil {
		ticket.Status = j.mapJiraStatus(ji.Fields.Status.Name)
	}

	if ji.Fields.Assignee != nil {
		ticket.Assignee = ji.Fields.Assignee.Name
		if ticket.Assignee == "" {
			ticket.Assignee = ji.Fields.Assignee.AccountID
		}
	}

	if ji.Fields.Created != "" {
		if t, err := time.Parse(time.RFC3339, ji.Fields.Created); err == nil {
			ticket.CreatedAt = t
		}
	}

	if ji.Fields.Updated != "" {
		if t, err := time.Parse(time.RFC3339, ji.Fields.Updated); err == nil {
			ticket.UpdatedAt = t
		}
	}

	return ticket
}

func (j *JiraTicketSystem) convertToJiraIssue(ticket *Ticket) *jiraIssue {
	ji := &jiraIssue{
		Fields: jiraFields{
			Summary: ticket.Summary,
			Labels:  ticket.Labels,
		},
	}

	// Embed silence reference in description if present
	description := ticket.Description
	if ticket.SilenceRef != "" {
		description = fmt.Sprintf("%s: %s\n\n%s", j.annotationPrefix, ticket.SilenceRef, description)
	}

	ji.Fields.Description = j.createJiraDescription(description)

	return ji
}

func (j *JiraTicketSystem) extractDescriptionText(desc *jiraDescription) string {
	var text strings.Builder
	for _, content := range desc.Content {
		for _, para := range content.Content {
			if para.Text != "" {
				if text.Len() > 0 {
					text.WriteString("\n")
				}
				text.WriteString(para.Text)
			}
		}
	}
	return text.String()
}

func (j *JiraTicketSystem) createJiraDescription(text string) *jiraDescription {
	return &jiraDescription{
		Type:    "doc",
		Version: 1,
		Content: []jiraDescriptionContent{
			{
				Type: "paragraph",
				Content: []jiraDescriptionParagraph{
					{
						Type: "text",
						Text: text,
					},
				},
			},
		},
	}
}

func (j *JiraTicketSystem) mapJiraStatus(status string) TicketStatus {
	status = strings.ToLower(status)
	switch {
	case strings.Contains(status, "open"), strings.Contains(status, "to do"):
		return StatusOpen
	case strings.Contains(status, "in progress"), strings.Contains(status, "in review"):
		return StatusInProgress
	case strings.Contains(status, "resolved"), strings.Contains(status, "done"):
		return StatusResolved
	case strings.Contains(status, "closed"):
		return StatusClosed
	case strings.Contains(status, "reopen"):
		return StatusReopened
	default:
		return StatusOpen
	}
}

// extractSilenceRef extracts the silence reference from a description
func (j *JiraTicketSystem) extractSilenceRef(description string) string {
	// Look for pattern "prefix: silence-id"
	prefix := fmt.Sprintf("%s: ", j.annotationPrefix)
	if len(description) < len(prefix) {
		return ""
	}

	if description[:len(prefix)] == prefix {
		// Extract until newline or end of string
		rest := description[len(prefix):]
		for i, c := range rest {
			if c == '\n' {
				return rest[:i]
			}
		}
		return rest
	}

	return ""
}
