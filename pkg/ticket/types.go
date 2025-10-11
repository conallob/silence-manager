package ticket

import "time"

// TicketStatus represents the status of a ticket
type TicketStatus string

const (
	StatusOpen       TicketStatus = "open"
	StatusInProgress TicketStatus = "in_progress"
	StatusResolved   TicketStatus = "resolved"
	StatusClosed     TicketStatus = "closed"
	StatusReopened   TicketStatus = "reopened"
)

// Ticket represents a ticket in a ticket tracking system
type Ticket struct {
	ID          string
	Key         string // Human-readable key (e.g., PROJ-123)
	Summary     string
	Description string
	Status      TicketStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
	SilenceRef  string // Reference to the associated silence ID
	Labels      []string
	Assignee    string
}

// TicketSystem is the interface that all ticket system implementations must satisfy
type TicketSystem interface {
	// GetTicket retrieves a ticket by its key
	GetTicket(key string) (*Ticket, error)

	// CreateTicket creates a new ticket and returns its key
	CreateTicket(ticket *Ticket) (string, error)

	// UpdateTicket updates an existing ticket
	UpdateTicket(ticket *Ticket) error

	// ReopenTicket reopens a closed/resolved ticket
	ReopenTicket(key string, comment string) error

	// CloseTicket marks a ticket as closed
	CloseTicket(key string, comment string) error

	// AddComment adds a comment to a ticket
	AddComment(key string, comment string) error

	// IsResolved checks if a ticket is in a resolved state
	IsResolved(ticket *Ticket) bool

	// IsClosed checks if a ticket is in a closed state
	IsClosed(ticket *Ticket) bool

	// IsOpen checks if a ticket is in an open state (open or in progress)
	IsOpen(ticket *Ticket) bool
}
