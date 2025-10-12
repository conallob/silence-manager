package metrics

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

// OTelPublisher publishes metrics to an OpenTelemetry collector
type OTelPublisher struct {
	meterProvider *sdkmetric.MeterProvider
	meter         metric.Meter
	ctx           context.Context

	// Build info tracking
	buildVersion   string
	buildCommit    string
	buildDate      string

	// Metrics for recording
	silenceChecks  []SilenceMetric
	silenceExpiries []SilenceMetric
}

// OTelConfig holds configuration for OpenTelemetry
type OTelConfig struct {
	URL      string
	Insecure bool
}

// NewOTelPublisher creates a new OpenTelemetry metrics publisher
func NewOTelPublisher(cfg OTelConfig) (Publisher, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("otel collector URL is required")
	}

	ctx := context.Background()

	// Create OTLP HTTP exporter
	opts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(cfg.URL),
	}

	if cfg.Insecure {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}

	exporter, err := otlpmetrichttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String("silence-manager"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create meter provider
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
	)

	// Create meter
	meter := meterProvider.Meter("silence-manager")

	log.Printf("Initialized OpenTelemetry metrics publisher: url=%s, insecure=%v", cfg.URL, cfg.Insecure)

	return &OTelPublisher{
		meterProvider:   meterProvider,
		meter:           meter,
		ctx:             ctx,
		silenceChecks:   make([]SilenceMetric, 0),
		silenceExpiries: make([]SilenceMetric, 0),
	}, nil
}

// RecordBuildInfo records version and build information
func (o *OTelPublisher) RecordBuildInfo(version, commit, buildDate string) {
	o.buildVersion = version
	o.buildCommit = commit
	o.buildDate = buildDate
}

// RecordSilenceCheck records when a silence was checked
func (o *OTelPublisher) RecordSilenceCheck(silenceID, ticketKey string, timestamp time.Time) {
	o.silenceChecks = append(o.silenceChecks, SilenceMetric{
		SilenceID: silenceID,
		TicketKey: ticketKey,
		Value:     float64(timestamp.Unix()),
		Timestamp: timestamp,
	})
}

// RecordSilenceExpiry records when a silence will expire
func (o *OTelPublisher) RecordSilenceExpiry(silenceID, ticketKey string, expiresAt time.Time) {
	secondsUntilExpiry := time.Until(expiresAt).Seconds()
	// If already expired, set to 0
	if secondsUntilExpiry < 0 {
		secondsUntilExpiry = 0
	}

	o.silenceExpiries = append(o.silenceExpiries, SilenceMetric{
		SilenceID: silenceID,
		TicketKey: ticketKey,
		Value:     secondsUntilExpiry,
		Timestamp: time.Now(),
	})
}

// Push sends all recorded metrics to the OpenTelemetry collector
func (o *OTelPublisher) Push() error {
	log.Println("Pushing metrics to OpenTelemetry collector")

	// Create build info gauge
	if o.buildVersion != "" {
		buildInfo, err := o.meter.Float64ObservableGauge("silence_manager_build_info",
			metric.WithDescription("Build information for silence-manager"),
		)
		if err != nil {
			return fmt.Errorf("failed to create build info gauge: %w", err)
		}

		_, err = o.meter.RegisterCallback(
			func(ctx context.Context, obs metric.Observer) error {
				obs.ObserveFloat64(buildInfo, 1,
					metric.WithAttributes(
						attribute.String("version", o.buildVersion),
						attribute.String("commit", o.buildCommit),
						attribute.String("build_date", o.buildDate),
					),
				)
				return nil
			},
			buildInfo,
		)
		if err != nil {
			return fmt.Errorf("failed to register build info callback: %w", err)
		}
	}

	// Record silence check timestamps
	if len(o.silenceChecks) > 0 {
		lastChecked, err := o.meter.Float64ObservableGauge("silence_manager_silence_last_checked",
			metric.WithDescription("Unix timestamp of when a silence was last checked"),
		)
		if err != nil {
			return fmt.Errorf("failed to create silence last checked gauge: %w", err)
		}

		checks := o.silenceChecks // Capture for closure
		_, err = o.meter.RegisterCallback(
			func(ctx context.Context, obs metric.Observer) error {
				for _, check := range checks {
					obs.ObserveFloat64(lastChecked, check.Value,
						metric.WithAttributes(
							attribute.String("silence_id", check.SilenceID),
							attribute.String("ticket", check.TicketKey),
						),
					)
				}
				return nil
			},
			lastChecked,
		)
		if err != nil {
			return fmt.Errorf("failed to register silence check callback: %w", err)
		}
	}

	// Record silence expiry times
	if len(o.silenceExpiries) > 0 {
		expiringIn, err := o.meter.Float64ObservableGauge("silence_manager_silence_expiring_in",
			metric.WithDescription("Seconds until a silence expires"),
		)
		if err != nil {
			return fmt.Errorf("failed to create silence expiring gauge: %w", err)
		}

		expiries := o.silenceExpiries // Capture for closure
		_, err = o.meter.RegisterCallback(
			func(ctx context.Context, obs metric.Observer) error {
				for _, expiry := range expiries {
					obs.ObserveFloat64(expiringIn, expiry.Value,
						metric.WithAttributes(
							attribute.String("silence_id", expiry.SilenceID),
							attribute.String("ticket", expiry.TicketKey),
						),
					)
				}
				return nil
			},
			expiringIn,
		)
		if err != nil {
			return fmt.Errorf("failed to register silence expiry callback: %w", err)
		}
	}

	// Force a flush to ensure metrics are sent
	if err := o.meterProvider.ForceFlush(o.ctx); err != nil {
		return fmt.Errorf("failed to flush metrics: %w", err)
	}

	log.Printf("Successfully pushed metrics to OpenTelemetry collector (checks=%d, expiries=%d)",
		len(o.silenceChecks), len(o.silenceExpiries))
	return nil
}

// Close cleans up resources and shuts down the meter provider
func (o *OTelPublisher) Close() error {
	if o.meterProvider != nil {
		if err := o.meterProvider.Shutdown(o.ctx); err != nil {
			return fmt.Errorf("failed to shutdown meter provider: %w", err)
		}
	}
	return nil
}
