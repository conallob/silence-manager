package alertmanager

import "time"

// Silence represents a silence in an alertmanager system
type Silence struct {
	ID          string
	CreatedBy   string
	Comment     string
	StartsAt    time.Time
	EndsAt      time.Time
	Matchers    []Matcher
	TicketRef   string // Reference to the associated ticket
}

// Matcher represents an alert matcher for a silence
type Matcher struct {
	Name    string
	Value   string
	IsRegex bool
	IsEqual bool // true for =, false for !=
}

// Alert represents an alert that has fired
type Alert struct {
	Labels      map[string]string
	Annotations map[string]string
	StartsAt    time.Time
	EndsAt      time.Time
	Status      string
}

// AlertManager is the interface that all alertmanager implementations must satisfy
type AlertManager interface {
	// GetSilence retrieves a silence by ID
	GetSilence(id string) (*Silence, error)

	// ListSilences returns all active silences
	ListSilences() ([]*Silence, error)

	// CreateSilence creates a new silence and returns its ID
	CreateSilence(silence *Silence) (string, error)

	// UpdateSilence updates an existing silence
	UpdateSilence(silence *Silence) error

	// DeleteSilence deletes a silence by ID
	DeleteSilence(id string) error

	// ExtendSilence extends the end time of a silence
	ExtendSilence(id string, newEndTime time.Time) error

	// GetAlerts returns all active alerts matching the given matchers
	GetAlerts(matchers []Matcher) ([]*Alert, error)
}
