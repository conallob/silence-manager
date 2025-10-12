package metrics

import "time"

// Publisher defines the interface for metrics publishers
type Publisher interface {
	// RecordBuildInfo records version and build information
	RecordBuildInfo(version, commit, buildDate string)

	// RecordSilenceCheck records when a silence was checked
	// silenceID is the unique identifier for the silence
	// ticketKey is the associated ticket reference
	// timestamp is when the check occurred
	RecordSilenceCheck(silenceID, ticketKey string, timestamp time.Time)

	// RecordSilenceExpiry records when a silence will expire
	// silenceID is the unique identifier for the silence
	// ticketKey is the associated ticket reference
	// expiresAt is when the silence will expire
	RecordSilenceExpiry(silenceID, ticketKey string, expiresAt time.Time)

	// Push sends all recorded metrics to the backend
	// This should be called after all metrics have been recorded
	Push() error

	// Close cleans up any resources used by the publisher
	Close() error
}

// SilenceMetric represents a metric associated with a silence
type SilenceMetric struct {
	SilenceID string
	TicketKey string
	Value     float64
	Timestamp time.Time
}

// BuildInfo represents version and build information
type BuildInfo struct {
	Version   string
	Commit    string
	BuildDate string
}
