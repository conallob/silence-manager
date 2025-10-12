package sync

import (
	"fmt"
	"testing"
	"time"

	"github.com/conallob/silence-manager/pkg/alertmanager"
	"github.com/conallob/silence-manager/pkg/ticket"
)

// Mock AlertManager implementation
type mockAlertManager struct {
	silences      map[string]*alertmanager.Silence
	alerts        []*alertmanager.Alert
	deletedIDs    []string
	extendedIDs   []string
	createdCount  int
	getSilenceErr error
	listErr       error
	deleteErr     error
	extendErr     error
	createErr     error
	getAlertsErr  error
}

func newMockAlertManager() *mockAlertManager {
	return &mockAlertManager{
		silences:     make(map[string]*alertmanager.Silence),
		alerts:       []*alertmanager.Alert{},
		deletedIDs:   []string{},
		extendedIDs:  []string{},
		createdCount: 0,
	}
}

func (m *mockAlertManager) GetSilence(id string) (*alertmanager.Silence, error) {
	if m.getSilenceErr != nil {
		return nil, m.getSilenceErr
	}
	silence, ok := m.silences[id]
	if !ok {
		return nil, fmt.Errorf("silence not found: %s", id)
	}
	return silence, nil
}

func (m *mockAlertManager) ListSilences() ([]*alertmanager.Silence, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	result := make([]*alertmanager.Silence, 0, len(m.silences))
	for _, s := range m.silences {
		result = append(result, s)
	}
	return result, nil
}

func (m *mockAlertManager) CreateSilence(silence *alertmanager.Silence) (string, error) {
	if m.createErr != nil {
		return "", m.createErr
	}
	id := fmt.Sprintf("silence-%d", m.createdCount)
	m.createdCount++
	silence.ID = id
	m.silences[id] = silence
	return id, nil
}

func (m *mockAlertManager) UpdateSilence(silence *alertmanager.Silence) error {
	m.silences[silence.ID] = silence
	return nil
}

func (m *mockAlertManager) DeleteSilence(id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deletedIDs = append(m.deletedIDs, id)
	delete(m.silences, id)
	return nil
}

func (m *mockAlertManager) ExtendSilence(id string, newEndTime time.Time) error {
	if m.extendErr != nil {
		return m.extendErr
	}
	m.extendedIDs = append(m.extendedIDs, id)
	if silence, ok := m.silences[id]; ok {
		silence.EndsAt = newEndTime
	}
	return nil
}

func (m *mockAlertManager) GetAlerts(matchers []alertmanager.Matcher) ([]*alertmanager.Alert, error) {
	if m.getAlertsErr != nil {
		return nil, m.getAlertsErr
	}
	return m.alerts, nil
}

// Mock TicketSystem implementation
type mockTicketSystem struct {
	tickets        map[string]*ticket.Ticket
	comments       map[string][]string
	reopenedKeys   []string
	closedKeys     []string
	getErr         error
	createErr      error
	updateErr      error
	reopenErr      error
	closeErr       error
	addCommentErr  error
}

func newMockTicketSystem() *mockTicketSystem {
	return &mockTicketSystem{
		tickets:      make(map[string]*ticket.Ticket),
		comments:     make(map[string][]string),
		reopenedKeys: []string{},
		closedKeys:   []string{},
	}
}

func (m *mockTicketSystem) GetTicket(key string) (*ticket.Ticket, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	t, ok := m.tickets[key]
	if !ok {
		return nil, fmt.Errorf("ticket not found: %s", key)
	}
	return t, nil
}

func (m *mockTicketSystem) CreateTicket(t *ticket.Ticket) (string, error) {
	if m.createErr != nil {
		return "", m.createErr
	}
	key := fmt.Sprintf("PROJ-%d", len(m.tickets)+1)
	t.Key = key
	m.tickets[key] = t
	return key, nil
}

func (m *mockTicketSystem) UpdateTicket(t *ticket.Ticket) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.tickets[t.Key] = t
	return nil
}

func (m *mockTicketSystem) ReopenTicket(key string, comment string) error {
	if m.reopenErr != nil {
		return m.reopenErr
	}
	m.reopenedKeys = append(m.reopenedKeys, key)
	if t, ok := m.tickets[key]; ok {
		t.Status = ticket.StatusReopened
	}
	if comment != "" {
		m.comments[key] = append(m.comments[key], comment)
	}
	return nil
}

func (m *mockTicketSystem) CloseTicket(key string, comment string) error {
	if m.closeErr != nil {
		return m.closeErr
	}
	m.closedKeys = append(m.closedKeys, key)
	if t, ok := m.tickets[key]; ok {
		t.Status = ticket.StatusClosed
	}
	if comment != "" {
		m.comments[key] = append(m.comments[key], comment)
	}
	return nil
}

func (m *mockTicketSystem) AddComment(key string, comment string) error {
	if m.addCommentErr != nil {
		return m.addCommentErr
	}
	m.comments[key] = append(m.comments[key], comment)
	return nil
}

func (m *mockTicketSystem) IsResolved(t *ticket.Ticket) bool {
	return t.Status == ticket.StatusResolved
}

func (m *mockTicketSystem) IsClosed(t *ticket.Ticket) bool {
	return t.Status == ticket.StatusClosed || t.Status == ticket.StatusResolved
}

func (m *mockTicketSystem) IsOpen(t *ticket.Ticket) bool {
	return t.Status == ticket.StatusOpen || t.Status == ticket.StatusInProgress
}

// Tests
func TestNewSynchronizer(t *testing.T) {
	am := newMockAlertManager()
	ts := newMockTicketSystem()
	cfg := DefaultConfig()

	sync := NewSynchronizer(am, ts, cfg)

	if sync == nil {
		t.Fatal("Expected non-nil synchronizer")
	}
	if sync.alertManager != am {
		t.Error("AlertManager not set correctly")
	}
	if sync.ticketSystem != ts {
		t.Error("TicketSystem not set correctly")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ExpiryThreshold != 24*time.Hour {
		t.Errorf("Expected expiry threshold 24h, got %v", cfg.ExpiryThreshold)
	}
	if cfg.ExtensionDuration != 7*24*time.Hour {
		t.Errorf("Expected extension duration 7 days, got %v", cfg.ExtensionDuration)
	}
	if cfg.DefaultSilenceDuration != 7*24*time.Hour {
		t.Errorf("Expected default silence duration 7 days, got %v", cfg.DefaultSilenceDuration)
	}
	if !cfg.CheckAlerts {
		t.Error("Expected CheckAlerts to be true")
	}
}

func TestSync_NoSilences(t *testing.T) {
	am := newMockAlertManager()
	ts := newMockTicketSystem()
	cfg := DefaultConfig()

	sync := NewSynchronizer(am, ts, cfg)
	result, err := sync.Sync()

	if err != nil {
		t.Fatalf("Sync() failed: %v", err)
	}
	if result.SilencesExtended != 0 {
		t.Errorf("Expected 0 silences extended, got %d", result.SilencesExtended)
	}
	if result.SilencesDeleted != 0 {
		t.Errorf("Expected 0 silences deleted, got %d", result.SilencesDeleted)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Expected 0 errors, got %d", len(result.Errors))
	}
}

func TestSync_SilenceWithoutTicketRef(t *testing.T) {
	am := newMockAlertManager()
	ts := newMockTicketSystem()
	cfg := DefaultConfig()

	// Add silence without ticket ref
	am.silences["silence-1"] = &alertmanager.Silence{
		ID:        "silence-1",
		CreatedBy: "user",
		Comment:   "No ticket",
		StartsAt:  time.Now(),
		EndsAt:    time.Now().Add(24 * time.Hour),
		TicketRef: "",
	}

	sync := NewSynchronizer(am, ts, cfg)
	result, err := sync.Sync()

	if err != nil {
		t.Fatalf("Sync() failed: %v", err)
	}
	// Should skip silences without ticket ref
	if result.SilencesExtended != 0 || result.SilencesDeleted != 0 {
		t.Error("Expected no action on silence without ticket ref")
	}
}

func TestProcessSilence_ResolvedTicket(t *testing.T) {
	am := newMockAlertManager()
	ts := newMockTicketSystem()
	cfg := DefaultConfig()

	// Add silence with ticket ref
	silence := &alertmanager.Silence{
		ID:        "silence-1",
		CreatedBy: "user",
		Comment:   "Test",
		StartsAt:  time.Now(),
		EndsAt:    time.Now().Add(24 * time.Hour),
		TicketRef: "PROJ-1",
	}
	am.silences["silence-1"] = silence

	// Add resolved ticket
	ts.tickets["PROJ-1"] = &ticket.Ticket{
		Key:    "PROJ-1",
		Status: ticket.StatusResolved,
	}

	sync := NewSynchronizer(am, ts, cfg)
	result, err := sync.Sync()

	if err != nil {
		t.Fatalf("Sync() failed: %v", err)
	}
	if result.SilencesDeleted != 1 {
		t.Errorf("Expected 1 silence deleted, got %d", result.SilencesDeleted)
	}
	if len(am.deletedIDs) != 1 || am.deletedIDs[0] != "silence-1" {
		t.Error("Expected silence-1 to be deleted")
	}
	if len(ts.comments["PROJ-1"]) != 1 {
		t.Errorf("Expected 1 comment on ticket, got %d", len(ts.comments["PROJ-1"]))
	}
}

func TestProcessSilence_OpenTicketExpiringNow(t *testing.T) {
	am := newMockAlertManager()
	ts := newMockTicketSystem()
	cfg := SyncConfig{
		ExpiryThreshold:        24 * time.Hour,
		ExtensionDuration:      7 * 24 * time.Hour,
		DefaultSilenceDuration: 7 * 24 * time.Hour,
		CheckAlerts:            false,
	}

	// Add silence expiring in 12 hours (within threshold)
	silence := &alertmanager.Silence{
		ID:        "silence-1",
		CreatedBy: "user",
		Comment:   "Test",
		StartsAt:  time.Now(),
		EndsAt:    time.Now().Add(12 * time.Hour),
		TicketRef: "PROJ-1",
	}
	am.silences["silence-1"] = silence

	// Add open ticket
	ts.tickets["PROJ-1"] = &ticket.Ticket{
		Key:    "PROJ-1",
		Status: ticket.StatusOpen,
	}

	sync := NewSynchronizer(am, ts, cfg)
	result, err := sync.Sync()

	if err != nil {
		t.Fatalf("Sync() failed: %v", err)
	}
	if result.SilencesExtended != 1 {
		t.Errorf("Expected 1 silence extended, got %d", result.SilencesExtended)
	}
	if len(am.extendedIDs) != 1 || am.extendedIDs[0] != "silence-1" {
		t.Error("Expected silence-1 to be extended")
	}
	if len(ts.comments["PROJ-1"]) != 1 {
		t.Errorf("Expected 1 comment on ticket, got %d", len(ts.comments["PROJ-1"]))
	}
}

func TestProcessSilence_OpenTicketAlreadyExpired(t *testing.T) {
	am := newMockAlertManager()
	ts := newMockTicketSystem()
	cfg := DefaultConfig()

	// Add expired silence
	silence := &alertmanager.Silence{
		ID:        "silence-1",
		CreatedBy: "user",
		Comment:   "Test",
		StartsAt:  time.Now().Add(-48 * time.Hour),
		EndsAt:    time.Now().Add(-1 * time.Hour),
		TicketRef: "PROJ-1",
	}
	am.silences["silence-1"] = silence

	// Add open ticket
	ts.tickets["PROJ-1"] = &ticket.Ticket{
		Key:    "PROJ-1",
		Status: ticket.StatusOpen,
	}

	sync := NewSynchronizer(am, ts, cfg)
	result, err := sync.Sync()

	if err != nil {
		t.Fatalf("Sync() failed: %v", err)
	}
	if result.SilencesExtended != 1 {
		t.Errorf("Expected 1 silence extended, got %d", result.SilencesExtended)
	}
	if len(am.extendedIDs) != 1 {
		t.Error("Expected expired silence to be extended")
	}
}

func TestProcessSilence_OpenTicketNotExpiringSoon(t *testing.T) {
	am := newMockAlertManager()
	ts := newMockTicketSystem()
	cfg := SyncConfig{
		ExpiryThreshold:        24 * time.Hour,
		ExtensionDuration:      7 * 24 * time.Hour,
		DefaultSilenceDuration: 7 * 24 * time.Hour,
		CheckAlerts:            false,
	}

	// Add silence expiring in 48 hours (outside threshold)
	silence := &alertmanager.Silence{
		ID:        "silence-1",
		CreatedBy: "user",
		Comment:   "Test",
		StartsAt:  time.Now(),
		EndsAt:    time.Now().Add(48 * time.Hour),
		TicketRef: "PROJ-1",
	}
	am.silences["silence-1"] = silence

	// Add open ticket
	ts.tickets["PROJ-1"] = &ticket.Ticket{
		Key:    "PROJ-1",
		Status: ticket.StatusOpen,
	}

	sync := NewSynchronizer(am, ts, cfg)
	result, err := sync.Sync()

	if err != nil {
		t.Fatalf("Sync() failed: %v", err)
	}
	// Should not extend since it's not expiring soon
	if result.SilencesExtended != 0 {
		t.Errorf("Expected 0 silences extended, got %d", result.SilencesExtended)
	}
}

func TestProcessSilence_TicketNotFound(t *testing.T) {
	am := newMockAlertManager()
	ts := newMockTicketSystem()
	cfg := DefaultConfig()

	// Add silence with ticket ref
	silence := &alertmanager.Silence{
		ID:        "silence-1",
		CreatedBy: "user",
		Comment:   "Test",
		StartsAt:  time.Now(),
		EndsAt:    time.Now().Add(24 * time.Hour),
		TicketRef: "PROJ-999",
	}
	am.silences["silence-1"] = silence

	sync := NewSynchronizer(am, ts, cfg)
	result, err := sync.Sync()

	if err != nil {
		t.Fatalf("Sync() failed: %v", err)
	}
	// Should record error for missing ticket
	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(result.Errors))
	}
}

func TestCheckRefiredAlerts_NoAlerts(t *testing.T) {
	am := newMockAlertManager()
	ts := newMockTicketSystem()
	cfg := SyncConfig{
		ExpiryThreshold:        24 * time.Hour,
		ExtensionDuration:      7 * 24 * time.Hour,
		DefaultSilenceDuration: 7 * 24 * time.Hour,
		CheckAlerts:            true,
	}

	sync := NewSynchronizer(am, ts, cfg)
	result, err := sync.Sync()

	if err != nil {
		t.Fatalf("Sync() failed: %v", err)
	}
	if result.TicketsReopened != 0 {
		t.Errorf("Expected 0 tickets reopened, got %d", result.TicketsReopened)
	}
}

func TestCheckRefiredAlerts_AlertWithoutTicket(t *testing.T) {
	am := newMockAlertManager()
	ts := newMockTicketSystem()
	cfg := SyncConfig{
		ExpiryThreshold:        24 * time.Hour,
		ExtensionDuration:      7 * 24 * time.Hour,
		DefaultSilenceDuration: 7 * 24 * time.Hour,
		CheckAlerts:            true,
	}

	// Add alert without ticket label
	am.alerts = []*alertmanager.Alert{
		{
			Labels: map[string]string{
				"alertname": "TestAlert",
			},
		},
	}

	sync := NewSynchronizer(am, ts, cfg)
	result, err := sync.Sync()

	if err != nil {
		t.Fatalf("Sync() failed: %v", err)
	}
	// Should not reopen anything
	if result.TicketsReopened != 0 {
		t.Errorf("Expected 0 tickets reopened, got %d", result.TicketsReopened)
	}
}

func TestCheckRefiredAlerts_ReopenClosedTicket(t *testing.T) {
	am := newMockAlertManager()
	ts := newMockTicketSystem()
	cfg := SyncConfig{
		ExpiryThreshold:        24 * time.Hour,
		ExtensionDuration:      7 * 24 * time.Hour,
		DefaultSilenceDuration: 7 * 24 * time.Hour,
		CheckAlerts:            true,
	}

	// Add alert with ticket label
	am.alerts = []*alertmanager.Alert{
		{
			Labels: map[string]string{
				"alertname": "TestAlert",
				"ticket":    "PROJ-1",
			},
		},
	}

	// Add closed ticket
	ts.tickets["PROJ-1"] = &ticket.Ticket{
		Key:    "PROJ-1",
		Status: ticket.StatusClosed,
	}

	sync := NewSynchronizer(am, ts, cfg)
	result, err := sync.Sync()

	if err != nil {
		t.Fatalf("Sync() failed: %v", err)
	}
	if result.TicketsReopened != 1 {
		t.Errorf("Expected 1 ticket reopened, got %d", result.TicketsReopened)
	}
	if result.SilencesCreated != 1 {
		t.Errorf("Expected 1 silence created, got %d", result.SilencesCreated)
	}
	if len(ts.reopenedKeys) != 1 || ts.reopenedKeys[0] != "PROJ-1" {
		t.Error("Expected PROJ-1 to be reopened")
	}
}

func TestCheckRefiredAlerts_OpenTicketWithRefiredAlert(t *testing.T) {
	am := newMockAlertManager()
	ts := newMockTicketSystem()
	cfg := SyncConfig{
		ExpiryThreshold:        24 * time.Hour,
		ExtensionDuration:      7 * 24 * time.Hour,
		DefaultSilenceDuration: 7 * 24 * time.Hour,
		CheckAlerts:            true,
	}

	// Add alert with ticket label
	am.alerts = []*alertmanager.Alert{
		{
			Labels: map[string]string{
				"alertname": "TestAlert",
				"ticket":    "PROJ-1",
			},
		},
	}

	// Add open ticket (should not reopen)
	ts.tickets["PROJ-1"] = &ticket.Ticket{
		Key:    "PROJ-1",
		Status: ticket.StatusOpen,
	}

	sync := NewSynchronizer(am, ts, cfg)
	result, err := sync.Sync()

	if err != nil {
		t.Fatalf("Sync() failed: %v", err)
	}
	// Should not reopen since ticket is already open
	if result.TicketsReopened != 0 {
		t.Errorf("Expected 0 tickets reopened, got %d", result.TicketsReopened)
	}
}

func TestCheckRefiredAlerts_ClosedTicketWithActiveSilence(t *testing.T) {
	am := newMockAlertManager()
	ts := newMockTicketSystem()
	cfg := SyncConfig{
		ExpiryThreshold:        24 * time.Hour,
		ExtensionDuration:      7 * 24 * time.Hour,
		DefaultSilenceDuration: 7 * 24 * time.Hour,
		CheckAlerts:            true,
	}

	// Add active silence
	am.silences["silence-1"] = &alertmanager.Silence{
		ID:        "silence-1",
		CreatedBy: "user",
		Comment:   "Test",
		StartsAt:  time.Now(),
		EndsAt:    time.Now().Add(24 * time.Hour),
		TicketRef: "PROJ-1",
	}

	// Add alert with ticket and silence labels
	am.alerts = []*alertmanager.Alert{
		{
			Labels: map[string]string{
				"alertname":  "TestAlert",
				"ticket":     "PROJ-1",
				"silence_id": "silence-1",
			},
		},
	}

	// Add closed ticket
	ts.tickets["PROJ-1"] = &ticket.Ticket{
		Key:    "PROJ-1",
		Status: ticket.StatusClosed,
	}

	sync := NewSynchronizer(am, ts, cfg)
	result, err := sync.Sync()

	if err != nil {
		t.Fatalf("Sync() failed: %v", err)
	}
	// Should not reopen since there's an active silence
	if result.TicketsReopened != 0 {
		t.Errorf("Expected 0 tickets reopened (has active silence), got %d", result.TicketsReopened)
	}
}

func TestCreateMatchersFromAlert(t *testing.T) {
	am := newMockAlertManager()
	ts := newMockTicketSystem()
	cfg := DefaultConfig()

	sync := NewSynchronizer(am, ts, cfg)

	alert := &alertmanager.Alert{
		Labels: map[string]string{
			"alertname": "TestAlert",
			"job":       "test-job",
			"instance":  "server1",
			"severity":  "critical",
			"other":     "ignored",
		},
	}

	matchers := sync.createMatchersFromAlert(alert)

	// Should create matchers for important labels only
	if len(matchers) != 4 {
		t.Errorf("Expected 4 matchers, got %d", len(matchers))
	}

	// Verify matchers are created for important labels
	foundLabels := make(map[string]bool)
	for _, m := range matchers {
		foundLabels[m.Name] = true
		if !m.IsEqual {
			t.Errorf("Expected IsEqual to be true for matcher %s", m.Name)
		}
		if m.IsRegex {
			t.Errorf("Expected IsRegex to be false for matcher %s", m.Name)
		}
	}

	expectedLabels := []string{"alertname", "job", "instance", "severity"}
	for _, label := range expectedLabels {
		if !foundLabels[label] {
			t.Errorf("Expected matcher for label %s", label)
		}
	}
}

func TestSync_ListSilencesError(t *testing.T) {
	am := newMockAlertManager()
	ts := newMockTicketSystem()
	cfg := DefaultConfig()

	am.listErr = fmt.Errorf("failed to list silences")

	sync := NewSynchronizer(am, ts, cfg)
	_, err := sync.Sync()

	if err == nil {
		t.Error("Expected error when ListSilences fails")
	}
}

func TestSync_DeleteSilenceError(t *testing.T) {
	am := newMockAlertManager()
	ts := newMockTicketSystem()
	cfg := DefaultConfig()

	am.deleteErr = fmt.Errorf("failed to delete")

	// Add silence with resolved ticket
	silence := &alertmanager.Silence{
		ID:        "silence-1",
		CreatedBy: "user",
		Comment:   "Test",
		StartsAt:  time.Now(),
		EndsAt:    time.Now().Add(24 * time.Hour),
		TicketRef: "PROJ-1",
	}
	am.silences["silence-1"] = silence

	ts.tickets["PROJ-1"] = &ticket.Ticket{
		Key:    "PROJ-1",
		Status: ticket.StatusResolved,
	}

	sync := NewSynchronizer(am, ts, cfg)
	result, err := sync.Sync()

	if err != nil {
		t.Fatalf("Sync() should not fail: %v", err)
	}
	// Should record error for failed delete
	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(result.Errors))
	}
}

func TestSync_ExtendSilenceError(t *testing.T) {
	am := newMockAlertManager()
	ts := newMockTicketSystem()
	cfg := DefaultConfig()

	am.extendErr = fmt.Errorf("failed to extend")

	// Add expiring silence
	silence := &alertmanager.Silence{
		ID:        "silence-1",
		CreatedBy: "user",
		Comment:   "Test",
		StartsAt:  time.Now(),
		EndsAt:    time.Now().Add(12 * time.Hour),
		TicketRef: "PROJ-1",
	}
	am.silences["silence-1"] = silence

	ts.tickets["PROJ-1"] = &ticket.Ticket{
		Key:    "PROJ-1",
		Status: ticket.StatusOpen,
	}

	sync := NewSynchronizer(am, ts, cfg)
	result, err := sync.Sync()

	if err != nil {
		t.Fatalf("Sync() should not fail: %v", err)
	}
	// Should record error for failed extend
	if len(result.Errors) != 1 {
		t.Errorf("Expected 1 error, got %d", len(result.Errors))
	}
}
