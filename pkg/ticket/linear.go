package ticket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// DefaultLinearAPIURL is the Linear GraphQL endpoint
const DefaultLinearAPIURL = "https://api.linear.app/graphql"

// LinearTicketSystem implements the TicketSystem interface for Linear (linear.app)
type LinearTicketSystem struct {
	apiURL           string
	apiKey           string
	teamID           string
	httpClient       *http.Client
	annotationPrefix string
}

// NewLinearTicketSystem creates a new Linear ticket system client.
// apiKey is a Linear personal API key (sent as-is) or an OAuth token prefixed
// with "Bearer ". teamID is the UUID of the team new issues are created in.
// apiURL may be empty to use the public Linear API.
func NewLinearTicketSystem(apiURL, apiKey, teamID, annotationPrefix string) *LinearTicketSystem {
	if apiURL == "" {
		apiURL = DefaultLinearAPIURL
	}
	if annotationPrefix == "" {
		annotationPrefix = "silence-manager"
	}
	return &LinearTicketSystem{
		apiURL:           apiURL,
		apiKey:           apiKey,
		teamID:           teamID,
		annotationPrefix: annotationPrefix,
		httpClient:       &http.Client{Timeout: 30 * time.Second},
	}
}

// Linear GraphQL structures
type linearGraphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type linearGraphQLError struct {
	Message string `json:"message"`
}

type linearState struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"` // triage, backlog, unstarted, started, completed, canceled
	Position float64 `json:"position"`
}

type linearIssue struct {
	ID          string       `json:"id"`
	Identifier  string       `json:"identifier"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	CreatedAt   string       `json:"createdAt"`
	UpdatedAt   string       `json:"updatedAt"`
	State       *linearState `json:"state"`
	Assignee    *struct {
		Name string `json:"name"`
	} `json:"assignee"`
	Labels *struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
	Team *struct {
		ID     string `json:"id"`
		States *struct {
			Nodes []linearState `json:"nodes"`
		} `json:"states"`
	} `json:"team"`
}

const linearIssueFields = `
	id identifier title description createdAt updatedAt
	state { id name type position }
	assignee { name }
	labels { nodes { name } }
`

// do executes a GraphQL request and decodes the "data" object into out
func (l *LinearTicketSystem) do(query string, vars map[string]any, out any) error {
	body, err := json.Marshal(linearGraphQLRequest{Query: query, Variables: vars})
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, l.apiURL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", l.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("linear request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Linear reports GraphQL errors with a non-200 status as well, so try to
	// decode the error list before giving up on the status code.
	var envelope struct {
		Data   json.RawMessage      `json:"data"`
		Errors []linearGraphQLError `json:"errors"`
	}
	decodeErr := json.Unmarshal(respBody, &envelope)

	if len(envelope.Errors) > 0 {
		msgs := make([]string, 0, len(envelope.Errors))
		for _, e := range envelope.Errors {
			msgs = append(msgs, e.Message)
		}
		return fmt.Errorf("linear API error: %s", strings.Join(msgs, "; "))
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}
	if decodeErr != nil {
		return fmt.Errorf("failed to decode response: %w", decodeErr)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return fmt.Errorf("failed to decode response data: %w", err)
	}
	return nil
}

// GetTicket retrieves a ticket by its identifier (e.g. ENG-123)
func (l *LinearTicketSystem) GetTicket(key string) (*Ticket, error) {
	var data struct {
		Issue *linearIssue `json:"issue"`
	}
	query := `query($id: String!) { issue(id: $id) {` + linearIssueFields + `} }`
	if err := l.do(query, map[string]any{"id": key}, &data); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			return nil, fmt.Errorf("ticket not found: %s", key)
		}
		return nil, fmt.Errorf("failed to get ticket: %w", err)
	}
	if data.Issue == nil {
		return nil, fmt.Errorf("ticket not found: %s", key)
	}
	return l.convertFromLinearIssue(data.Issue), nil
}

// CreateTicket creates a new ticket and returns its identifier
func (l *LinearTicketSystem) CreateTicket(ticket *Ticket) (string, error) {
	input := map[string]any{
		"teamId":      l.teamID,
		"title":       ticket.Summary,
		"description": l.buildDescription(ticket),
	}

	var data struct {
		IssueCreate struct {
			Success bool         `json:"success"`
			Issue   *linearIssue `json:"issue"`
		} `json:"issueCreate"`
	}
	query := `mutation($input: IssueCreateInput!) { issueCreate(input: $input) { success issue { id identifier } } }`
	if err := l.do(query, map[string]any{"input": input}, &data); err != nil {
		return "", fmt.Errorf("failed to create ticket: %w", err)
	}
	if !data.IssueCreate.Success || data.IssueCreate.Issue == nil {
		return "", fmt.Errorf("failed to create ticket: linear reported failure")
	}
	return data.IssueCreate.Issue.Identifier, nil
}

// UpdateTicket updates the title and description of an existing ticket
func (l *LinearTicketSystem) UpdateTicket(ticket *Ticket) error {
	input := map[string]any{
		"title":       ticket.Summary,
		"description": l.buildDescription(ticket),
	}
	return l.updateIssue(ticket.Key, input)
}

// ReopenTicket reopens a closed/resolved ticket. The state is changed before
// the comment is posted so that a failed state change is not left with a
// comment that would be posted again on the next run.
func (l *LinearTicketSystem) ReopenTicket(key string, comment string) error {
	state, err := l.findState(key, "unstarted", "backlog")
	if err != nil {
		return err
	}
	if err := l.updateIssue(key, map[string]any{"stateId": state.ID}); err != nil {
		return err
	}
	l.commentBestEffort(key, comment)
	return nil
}

// CloseTicket marks a ticket as closed
func (l *LinearTicketSystem) CloseTicket(key string, comment string) error {
	state, err := l.findState(key, "completed", "canceled")
	if err != nil {
		return err
	}
	if err := l.updateIssue(key, map[string]any{"stateId": state.ID}); err != nil {
		return err
	}
	l.commentBestEffort(key, comment)
	return nil
}

// commentBestEffort posts a comment after a successful state change; a failure
// is logged rather than returned because the state change cannot be undone.
func (l *LinearTicketSystem) commentBestEffort(key, comment string) {
	if comment == "" {
		return
	}
	if err := l.AddComment(key, comment); err != nil {
		log.Printf("Warning: failed to add comment to Linear ticket %s: %v", key, err)
	}
}

// AddComment adds a comment to a ticket
func (l *LinearTicketSystem) AddComment(key string, comment string) error {
	// commentCreate needs the issue UUID or identifier; Linear accepts both.
	var data struct {
		CommentCreate struct {
			Success bool `json:"success"`
		} `json:"commentCreate"`
	}
	query := `mutation($input: CommentCreateInput!) { commentCreate(input: $input) { success } }`
	input := map[string]any{"issueId": key, "body": comment}
	if err := l.do(query, map[string]any{"input": input}, &data); err != nil {
		return fmt.Errorf("failed to add comment: %w", err)
	}
	if !data.CommentCreate.Success {
		return fmt.Errorf("failed to add comment: linear reported failure")
	}
	return nil
}

// IsResolved checks if a ticket is in a resolved state
func (l *LinearTicketSystem) IsResolved(ticket *Ticket) bool {
	return ticket.Status == StatusResolved
}

// IsClosed checks if a ticket is in a closed state
func (l *LinearTicketSystem) IsClosed(ticket *Ticket) bool {
	return ticket.Status == StatusClosed || ticket.Status == StatusResolved
}

// IsOpen checks if a ticket is in an open state
func (l *LinearTicketSystem) IsOpen(ticket *Ticket) bool {
	return ticket.Status == StatusOpen || ticket.Status == StatusInProgress
}

// Helper functions

func (l *LinearTicketSystem) updateIssue(key string, input map[string]any) error {
	var data struct {
		IssueUpdate struct {
			Success bool `json:"success"`
		} `json:"issueUpdate"`
	}
	query := `mutation($id: String!, $input: IssueUpdateInput!) { issueUpdate(id: $id, input: $input) { success } }`
	if err := l.do(query, map[string]any{"id": key, "input": input}, &data); err != nil {
		return fmt.Errorf("failed to update ticket: %w", err)
	}
	if !data.IssueUpdate.Success {
		return fmt.Errorf("failed to update ticket %s: linear reported failure", key)
	}
	return nil
}

// findState looks up a workflow state in the issue's team, trying each state
// type in order of preference and picking the lowest-positioned match.
func (l *LinearTicketSystem) findState(key string, types ...string) (*linearState, error) {
	var data struct {
		Issue *linearIssue `json:"issue"`
	}
	query := `query($id: String!) { issue(id: $id) { team { id states { nodes { id name type position } } } } }`
	if err := l.do(query, map[string]any{"id": key}, &data); err != nil {
		return nil, fmt.Errorf("failed to get workflow states: %w", err)
	}
	if data.Issue == nil || data.Issue.Team == nil || data.Issue.Team.States == nil {
		return nil, fmt.Errorf("no workflow states found for ticket %s", key)
	}

	for _, typ := range types {
		var best *linearState
		nodes := data.Issue.Team.States.Nodes
		for i := range nodes {
			if nodes[i].Type != typ {
				continue
			}
			if best == nil || nodes[i].Position < best.Position {
				best = &nodes[i]
			}
		}
		if best != nil {
			return best, nil
		}
	}
	return nil, fmt.Errorf("no workflow state of type %s found for ticket %s", strings.Join(types, " or "), key)
}

func (l *LinearTicketSystem) convertFromLinearIssue(li *linearIssue) *Ticket {
	ticket := &Ticket{
		ID:          li.ID,
		Key:         li.Identifier,
		Summary:     li.Title,
		Description: li.Description,
		SilenceRef:  extractSilenceRef(li.Description, l.annotationPrefix),
	}
	if li.State != nil {
		ticket.Status = l.mapLinearStateType(li.State.Type)
	}
	if li.Assignee != nil {
		ticket.Assignee = li.Assignee.Name
	}
	if li.Labels != nil {
		for _, label := range li.Labels.Nodes {
			ticket.Labels = append(ticket.Labels, label.Name)
		}
	}
	if t, err := time.Parse(time.RFC3339, li.CreatedAt); err == nil {
		ticket.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, li.UpdatedAt); err == nil {
		ticket.UpdatedAt = t
	}
	return ticket
}

// mapLinearStateType maps Linear's fixed workflow state types to TicketStatus
func (l *LinearTicketSystem) mapLinearStateType(stateType string) TicketStatus {
	switch stateType {
	case "started":
		return StatusInProgress
	case "completed":
		return StatusResolved
	case "canceled":
		return StatusClosed
	default: // triage, backlog, unstarted
		return StatusOpen
	}
}

func (l *LinearTicketSystem) buildDescription(ticket *Ticket) string {
	return withSilenceRef(ticket.Description, ticket.SilenceRef, l.annotationPrefix)
}
