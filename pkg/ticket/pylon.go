package ticket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// DefaultPylonAPIURL is the Pylon REST API base URL
const DefaultPylonAPIURL = "https://api.usepylon.com"

// PylonTicketSystem implements the TicketSystem interface for Pylon (usepylon.com).
//
// Pylon issues are identified by their issue number (e.g. "1234"), which is
// used as the ticket key. Pylon only has a single terminal state ("closed"),
// which is reported as StatusResolved so that the synchronizer deletes the
// silence, as it does for Done tickets in Jira.
type PylonTicketSystem struct {
	baseURL          string
	apiToken         string
	accountID        string
	requesterEmail   string
	httpClient       *http.Client
	annotationPrefix string
}

// NewPylonTicketSystem creates a new Pylon ticket system client.
// New issues are created for accountID (required by Pylon unless
// requesterEmail is set). baseURL may be empty to use the public Pylon API.
func NewPylonTicketSystem(baseURL, apiToken, accountID, requesterEmail, annotationPrefix string) *PylonTicketSystem {
	if baseURL == "" {
		baseURL = DefaultPylonAPIURL
	}
	if annotationPrefix == "" {
		annotationPrefix = "silence-manager"
	}
	return &PylonTicketSystem{
		baseURL:          strings.TrimSuffix(baseURL, "/"),
		apiToken:         apiToken,
		accountID:        accountID,
		requesterEmail:   requesterEmail,
		annotationPrefix: annotationPrefix,
		httpClient:       &http.Client{Timeout: 30 * time.Second},
	}
}

// Pylon API structures
type pylonIssue struct {
	ID        string   `json:"id"`
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	BodyHTML  string   `json:"body_html"`
	State     string   `json:"state"`
	CreatedAt string   `json:"created_at"`
	Tags      []string `json:"tags"`
	Assignee  *struct {
		Email string `json:"email"`
	} `json:"assignee"`
}

type pylonIssueResponse struct {
	Data pylonIssue `json:"data"`
}

// do executes a request against the Pylon API. A nil out skips response decoding.
func (p *PylonTicketSystem) do(method, path string, body, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("failed to marshal request: %w", err)
		}
		reader = bytes.NewBuffer(b)
	}

	req, err := http.NewRequest(method, p.baseURL+path, reader)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("pylon request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		respBody, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return resp.StatusCode, nil
}

// GetTicket retrieves a ticket by its issue number
func (p *PylonTicketSystem) GetTicket(key string) (*Ticket, error) {
	var result pylonIssueResponse
	status, err := p.do(http.MethodGet, "/issues/"+url.PathEscape(key), nil, &result)
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("ticket not found: %s", key)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get ticket: %w", err)
	}
	return p.convertFromPylonIssue(&result.Data), nil
}

// CreateTicket creates a new ticket and returns its issue number
func (p *PylonTicketSystem) CreateTicket(ticket *Ticket) (string, error) {
	body := map[string]any{
		"title":     ticket.Summary,
		"body_html": p.buildBodyHTML(ticket),
	}
	if p.accountID != "" {
		body["account_id"] = p.accountID
	}
	if p.requesterEmail != "" {
		body["requester_email"] = p.requesterEmail
	}
	if len(ticket.Labels) > 0 {
		body["tags"] = ticket.Labels
	}

	var result pylonIssueResponse
	if _, err := p.do(http.MethodPost, "/issues", body, &result); err != nil {
		return "", fmt.Errorf("failed to create ticket: %w", err)
	}
	return pylonKey(&result.Data), nil
}

// UpdateTicket updates the title and body of an existing ticket
func (p *PylonTicketSystem) UpdateTicket(ticket *Ticket) error {
	body := map[string]any{
		"title":     ticket.Summary,
		"body_html": p.buildBodyHTML(ticket),
	}
	if _, err := p.do(http.MethodPatch, "/issues/"+url.PathEscape(ticket.Key), body, nil); err != nil {
		return fmt.Errorf("failed to update ticket: %w", err)
	}
	return nil
}

// ReopenTicket reopens a closed ticket by moving it back to the "new" state
func (p *PylonTicketSystem) ReopenTicket(key string, comment string) error {
	if comment != "" {
		if err := p.AddComment(key, comment); err != nil {
			return fmt.Errorf("failed to add comment: %w", err)
		}
	}
	return p.setState(key, "new")
}

// CloseTicket marks a ticket as closed
func (p *PylonTicketSystem) CloseTicket(key string, comment string) error {
	if comment != "" {
		if err := p.AddComment(key, comment); err != nil {
			return fmt.Errorf("failed to add comment: %w", err)
		}
	}
	return p.setState(key, "closed")
}

// AddComment adds an internal note to a ticket (never visible to the customer)
func (p *PylonTicketSystem) AddComment(key string, comment string) error {
	body := map[string]any{"body_html": "<p>" + html.EscapeString(comment) + "</p>"}
	if _, err := p.do(http.MethodPost, "/issues/"+url.PathEscape(key)+"/note", body, nil); err != nil {
		return fmt.Errorf("failed to add comment: %w", err)
	}
	return nil
}

// IsResolved checks if a ticket is in a resolved state (a closed Pylon issue)
func (p *PylonTicketSystem) IsResolved(ticket *Ticket) bool {
	return ticket.Status == StatusResolved
}

// IsClosed checks if a ticket is in a closed state
func (p *PylonTicketSystem) IsClosed(ticket *Ticket) bool {
	return ticket.Status == StatusClosed || ticket.Status == StatusResolved
}

// IsOpen checks if a ticket is in an open state
func (p *PylonTicketSystem) IsOpen(ticket *Ticket) bool {
	return ticket.Status == StatusOpen || ticket.Status == StatusInProgress
}

// Helper functions

func (p *PylonTicketSystem) setState(key, state string) error {
	if _, err := p.do(http.MethodPatch, "/issues/"+url.PathEscape(key), map[string]any{"state": state}, nil); err != nil {
		return fmt.Errorf("failed to set ticket state: %w", err)
	}
	return nil
}

func pylonKey(pi *pylonIssue) string {
	if pi.Number > 0 {
		return fmt.Sprintf("%d", pi.Number)
	}
	return pi.ID
}

func (p *PylonTicketSystem) convertFromPylonIssue(pi *pylonIssue) *Ticket {
	description := pylonHTMLToText(pi.BodyHTML)
	ticket := &Ticket{
		ID:          pi.ID,
		Key:         pylonKey(pi),
		Summary:     pi.Title,
		Description: description,
		Status:      p.mapPylonState(pi.State),
		Labels:      pi.Tags,
		SilenceRef:  extractSilenceRef(description, p.annotationPrefix),
	}
	if pi.Assignee != nil {
		ticket.Assignee = pi.Assignee.Email
	}
	if t, err := time.Parse(time.RFC3339, pi.CreatedAt); err == nil {
		ticket.CreatedAt = t
	}
	return ticket
}

// mapPylonState maps Pylon issue states to TicketStatus. Pylon's standard
// states are new, waiting_on_you, waiting_on_customer, on_hold and closed;
// workspaces may also define custom states, which are treated as open.
func (p *PylonTicketSystem) mapPylonState(state string) TicketStatus {
	switch strings.ToLower(state) {
	case "closed":
		return StatusResolved
	case "waiting_on_you", "waiting_on_customer", "on_hold":
		return StatusInProgress
	default: // new and custom states
		return StatusOpen
	}
}

func (p *PylonTicketSystem) buildBodyHTML(ticket *Ticket) string {
	text := withSilenceRef(ticket.Description, ticket.SilenceRef, p.annotationPrefix)
	var b strings.Builder
	for _, para := range strings.Split(text, "\n\n") {
		if para == "" {
			continue
		}
		b.WriteString("<p>")
		b.WriteString(strings.ReplaceAll(html.EscapeString(para), "\n", "<br>"))
		b.WriteString("</p>")
	}
	return b.String()
}

var (
	pylonBlockBreak = regexp.MustCompile(`(?i)</p>|<br\s*/?>|</div>|</li>`)
	pylonTag        = regexp.MustCompile(`<[^>]*>`)
)

// pylonHTMLToText reduces Pylon's HTML body to plain text, one block per line
func pylonHTMLToText(s string) string {
	s = pylonBlockBreak.ReplaceAllString(s, "\n")
	s = pylonTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(s)
}
