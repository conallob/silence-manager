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
│   └── config/                 # Configuration management
│       └── config.go           # Environment-based configuration
├── deployments/                # Kubernetes manifests
│   ├── cronjob.yaml           # CronJob definition
│   ├── configmap.yaml         # Configuration
│   ├── secret.yaml.example    # Secret template
│   ├── serviceaccount.yaml    # ServiceAccount
│   └── kustomization.yaml     # Kustomize configuration
├── Dockerfile                  # Container image build
└── README.md                   # Comprehensive documentation
```

### Architecture Decisions Made

1. **Alertmanager Integration**: Uses Prometheus Alertmanager API v2 for all operations (GET, POST, DELETE silences and alerts)

2. **Ticket Tracking Integration**: Initial support for Atlassian Jira using API v3 with basic authentication

3. **Synchronization Strategy**: Polling-based approach running as a Kubernetes CronJob (default: every 15 minutes)

4. **State Management**: Stateless design where coupling is tracked through:
   - Silence comments contain ticket references: `Ticket: PROJECT-123`
   - Ticket descriptions contain silence references: `Silence: <silence-id>`

### Key Features

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
- `ALERTMANAGER_URL`: Alertmanager URL (default: http://alertmanager:9093)
- `ALERTMANAGER_AUTH_TYPE`: Authentication type - "none", "basic", or "bearer" (default: none)
- `ALERTMANAGER_USERNAME`: Username for basic auth
- `ALERTMANAGER_PASSWORD`: Password for basic auth
- `ALERTMANAGER_BEARER_TOKEN`: Bearer token for token auth
- `SYNC_EXPIRY_THRESHOLD_HOURS`: Hours before expiry to extend (default: 24)
- `SYNC_EXTENSION_DURATION_HOURS`: Hours to extend by (default: 168)
- `SYNC_DEFAULT_SILENCE_DURATION_HOURS`: Default silence duration (default: 168)
- `SYNC_CHECK_ALERTS`: Check for refired alerts (default: true)

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
- Synchronization logic: `pkg/sync/sync.go:32`
- Configuration: `pkg/config/config.go:10`
