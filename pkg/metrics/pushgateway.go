package metrics

import (
	"fmt"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
)

// PushgatewayPublisher publishes metrics to a Prometheus Pushgateway
type PushgatewayPublisher struct {
	url      string
	jobName  string
	registry *prometheus.Registry

	// Metrics
	buildInfo         *prometheus.GaugeVec
	silenceLastChecked *prometheus.GaugeVec
	silenceExpiringIn  *prometheus.GaugeVec
}

// PushgatewayConfig holds configuration for Pushgateway
type PushgatewayConfig struct {
	URL     string
	JobName string
}

// NewPushgatewayPublisher creates a new Pushgateway metrics publisher
func NewPushgatewayPublisher(cfg PushgatewayConfig) (Publisher, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("pushgateway URL is required")
	}

	if cfg.JobName == "" {
		cfg.JobName = "silence_manager"
	}

	// Create a new registry for this publisher
	registry := prometheus.NewRegistry()

	// Create metrics
	buildInfo := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "silence_manager_build_info",
			Help: "Build information for silence-manager including version, commit, and build date",
		},
		[]string{"version", "commit", "build_date"},
	)

	silenceLastChecked := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "silence_manager_silence_last_checked",
			Help: "Unix timestamp of when a silence was last checked",
		},
		[]string{"silence_id", "ticket"},
	)

	silenceExpiringIn := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "silence_manager_silence_expiring_in",
			Help: "Seconds until a silence expires",
		},
		[]string{"silence_id", "ticket"},
	)

	// Register metrics
	registry.MustRegister(buildInfo)
	registry.MustRegister(silenceLastChecked)
	registry.MustRegister(silenceExpiringIn)

	log.Printf("Initialized Pushgateway metrics publisher: url=%s, job=%s", cfg.URL, cfg.JobName)

	return &PushgatewayPublisher{
		url:                cfg.URL,
		jobName:            cfg.JobName,
		registry:           registry,
		buildInfo:          buildInfo,
		silenceLastChecked: silenceLastChecked,
		silenceExpiringIn:  silenceExpiringIn,
	}, nil
}

// RecordBuildInfo records version and build information
func (p *PushgatewayPublisher) RecordBuildInfo(version, commit, buildDate string) {
	p.buildInfo.WithLabelValues(version, commit, buildDate).Set(1)
}

// RecordSilenceCheck records when a silence was checked
func (p *PushgatewayPublisher) RecordSilenceCheck(silenceID, ticketKey string, timestamp time.Time) {
	p.silenceLastChecked.WithLabelValues(silenceID, ticketKey).Set(float64(timestamp.Unix()))
}

// RecordSilenceExpiry records when a silence will expire
func (p *PushgatewayPublisher) RecordSilenceExpiry(silenceID, ticketKey string, expiresAt time.Time) {
	secondsUntilExpiry := time.Until(expiresAt).Seconds()
	// If already expired, set to 0
	if secondsUntilExpiry < 0 {
		secondsUntilExpiry = 0
	}
	p.silenceExpiringIn.WithLabelValues(silenceID, ticketKey).Set(secondsUntilExpiry)
}

// Push sends all recorded metrics to the Pushgateway
func (p *PushgatewayPublisher) Push() error {
	log.Printf("Pushing metrics to Pushgateway: %s", p.url)

	pusher := push.New(p.url, p.jobName).
		Gatherer(p.registry)

	if err := pusher.Push(); err != nil {
		return fmt.Errorf("failed to push metrics to pushgateway: %w", err)
	}

	log.Println("Successfully pushed metrics to Pushgateway")
	return nil
}

// Close cleans up any resources
func (p *PushgatewayPublisher) Close() error {
	// No cleanup needed for Pushgateway
	return nil
}
