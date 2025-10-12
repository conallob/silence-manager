# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

silence-manager is a Kubernetes CronJob utility written in Golang that synchronizes Prometheus Alertmanager silences with ticket tracking systems (initially Jira). It ensures that both systems remain in sync by automatically managing silences based on ticket status.

## Current State

The project has been fully implemented with the following components:

- **Core Functionality**: Complete implementation of synchronization logic
- **Abstract Interfaces**: Extensible design supporting multiple alertmanager and ticket systems
- **Prometheus Alertmanager Client**: Full API integration
- **Jira Ticket Client**: Complete Jira API v3 integration
- **Kubernetes Deployment**: CronJob, ConfigMap, Secret, and ServiceAccount manifests
- **Docker Support**: Multi-stage Dockerfile for containerization

## Architecture

### Project Structure

```
silence-manager/
├── cmd/silence-manager/        # Main application entry point
├── pkg/
│   ├── alertmanager/           # AlertManager interface and Prometheus implementation
│   │   ├── types.go            # Interface definitions and common types
│   │   └── prometheus.go       # Prometheus Alertmanager client
│   ├── ticket/                 # Ticket interface and implementations
│   │   ├── types.go            # Interface definitions and common types
│   │   └── jira.go             # Jira ticket system client
│   ├── sync/                   # Core synchronization logic
│   │   └── sync.go             # Synchronizer implementation
│   ├── metrics/                # Metrics publishing
│   │   ├── types.go            # Interface definitions and common types
│   │   ├── noop.go             # No-op publisher (default)
│   │   ├── pushgateway.go      # Prometheus Pushgateway client
│   │   └── otel.go             # OpenTelemetry Collector client
│   ├── k8s/                    # Kubernetes integration
│   │   └── discovery.go        # Service discovery for Alertmanager and metrics backends
│   └── config/                 # Configuration management
│       └── config.go           # Environment-based configuration
├── deployments/                # Kubernetes manifests
│   ├── cronjob.yaml           # CronJob definition
│   ├── configmap.yaml         # Configuration
│   ├── secret.yaml.example    # Secret template
│   ├── serviceaccount.yaml    # ServiceAccount
│   ├── clusterrole.yaml       # ClusterRole for service discovery
│   ├── clusterrolebinding.yaml # ClusterRoleBinding for service discovery
│   └── kustomization.yaml     # Kustomize configuration
├── Dockerfile                  # Container image build
└── README.md                   # Comprehensive documentation
```

### Architecture Decisions Made

1. **Alertmanager Integration**: Uses Prometheus Alertmanager API v2 for all operations (GET, POST, DELETE silences and alerts)

2. **Ticket Tracking Integration**: Initial support for Atlassian Jira using API v3 with basic authentication

3. **Synchronization Strategy**: Polling-based approach running as a Kubernetes CronJob (default: every 15 minutes)

4. **State Management**: Stateless design where coupling is tracked through annotations with a configurable prefix (default: `silence-manager`):
   - Silence comments contain ticket references: `# silence-manager: PROJECT-123`
   - Ticket descriptions contain silence references: `silence-manager: <silence-id>`

### Key Features

- **Kubernetes Service Discovery**: Automatically discovers Alertmanager services across all namespaces using the Kubernetes API
- **Optional Metrics Publishing**: Publish metrics to Prometheus Pushgateway or OpenTelemetry Collector (disabled by default)
- **Automatic Silence Extension**: When a ticket is open and the silence is about to expire
- **Automatic Silence Deletion**: When a ticket is resolved
- **Automatic Ticket Reopening**: When a ticket is closed but the alert refires
- **Flexible Authentication**: Support for basic auth and bearer token authentication for Alertmanager
- **Secret Management**: Compatible with External Secrets Operator for production-grade secret management
- **Configurable Thresholds**: All durations and behaviors configurable via environment variables
- **Comprehensive Logging**: Detailed logging for debugging and monitoring

## Development Setup

### Building the Project

```bash
# Build locally
go build -o silence-manager ./cmd/silence-manager

# Build Docker image
docker build -t silence-manager:latest .

# Test goreleaser configuration
goreleaser build --snapshot --clean
```

### Creating a Release

This project uses GoReleaser for automated releases:

1. Ensure all changes are committed and pushed
2. Create and push a new tag:
   ```bash
   git tag -a v0.1.0 -m "Release v0.1.0"
   git push origin v0.1.0
   ```
3. GitHub Actions will automatically:
   - Build binaries for Linux, macOS, and Windows
   - Build multi-arch container images (amd64, arm64)
   - Push images to GitHub Container Registry
   - Create a GitHub release with artifacts

Configuration files:
- `.goreleaser.yaml` - GoReleaser configuration
- `.github/workflows/release.yml` - Release workflow
- `Dockerfile.goreleaser` - Dockerfile for container images

### Running Tests

```bash
go test ./...
```

### Running Locally

```bash
# Set required environment variables
export JIRA_URL="https://yourcompany.atlassian.net"
export JIRA_USERNAME="admin@example.com"
export JIRA_API_TOKEN="your-api-token"
export JIRA_PROJECT_KEY="OPS"
export ALERTMANAGER_URL="http://localhost:9093"

# Run the application
go run ./cmd/silence-manager
```

### Configuration

All configuration is via environment variables (see pkg/config/config.go):

**Required:**
- `JIRA_URL`: Jira instance URL
- `JIRA_USERNAME`: Jira username/email
- `JIRA_API_TOKEN`: Jira API token
- `JIRA_PROJECT_KEY`: Default Jira project key

**Optional:**
- `ALERTMANAGER_URL`: Alertmanager URL (if not set, auto-discovery is enabled)
- `ALERTMANAGER_AUTO_DISCOVER`: Enable auto-discovery (default: true when URL is empty)
- `ALERTMANAGER_DISCOVERY_SERVICE_NAME`: Service name pattern for discovery (default: alertmanager)
- `ALERTMANAGER_DISCOVERY_SERVICE_LABEL`: Label selector for discovery (default: app=alertmanager)
- `ALERTMANAGER_DISCOVERY_PORT`: Port for discovered services (default: 9093)
- `ALERTMANAGER_DISCOVERY_NAMESPACES`: Comma-separated list of preferred namespaces (default: monitoring,default)
- `ALERTMANAGER_AUTH_TYPE`: Authentication type - "none", "basic", or "bearer" (default: none)
- `ALERTMANAGER_USERNAME`: Username for basic auth
- `ALERTMANAGER_PASSWORD`: Password for basic auth
- `ALERTMANAGER_BEARER_TOKEN`: Bearer token for token auth
- `SYNC_ANNOTATION_PREFIX`: Prefix for annotations linking silences and tickets (default: silence-manager)
- `SYNC_EXPIRY_THRESHOLD_HOURS`: Hours before expiry to extend (default: 24)
- `SYNC_EXTENSION_DURATION_HOURS`: Hours to extend by (default: 168)
- `SYNC_DEFAULT_SILENCE_DURATION_HOURS`: Default silence duration (default: 168)
- `SYNC_CHECK_ALERTS`: Check for refired alerts (default: true)

**Metrics (Optional - disabled by default):**
- `METRICS_ENABLED`: Enable metrics publishing (default: false)
- `METRICS_BACKEND`: Metrics backend - "pushgateway" or "otel" (required if enabled)
- `METRICS_URL`: Metrics backend URL (if not set and metrics enabled, auto-discovery is used)
- `METRICS_PUSHGATEWAY_JOB_NAME`: Job name for Pushgateway (default: silence_manager)
- `METRICS_OTEL_INSECURE`: Use insecure connection for OTel (default: true)
- `METRICS_DISCOVERY_SERVICE_NAME`: Service name pattern for discovery
- `METRICS_DISCOVERY_SERVICE_LABEL`: Label selector for discovery
- `METRICS_DISCOVERY_PORT`: Port for discovered services (9091 for Pushgateway, 4318 for OTel)
- `METRICS_DISCOVERY_NAMESPACES`: Comma-separated list of preferred namespaces (default: monitoring,default)

## Extending the Application

### Adding a New Ticket System

1. Implement the `ticket.TicketSystem` interface in `pkg/ticket/`
2. Add configuration fields in `pkg/config/config.go`
3. Update `cmd/silence-manager/main.go` to instantiate the new client based on config

### Adding a New Alertmanager System

1. Implement the `alertmanager.AlertManager` interface in `pkg/alertmanager/`
2. Add configuration fields in `pkg/config/config.go`
3. Update `cmd/silence-manager/main.go` to instantiate the new client based on config

## File References

- Main application: `cmd/silence-manager/main.go`
- AlertManager interface: `pkg/alertmanager/types.go:22`
- Prometheus client: `pkg/alertmanager/prometheus.go:13`
- Ticket interface: `pkg/ticket/types.go:33`
- Jira client: `pkg/ticket/jira.go:14`
- Synchronization logic: `pkg/sync/sync.go:25`
- Metrics interface: `pkg/metrics/types.go:6`
- Pushgateway client: `pkg/metrics/pushgateway.go:12`
- OTel client: `pkg/metrics/otel.go:17`
- Kubernetes service discovery: `pkg/k8s/discovery.go:20`
- Configuration: `pkg/config/config.go:12`

## Kubernetes Service Discovery

The application includes automatic service discovery for Alertmanager using the Kubernetes API:

### How It Works

1. When `ALERTMANAGER_URL` is not set (or empty), auto-discovery is automatically enabled
2. The application uses in-cluster Kubernetes credentials to query the API
3. It searches for services matching either:
   - A label selector (default: `app=alertmanager`)
   - A name pattern (default: contains `alertmanager`)
4. Search order:
   - Preferred namespaces first (default: `monitoring`, `default`)
   - All other namespaces if not found in preferred namespaces
5. The first matching service is selected and used

### RBAC Requirements

The service account requires the following cluster-wide permissions:
- `get`, `list` on `services` and `endpoints`
- `get`, `list` on `namespaces`

These are defined in:
- ClusterRole: `deployments/clusterrole.yaml`
- ClusterRoleBinding: `deployments/clusterrolebinding.yaml`

### Discovery Configuration

Discovery behavior can be customized through environment variables:
- `ALERTMANAGER_DISCOVERY_SERVICE_NAME`: Service name pattern to match
- `ALERTMANAGER_DISCOVERY_SERVICE_LABEL`: Label selector for matching services
- `ALERTMANAGER_DISCOVERY_PORT`: Port to use (default: 9093)
- `ALERTMANAGER_DISCOVERY_NAMESPACES`: Comma-separated list of preferred namespaces

### Disabling Auto-Discovery

To use a specific Alertmanager URL instead of auto-discovery:
1. Set `ALERTMANAGER_URL` environment variable in the CronJob
2. Auto-discovery will be automatically disabled when a URL is provided
