package main

import (
	"log"
	"os"

	"github.com/conallob/silence-manager/pkg/alertmanager"
	"github.com/conallob/silence-manager/pkg/config"
	"github.com/conallob/silence-manager/pkg/k8s"
	"github.com/conallob/silence-manager/pkg/metrics"
	"github.com/conallob/silence-manager/pkg/sync"
	"github.com/conallob/silence-manager/pkg/ticket"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("Starting silence-manager version=%s commit=%s date=%s", version, commit, date)

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
		os.Exit(1)
	}

	log.Printf("Configuration loaded successfully")
	log.Printf("Jira URL: %s", cfg.Jira.URL)
	log.Printf("Jira Project: %s", cfg.Jira.ProjectKey)

	// Determine Alertmanager URL (auto-discovery or explicit)
	alertmanagerURL := cfg.Alertmanager.URL
	if cfg.Alertmanager.AutoDiscover {
		log.Println("Alertmanager auto-discovery enabled")
		log.Printf("Discovery config: service-name=%s, label=%s, port=%d, namespaces=%v",
			cfg.Alertmanager.DiscoveryServiceName,
			cfg.Alertmanager.DiscoveryServiceLabel,
			cfg.Alertmanager.DiscoveryPort,
			cfg.Alertmanager.DiscoveryNamespaces)

		discovered, err := k8s.DiscoverAlertmanager(k8s.DiscoveryConfig{
			ServiceName:      cfg.Alertmanager.DiscoveryServiceName,
			ServiceLabel:     cfg.Alertmanager.DiscoveryServiceLabel,
			Port:             cfg.Alertmanager.DiscoveryPort,
			PreferNamespaces: cfg.Alertmanager.DiscoveryNamespaces,
		})
		if err != nil {
			log.Fatalf("Failed to discover Alertmanager: %v", err)
			os.Exit(1)
		}
		alertmanagerURL = discovered.URL
		log.Printf("Using discovered Alertmanager: %s", alertmanagerURL)
	} else {
		log.Printf("Using configured Alertmanager URL: %s", alertmanagerURL)
	}

	log.Printf("Alertmanager URL: %s", alertmanagerURL)
	log.Printf("Alertmanager Auth Type: %s", cfg.Alertmanager.AuthType)

	// Initialize Alertmanager client
	am := alertmanager.NewPrometheusAlertManagerWithConfig(alertmanager.AlertManagerConfig{
		BaseURL:          alertmanagerURL,
		AuthType:         cfg.Alertmanager.AuthType,
		Username:         cfg.Alertmanager.Username,
		Password:         cfg.Alertmanager.Password,
		BearerToken:      cfg.Alertmanager.BearerToken,
		AnnotationPrefix: cfg.Sync.AnnotationPrefix,
	})
	log.Println("Initialized Prometheus Alertmanager client")

	// Initialize Jira client
	ts := ticket.NewJiraTicketSystem(
		cfg.Jira.URL,
		cfg.Jira.Username,
		cfg.Jira.APIToken,
		cfg.Jira.ProjectKey,
		cfg.Sync.AnnotationPrefix,
	)
	log.Println("Initialized Jira ticket system client")

	// Create synchronizer
	expiryThreshold, extensionDuration, defaultSilenceDuration := cfg.GetSyncDurations()
	syncConfig := sync.SyncConfig{
		ExpiryThreshold:        expiryThreshold,
		ExtensionDuration:      extensionDuration,
		DefaultSilenceDuration: defaultSilenceDuration,
		CheckAlerts:            cfg.Sync.CheckAlerts,
	}

	log.Printf("Sync configuration:")
	log.Printf("  Annotation prefix: %s", cfg.Sync.AnnotationPrefix)
	log.Printf("  Expiry threshold: %v", syncConfig.ExpiryThreshold)
	log.Printf("  Extension duration: %v", syncConfig.ExtensionDuration)
	log.Printf("  Default silence duration: %v", syncConfig.DefaultSilenceDuration)
	log.Printf("  Check alerts: %v", syncConfig.CheckAlerts)

	synchronizer := sync.NewSynchronizer(am, ts, syncConfig)
	log.Println("Created synchronizer")

	// Initialize metrics publisher if enabled
	if cfg.Metrics.Enabled {
		log.Printf("Metrics publishing enabled: backend=%s", cfg.Metrics.Backend)

		metricsURL := cfg.Metrics.URL
		if cfg.Metrics.AutoDiscover {
			log.Println("Metrics backend auto-discovery enabled")
			log.Printf("Discovery config: service-name=%s, label=%s, port=%d, namespaces=%v",
				cfg.Metrics.DiscoveryServiceName,
				cfg.Metrics.DiscoveryServiceLabel,
				cfg.Metrics.DiscoveryPort,
				cfg.Metrics.DiscoveryNamespaces)

			var discovered *k8s.DiscoveredService
			var discErr error

			discoveryConfig := k8s.DiscoveryConfig{
				ServiceName:      cfg.Metrics.DiscoveryServiceName,
				ServiceLabel:     cfg.Metrics.DiscoveryServiceLabel,
				Port:             cfg.Metrics.DiscoveryPort,
				PreferNamespaces: cfg.Metrics.DiscoveryNamespaces,
			}

			switch cfg.Metrics.Backend {
			case "pushgateway":
				discovered, discErr = k8s.DiscoverPushgateway(discoveryConfig)
			case "otel":
				discovered, discErr = k8s.DiscoverOTelCollector(discoveryConfig)
			default:
				log.Fatalf("Unknown metrics backend: %s", cfg.Metrics.Backend)
				os.Exit(1)
			}

			if discErr != nil {
				log.Fatalf("Failed to discover metrics backend: %v", discErr)
				os.Exit(1)
			}

			metricsURL = discovered.URL
			log.Printf("Using discovered metrics backend: %s", metricsURL)
		} else {
			log.Printf("Using configured metrics backend URL: %s", metricsURL)
		}

		var publisher metrics.Publisher
		var metricsErr error

		switch cfg.Metrics.Backend {
		case "pushgateway":
			publisher, metricsErr = metrics.NewPushgatewayPublisher(metrics.PushgatewayConfig{
				URL:     metricsURL,
				JobName: cfg.Metrics.JobName,
			})
		case "otel":
			publisher, metricsErr = metrics.NewOTelPublisher(metrics.OTelConfig{
				URL:      metricsURL,
				Insecure: cfg.Metrics.OTelInsecure,
			})
		default:
			log.Fatalf("Unknown metrics backend: %s", cfg.Metrics.Backend)
			os.Exit(1)
		}

		if metricsErr != nil {
			log.Fatalf("Failed to initialize metrics publisher: %v", metricsErr)
			os.Exit(1)
		}

		// Record build info
		publisher.RecordBuildInfo(version, commit, date)

		// Set the publisher on the synchronizer
		synchronizer.SetMetricsPublisher(publisher)
		log.Printf("Metrics publisher initialized and configured")

		// Ensure we close the publisher when done
		defer func() {
			if err := publisher.Close(); err != nil {
				log.Printf("Warning: failed to close metrics publisher: %v", err)
			}
		}()
	} else {
		log.Println("Metrics publishing disabled")
	}

	// Perform synchronization
	log.Println("Starting synchronization run...")
	result, err := synchronizer.Sync()
	if err != nil {
		log.Printf("Synchronization completed with errors: %v", err)
	}

	// Log results
	log.Println("=== Synchronization Results ===")
	log.Printf("Silences extended: %d", result.SilencesExtended)
	log.Printf("Silences deleted: %d", result.SilencesDeleted)
	log.Printf("Silences created: %d", result.SilencesCreated)
	log.Printf("Tickets reopened: %d", result.TicketsReopened)
	log.Printf("Errors: %d", len(result.Errors))

	if len(result.Errors) > 0 {
		log.Println("Errors encountered:")
		for i, err := range result.Errors {
			log.Printf("  %d. %v", i+1, err)
		}
		os.Exit(1)
	}

	log.Println("Synchronization completed successfully")
}
