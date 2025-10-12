package sync

import (
	"fmt"
	"log"
	"time"

	"github.com/conallob/silence-manager/pkg/alertmanager"
	"github.com/conallob/silence-manager/pkg/metrics"
	"github.com/conallob/silence-manager/pkg/ticket"
)

// SyncConfig holds configuration for the synchronization process
type SyncConfig struct {
	// ExpiryThreshold is the duration before expiry when we should extend a silence
	ExpiryThreshold time.Duration
	// ExtensionDuration is how long to extend a silence when it's about to expire
	ExtensionDuration time.Duration
	// DefaultSilenceDuration is the default duration for new silences
	DefaultSilenceDuration time.Duration
	// CheckAlerts determines whether to check for refired alerts
	CheckAlerts bool
}

// Synchronizer handles synchronization between alertmanager and ticket system
type Synchronizer struct {
	alertManager     alertmanager.AlertManager
	ticketSystem     ticket.TicketSystem
	config           SyncConfig
	metricsPublisher metrics.Publisher
}

// NewSynchronizer creates a new synchronizer
func NewSynchronizer(am alertmanager.AlertManager, ts ticket.TicketSystem, config SyncConfig) *Synchronizer {
	return &Synchronizer{
		alertManager:     am,
		ticketSystem:     ts,
		config:           config,
		metricsPublisher: metrics.NewNoopPublisher(), // Default to no-op
	}
}

// SetMetricsPublisher sets the metrics publisher for the synchronizer
func (s *Synchronizer) SetMetricsPublisher(publisher metrics.Publisher) {
	s.metricsPublisher = publisher
}

// SyncResult contains the results of a synchronization run
type SyncResult struct {
	SilencesExtended int
	SilencesDeleted  int
	SilencesCreated  int
	TicketsReopened  int
	Errors           []error
}

// Sync performs a full synchronization between alertmanager and ticket system
func (s *Synchronizer) Sync() (*SyncResult, error) {
	result := &SyncResult{
		Errors: make([]error, 0),
	}

	log.Println("Starting synchronization...")

	// Get all active silences
	silences, err := s.alertManager.ListSilences()
	if err != nil {
		return result, fmt.Errorf("failed to list silences: %w", err)
	}

	log.Printf("Found %d active silences", len(silences))

	// Process each silence
	now := time.Now()
	for _, silence := range silences {
		if silence.TicketRef == "" {
			log.Printf("Silence %s has no ticket reference, skipping", silence.ID)
			continue
		}

		// Record metrics for this silence
		s.metricsPublisher.RecordSilenceCheck(silence.ID, silence.TicketRef, now)
		s.metricsPublisher.RecordSilenceExpiry(silence.ID, silence.TicketRef, silence.EndsAt)

		if err := s.processSilence(silence, result); err != nil {
			log.Printf("Error processing silence %s: %v", silence.ID, err)
			result.Errors = append(result.Errors, fmt.Errorf("silence %s: %w", silence.ID, err))
		}
	}

	// Check for refired alerts if enabled
	if s.config.CheckAlerts {
		if err := s.checkRefiredAlerts(result); err != nil {
			log.Printf("Error checking refired alerts: %v", err)
			result.Errors = append(result.Errors, fmt.Errorf("check refired alerts: %w", err))
		}
	}

	log.Printf("Synchronization complete: extended=%d, deleted=%d, created=%d, reopened=%d, errors=%d",
		result.SilencesExtended, result.SilencesDeleted, result.SilencesCreated, result.TicketsReopened, len(result.Errors))

	// Push metrics to backend
	if err := s.metricsPublisher.Push(); err != nil {
		log.Printf("Warning: failed to push metrics: %v", err)
		result.Errors = append(result.Errors, fmt.Errorf("push metrics: %w", err))
	}

	return result, nil
}

// processSilence handles the synchronization logic for a single silence
func (s *Synchronizer) processSilence(silence *alertmanager.Silence, result *SyncResult) error {
	// Get the associated ticket
	tkt, err := s.ticketSystem.GetTicket(silence.TicketRef)
	if err != nil {
		return fmt.Errorf("failed to get ticket %s: %w", silence.TicketRef, err)
	}

	log.Printf("Processing silence %s with ticket %s (status: %s)", silence.ID, tkt.Key, tkt.Status)

	// Case 1: Ticket is resolved -> delete silence
	if s.ticketSystem.IsResolved(tkt) {
		log.Printf("Ticket %s is resolved, deleting silence %s", tkt.Key, silence.ID)
		if err := s.alertManager.DeleteSilence(silence.ID); err != nil {
			return fmt.Errorf("failed to delete silence: %w", err)
		}
		if err := s.ticketSystem.AddComment(tkt.Key, fmt.Sprintf("Silence %s has been automatically deleted because the ticket is resolved.", silence.ID)); err != nil {
			log.Printf("Warning: failed to add comment to ticket %s: %v", tkt.Key, err)
		}
		result.SilencesDeleted++
		return nil
	}

	// Case 2: Ticket is open and silence is about to expire -> extend silence
	if s.ticketSystem.IsOpen(tkt) {
		timeUntilExpiry := time.Until(silence.EndsAt)
		if timeUntilExpiry < s.config.ExpiryThreshold && timeUntilExpiry > 0 {
			newEndTime := time.Now().Add(s.config.ExtensionDuration)
			log.Printf("Ticket %s is open and silence %s expires in %v, extending until %v",
				tkt.Key, silence.ID, timeUntilExpiry, newEndTime)
			if err := s.alertManager.ExtendSilence(silence.ID, newEndTime); err != nil {
				return fmt.Errorf("failed to extend silence: %w", err)
			}
			if err := s.ticketSystem.AddComment(tkt.Key, fmt.Sprintf("Silence %s has been automatically extended until %v.", silence.ID, newEndTime.Format(time.RFC3339))); err != nil {
				log.Printf("Warning: failed to add comment to ticket %s: %v", tkt.Key, err)
			}
			result.SilencesExtended++
			return nil
		}

		// If silence has already expired, extend it
		if timeUntilExpiry <= 0 {
			newEndTime := time.Now().Add(s.config.ExtensionDuration)
			log.Printf("Ticket %s is open and silence %s has expired, extending until %v",
				tkt.Key, silence.ID, newEndTime)
			if err := s.alertManager.ExtendSilence(silence.ID, newEndTime); err != nil {
				return fmt.Errorf("failed to extend expired silence: %w", err)
			}
			if err := s.ticketSystem.AddComment(tkt.Key, fmt.Sprintf("Silence %s was expired and has been automatically extended until %v.", silence.ID, newEndTime.Format(time.RFC3339))); err != nil {
				log.Printf("Warning: failed to add comment to ticket %s: %v", tkt.Key, err)
			}
			result.SilencesExtended++
			return nil
		}
	}

	return nil
}

// checkRefiredAlerts checks if any alerts have refired for closed tickets and reopens them
func (s *Synchronizer) checkRefiredAlerts(result *SyncResult) error {
	// This is a more complex operation that requires tracking
	// We need to identify tickets that:
	// 1. Are closed
	// 2. Have an associated silence that has expired or doesn't exist
	// 3. Have alerts that are currently firing

	// For this implementation, we'll need to maintain some state or query both systems
	// Since we're running as a cron job, we'll check recent alerts

	// Get all alerts
	allAlerts, err := s.alertManager.GetAlerts(nil)
	if err != nil {
		return fmt.Errorf("failed to get alerts: %w", err)
	}

	log.Printf("Checking %d active alerts for closed tickets", len(allAlerts))

	// For each alert, check if there's a ticket reference in the labels
	for _, alert := range allAlerts {
		ticketRef, hasTicket := alert.Labels["ticket"]
		silenceID, hasSilence := alert.Labels["silence_id"]

		if !hasTicket {
			continue
		}

		// Get the ticket
		tkt, err := s.ticketSystem.GetTicket(ticketRef)
		if err != nil {
			log.Printf("Warning: failed to get ticket %s for alert: %v", ticketRef, err)
			continue
		}

		// If ticket is closed and there's no active silence, reopen ticket and create silence
		if s.ticketSystem.IsClosed(tkt) {
			// Check if there's an active silence
			hasActiveSilence := false
			if hasSilence {
				silence, err := s.alertManager.GetSilence(silenceID)
				if err == nil && time.Now().Before(silence.EndsAt) {
					hasActiveSilence = true
				}
			}

			if !hasActiveSilence {
				log.Printf("Alert refired for closed ticket %s, reopening and creating silence", tkt.Key)

				// Reopen the ticket
				reopenMsg := fmt.Sprintf("Alert has refired. Automatically reopening ticket and creating new silence.\n\nAlert: %v", alert.Labels)
				if err := s.ticketSystem.ReopenTicket(tkt.Key, reopenMsg); err != nil {
					log.Printf("Error reopening ticket %s: %v", tkt.Key, err)
					result.Errors = append(result.Errors, fmt.Errorf("reopen ticket %s: %w", tkt.Key, err))
					continue
				}
				result.TicketsReopened++

				// Create a new silence with the same matchers as before
				newSilence := &alertmanager.Silence{
					CreatedBy: "silence-manager",
					Comment:   fmt.Sprintf("Automatically recreated for refired alert"),
					StartsAt:  time.Now(),
					EndsAt:    time.Now().Add(s.config.DefaultSilenceDuration),
					TicketRef: tkt.Key,
					Matchers:  s.createMatchersFromAlert(alert),
				}

				silenceID, err := s.alertManager.CreateSilence(newSilence)
				if err != nil {
					log.Printf("Error creating silence for ticket %s: %v", tkt.Key, err)
					result.Errors = append(result.Errors, fmt.Errorf("create silence for %s: %w", tkt.Key, err))
					continue
				}

				result.SilencesCreated++
				log.Printf("Created new silence %s for reopened ticket %s", silenceID, tkt.Key)

				// Add comment to ticket with new silence ID
				if err := s.ticketSystem.AddComment(tkt.Key, fmt.Sprintf("New silence created: %s", silenceID)); err != nil {
					log.Printf("Warning: failed to add comment to ticket %s: %v", tkt.Key, err)
				}
			}
		}
	}

	return nil
}

// createMatchersFromAlert creates matchers from an alert's labels
func (s *Synchronizer) createMatchersFromAlert(alert *alertmanager.Alert) []alertmanager.Matcher {
	matchers := make([]alertmanager.Matcher, 0)

	// Add matchers for common labels
	importantLabels := []string{"alertname", "job", "instance", "severity"}
	for _, label := range importantLabels {
		if value, exists := alert.Labels[label]; exists {
			matchers = append(matchers, alertmanager.Matcher{
				Name:    label,
				Value:   value,
				IsRegex: false,
				IsEqual: true,
			})
		}
	}

	return matchers
}

// DefaultConfig returns a default synchronization configuration
func DefaultConfig() SyncConfig {
	return SyncConfig{
		ExpiryThreshold:        24 * time.Hour, // Extend if expiring within 24 hours
		ExtensionDuration:      7 * 24 * time.Hour, // Extend by 7 days
		DefaultSilenceDuration: 7 * 24 * time.Hour, // New silences last 7 days
		CheckAlerts:            true,
	}
}
