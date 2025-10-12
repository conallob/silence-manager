package metrics

import "time"

// NoopPublisher is a metrics publisher that does nothing
// Used when metrics are disabled (the default)
type NoopPublisher struct{}

// NewNoopPublisher creates a new no-op publisher
func NewNoopPublisher() Publisher {
	return &NoopPublisher{}
}

// RecordBuildInfo does nothing
func (n *NoopPublisher) RecordBuildInfo(version, commit, buildDate string) {
	// No-op
}

// RecordSilenceCheck does nothing
func (n *NoopPublisher) RecordSilenceCheck(silenceID, ticketKey string, timestamp time.Time) {
	// No-op
}

// RecordSilenceExpiry does nothing
func (n *NoopPublisher) RecordSilenceExpiry(silenceID, ticketKey string, expiresAt time.Time) {
	// No-op
}

// Push does nothing
func (n *NoopPublisher) Push() error {
	return nil
}

// Close does nothing
func (n *NoopPublisher) Close() error {
	return nil
}
