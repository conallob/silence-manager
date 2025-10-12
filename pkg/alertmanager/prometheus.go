package alertmanager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// PrometheusAlertManager implements the AlertManager interface for Prometheus Alertmanager
type PrometheusAlertManager struct {
	baseURL          string
	authType         string
	username         string
	password         string
	bearerToken      string
	httpClient       *http.Client
	annotationPrefix string
}

// AlertManagerConfig holds configuration for creating a new Alertmanager client
type AlertManagerConfig struct {
	BaseURL          string
	AuthType         string // "none", "basic", "bearer"
	Username         string
	Password         string
	BearerToken      string
	AnnotationPrefix string
}

// NewPrometheusAlertManager creates a new Prometheus Alertmanager client
func NewPrometheusAlertManager(baseURL string) *PrometheusAlertManager {
	return NewPrometheusAlertManagerWithConfig(AlertManagerConfig{
		BaseURL:          baseURL,
		AuthType:         "none",
		AnnotationPrefix: "silence-manager",
	})
}

// NewPrometheusAlertManagerWithConfig creates a new Prometheus Alertmanager client with configuration
func NewPrometheusAlertManagerWithConfig(config AlertManagerConfig) *PrometheusAlertManager {
	prefix := config.AnnotationPrefix
	if prefix == "" {
		prefix = "silence-manager"
	}
	return &PrometheusAlertManager{
		baseURL:          config.BaseURL,
		authType:         config.AuthType,
		username:         config.Username,
		password:         config.Password,
		bearerToken:      config.BearerToken,
		annotationPrefix: prefix,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// addAuth adds authentication headers to the HTTP request
func (p *PrometheusAlertManager) addAuth(req *http.Request) {
	switch p.authType {
	case "basic":
		req.SetBasicAuth(p.username, p.password)
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+p.bearerToken)
	case "none":
		// No authentication
	}
}

// API response structures for Prometheus Alertmanager
type promSilence struct {
	ID        string         `json:"id,omitempty"`
	Status    *silenceStatus `json:"status,omitempty"`
	Comment   string         `json:"comment"`
	CreatedBy string         `json:"createdBy"`
	StartsAt  time.Time      `json:"startsAt"`
	EndsAt    time.Time      `json:"endsAt"`
	Matchers  []promMatcher  `json:"matchers"`
}

type silenceStatus struct {
	State string `json:"state"`
}

type promMatcher struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
	IsEqual bool   `json:"isEqual"`
}

type promAlert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
	Status      struct {
		State string `json:"state"`
	} `json:"status"`
}

// GetSilence retrieves a silence by ID
func (p *PrometheusAlertManager) GetSilence(id string) (*Silence, error) {
	url := fmt.Sprintf("%s/api/v2/silence/%s", p.baseURL, id)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.addAuth(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get silence: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("silence not found: %s", id)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var ps promSilence
	if err := json.NewDecoder(resp.Body).Decode(&ps); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return p.convertFromPromSilence(&ps), nil
}

// ListSilences returns all active silences
func (p *PrometheusAlertManager) ListSilences() ([]*Silence, error) {
	url := fmt.Sprintf("%s/api/v2/silences", p.baseURL)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.addAuth(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list silences: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var psList []promSilence
	if err := json.NewDecoder(resp.Body).Decode(&psList); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	silences := make([]*Silence, 0, len(psList))
	for i := range psList {
		// Only include active or pending silences
		if psList[i].Status != nil &&
			(psList[i].Status.State == "active" || psList[i].Status.State == "pending") {
			silences = append(silences, p.convertFromPromSilence(&psList[i]))
		}
	}

	return silences, nil
}

// CreateSilence creates a new silence and returns its ID
func (p *PrometheusAlertManager) CreateSilence(silence *Silence) (string, error) {
	ps := p.convertToPromSilence(silence)

	body, err := json.Marshal(ps)
	if err != nil {
		return "", fmt.Errorf("failed to marshal silence: %w", err)
	}

	url := fmt.Sprintf("%s/api/v2/silences", p.baseURL)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	p.addAuth(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to create silence: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(responseBody))
	}

	var result struct {
		SilenceID string `json:"silenceID"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return result.SilenceID, nil
}

// UpdateSilence updates an existing silence
func (p *PrometheusAlertManager) UpdateSilence(silence *Silence) error {
	// In Alertmanager, updating a silence requires deleting and recreating it
	// However, we can reuse the same ID by including it in the POST
	ps := p.convertToPromSilence(silence)
	ps.ID = silence.ID

	body, err := json.Marshal(ps)
	if err != nil {
		return fmt.Errorf("failed to marshal silence: %w", err)
	}

	url := fmt.Sprintf("%s/api/v2/silences", p.baseURL)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	p.addAuth(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update silence: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(responseBody))
	}

	return nil
}

// DeleteSilence deletes a silence by ID
func (p *PrometheusAlertManager) DeleteSilence(id string) error {
	url := fmt.Sprintf("%s/api/v2/silence/%s", p.baseURL, id)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}
	p.addAuth(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete silence: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ExtendSilence extends the end time of a silence
func (p *PrometheusAlertManager) ExtendSilence(id string, newEndTime time.Time) error {
	silence, err := p.GetSilence(id)
	if err != nil {
		return fmt.Errorf("failed to get silence for extension: %w", err)
	}

	silence.EndsAt = newEndTime
	return p.UpdateSilence(silence)
}

// GetAlerts returns all active alerts matching the given matchers
func (p *PrometheusAlertManager) GetAlerts(matchers []Matcher) ([]*Alert, error) {
	url := fmt.Sprintf("%s/api/v2/alerts", p.baseURL)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.addAuth(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get alerts: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var paList []promAlert
	if err := json.NewDecoder(resp.Body).Decode(&paList); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	alerts := make([]*Alert, 0)
	for i := range paList {
		// Only include firing alerts
		if paList[i].Status.State == "active" {
			alert := p.convertFromPromAlert(&paList[i])
			if p.matchesMatchers(alert, matchers) {
				alerts = append(alerts, alert)
			}
		}
	}

	return alerts, nil
}

// Helper functions for conversion
func (p *PrometheusAlertManager) convertFromPromSilence(ps *promSilence) *Silence {
	matchers := make([]Matcher, len(ps.Matchers))
	for i, m := range ps.Matchers {
		matchers[i] = Matcher{
			Name:    m.Name,
			Value:   m.Value,
			IsRegex: m.IsRegex,
			IsEqual: m.IsEqual,
		}
	}

	// Extract ticket reference from comment if it follows the pattern "# prefix: TICKET-123"
	ticketRef := p.extractTicketRef(ps.Comment)

	return &Silence{
		ID:        ps.ID,
		CreatedBy: ps.CreatedBy,
		Comment:   ps.Comment,
		StartsAt:  ps.StartsAt,
		EndsAt:    ps.EndsAt,
		Matchers:  matchers,
		TicketRef: ticketRef,
	}
}

func (p *PrometheusAlertManager) convertToPromSilence(s *Silence) *promSilence {
	matchers := make([]promMatcher, len(s.Matchers))
	for i, m := range s.Matchers {
		matchers[i] = promMatcher{
			Name:    m.Name,
			Value:   m.Value,
			IsRegex: m.IsRegex,
			IsEqual: m.IsEqual,
		}
	}

	// Embed ticket reference in comment if present
	comment := s.Comment
	if s.TicketRef != "" {
		comment = fmt.Sprintf("# %s: %s\n%s", p.annotationPrefix, s.TicketRef, comment)
	}

	return &promSilence{
		ID:        s.ID,
		CreatedBy: s.CreatedBy,
		Comment:   comment,
		StartsAt:  s.StartsAt,
		EndsAt:    s.EndsAt,
		Matchers:  matchers,
	}
}

func (p *PrometheusAlertManager) convertFromPromAlert(pa *promAlert) *Alert {
	return &Alert{
		Labels:      pa.Labels,
		Annotations: pa.Annotations,
		StartsAt:    pa.StartsAt,
		EndsAt:      pa.EndsAt,
		Status:      pa.Status.State,
	}
}

func (p *PrometheusAlertManager) matchesMatchers(alert *Alert, matchers []Matcher) bool {
	if len(matchers) == 0 {
		return true
	}

	for _, matcher := range matchers {
		labelValue, exists := alert.Labels[matcher.Name]

		if matcher.IsRegex {
			// For simplicity, we'll do basic string matching here
			// In production, you'd want to compile and cache regex patterns
			matched := labelValue == matcher.Value
			if matcher.IsEqual != matched {
				return false
			}
		} else {
			if matcher.IsEqual {
				if !exists || labelValue != matcher.Value {
					return false
				}
			} else {
				if exists && labelValue == matcher.Value {
					return false
				}
			}
		}
	}

	return true
}

// extractTicketRef extracts the ticket reference from a comment
func (p *PrometheusAlertManager) extractTicketRef(comment string) string {
	// Look for pattern "# prefix: TICKET-123"
	prefix := fmt.Sprintf("# %s: ", p.annotationPrefix)
	if len(comment) < len(prefix) {
		return ""
	}

	if comment[:len(prefix)] == prefix {
		// Extract until newline or end of string
		rest := comment[len(prefix):]
		for i, c := range rest {
			if c == '\n' {
				return rest[:i]
			}
		}
		return rest
	}

	return ""
}
