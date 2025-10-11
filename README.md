# Silence Manager

A Kubernetes CronJob utility written in Golang that synchronizes Prometheus Alertmanager silences with ticket tracking systems (initially Jira).

## Overview

Silence Manager ensures that Alertmanager silences and tracking tickets remain synchronized by:

1. **Extending silences** when the associated ticket is still open and the silence is about to expire
2. **Deleting silences** when the associated ticket is marked as resolved
3. **Reopening tickets and recreating silences** when a ticket is closed but the alert refires

## Architecture

The application uses abstract interfaces to support multiple alertmanager and ticket system implementations:

- **AlertManager Interface**: Abstracts alertmanager operations (currently supports Prometheus Alertmanager)
- **Ticket System Interface**: Abstracts ticket operations (currently supports Atlassian Jira)

This design allows for easy extension to support additional systems in the future.

## Features

- Automatic silence extension for open tickets
- Automatic silence deletion for resolved tickets
- Automatic ticket reopening and silence recreation for refired alerts
- Configurable thresholds and durations
- Runs as a Kubernetes CronJob
- Comprehensive logging

## Project Structure

```
silence-manager/
├── cmd/
│   └── silence-manager/    # Main application entry point
├── pkg/
│   ├── alertmanager/        # Alertmanager interface and Prometheus implementation
│   ├── ticket/              # Ticket interface and Jira implementation
│   ├── sync/                # Synchronization logic
│   └── config/              # Configuration management
├── deployments/             # Kubernetes manifests
├── Dockerfile               # Container image build
└── README.md
```

## Prerequisites

- Go 1.21 or higher
- Docker (for building container images)
- Kubernetes cluster
- Prometheus Alertmanager instance
- Jira account with API token

## Configuration

The application is configured via environment variables:

### Required Configuration

| Variable | Description | Example |
|----------|-------------|---------|
| `JIRA_URL` | Jira instance URL | `https://yourcompany.atlassian.net` |
| `JIRA_USERNAME` | Jira username (email) | `admin@example.com` |
| `JIRA_API_TOKEN` | Jira API token | `your-api-token` |
| `JIRA_PROJECT_KEY` | Jira project key | `OPS` |

### Optional Configuration

#### Alertmanager Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `ALERTMANAGER_URL` | Alertmanager URL | `http://alertmanager:9093` |
| `ALERTMANAGER_AUTH_TYPE` | Authentication type: `none`, `basic`, or `bearer` | `none` |
| `ALERTMANAGER_USERNAME` | Username for basic auth | - |
| `ALERTMANAGER_PASSWORD` | Password for basic auth | - |
| `ALERTMANAGER_BEARER_TOKEN` | Bearer token for token auth | - |

#### Sync Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `SYNC_EXPIRY_THRESHOLD_HOURS` | Hours before expiry to extend silence | `24` |
| `SYNC_EXTENSION_DURATION_HOURS` | Hours to extend silence by | `168` (7 days) |
| `SYNC_DEFAULT_SILENCE_DURATION_HOURS` | Default duration for new silences | `168` (7 days) |
| `SYNC_CHECK_ALERTS` | Check for refired alerts | `true` |

## Building

### Local Build

```bash
go build -o silence-manager ./cmd/silence-manager
```

### Docker Build

```bash
docker build -t silence-manager:latest .
```

### For Kubernetes

```bash
# Build and tag for your registry
docker build -t your-registry/silence-manager:latest .
docker push your-registry/silence-manager:latest
```

## Installation

### Using Pre-built Binaries

Download the latest release for your platform from the [Releases page](https://github.com/conallob/silence-manager/releases):

```bash
# Example for Linux amd64
wget https://github.com/conallob/silence-manager/releases/download/v0.1.0/silence-manager_Linux_x86_64.tar.gz
tar -xzf silence-manager_Linux_x86_64.tar.gz
./silence-manager
```

### Using Container Images

Pre-built multi-arch container images are available from GitHub Container Registry:

```bash
# Pull the latest version
docker pull ghcr.io/conallob/silence-manager:latest

# Or a specific version
docker pull ghcr.io/conallob/silence-manager:v0.1.0
```

Supported architectures: `amd64`, `arm64`

### Using Go Install

```bash
go install github.com/conallob/silence-manager/cmd/silence-manager@latest
```

## Releasing

This project uses [GoReleaser](https://goreleaser.com/) for automated releases. To create a new release:

1. Create and push a new tag:
   ```bash
   git tag -a v0.1.0 -m "Release v0.1.0"
   git push origin v0.1.0
   ```

2. GitHub Actions will automatically:
   - Build binaries for multiple platforms (Linux, macOS, Windows)
   - Build multi-arch container images (amd64, arm64)
   - Push container images to GitHub Container Registry
   - Create a GitHub release with artifacts and release notes

3. The release will be available at:
   - GitHub Releases: `https://github.com/conallob/silence-manager/releases`
   - Container Registry: `ghcr.io/conallob/silence-manager:VERSION`

## Deployment

### 1. Create Namespace

```bash
kubectl create namespace monitoring
```

### 2. Create Secret

You have multiple options for managing secrets:

#### Option A: kubectl (Simple)

Create a secret with your credentials:

```bash
kubectl create secret generic silence-manager-secrets \
  --from-literal=jira-url=https://yourcompany.atlassian.net \
  --from-literal=jira-username=admin@example.com \
  --from-literal=jira-api-token=your-api-token \
  -n monitoring
```

If your Alertmanager requires authentication:

```bash
# For basic auth
kubectl create secret generic silence-manager-secrets \
  --from-literal=jira-url=https://yourcompany.atlassian.net \
  --from-literal=jira-username=admin@example.com \
  --from-literal=jira-api-token=your-api-token \
  --from-literal=alertmanager-username=admin \
  --from-literal=alertmanager-password=your-password \
  -n monitoring

# For bearer token auth
kubectl create secret generic silence-manager-secrets \
  --from-literal=jira-url=https://yourcompany.atlassian.net \
  --from-literal=jira-username=admin@example.com \
  --from-literal=jira-api-token=your-api-token \
  --from-literal=alertmanager-bearer-token=your-bearer-token \
  -n monitoring
```

#### Option B: External Secrets Operator (Recommended for Production)

For production environments, use [External Secrets Operator](https://external-secrets.io/) to sync secrets from your secret management system (AWS Secrets Manager, HashiCorp Vault, Azure Key Vault, GCP Secret Manager, etc.):

1. Install External Secrets Operator:
   ```bash
   helm repo add external-secrets https://charts.external-secrets.io
   helm install external-secrets external-secrets/external-secrets -n external-secrets-system --create-namespace
   ```

2. Create a SecretStore pointing to your secrets backend (example for AWS Secrets Manager):
   ```yaml
   apiVersion: external-secrets.io/v1beta1
   kind: SecretStore
   metadata:
     name: aws-secretsmanager
     namespace: monitoring
   spec:
     provider:
       aws:
         service: SecretsManager
         region: us-east-1
         auth:
           jwt:
             serviceAccountRef:
               name: external-secrets-sa
   ```

3. Create an ExternalSecret resource (see `deployments/externalsecret.yaml.example`)

4. The operator will automatically create and sync the `silence-manager-secrets` Kubernetes secret

### 3. Configure Settings

Edit `deployments/configmap.yaml` to set your desired configuration:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: silence-manager-config
  namespace: monitoring
data:
  # Alertmanager Configuration
  alertmanager-auth-type: "none"  # Options: "none", "basic", "bearer"

  # Jira Configuration
  jira-project-key: "YOUR-PROJECT-KEY"

  # Sync Configuration
  sync-expiry-threshold-hours: "24"
  sync-extension-duration-hours: "168"
  sync-default-silence-duration-hours: "168"
  sync-check-alerts: "true"
```

Set `alertmanager-auth-type` to:
- `"none"` - No authentication (default)
- `"basic"` - Basic authentication (requires `alertmanager-username` and `alertmanager-password` in secret)
- `"bearer"` - Bearer token authentication (requires `alertmanager-bearer-token` in secret)

### 4. Deploy with Kustomize

```bash
kubectl apply -k deployments/
```

Or apply manifests individually:

```bash
kubectl apply -f deployments/serviceaccount.yaml
kubectl apply -f deployments/configmap.yaml
kubectl apply -f deployments/cronjob.yaml
```

### 5. Update CronJob Image

Update the image in `deployments/cronjob.yaml` to point to your container registry:

```yaml
containers:
- name: silence-manager
  image: your-registry/silence-manager:latest
```

## Usage

### Creating Linked Silences and Tickets

To link a silence with a ticket, include the ticket reference in the silence comment using the format:

```
Ticket: PROJECT-123
<additional comment>
```

The synchronizer will automatically extract the ticket reference and manage the silence accordingly.

### Manual Trigger

To manually trigger a sync run for testing:

```bash
kubectl create job --from=cronjob/silence-manager manual-sync-1 -n monitoring
```

### View Logs

```bash
# View CronJob logs
kubectl logs -l app=silence-manager -n monitoring --tail=100

# View specific job logs
kubectl logs job/silence-manager-<timestamp> -n monitoring
```

## How It Works

### Synchronization Logic

The synchronizer runs on a schedule (default: every 15 minutes) and performs the following:

1. **Retrieve all active silences** from Alertmanager
2. **For each silence with a ticket reference**:
   - Fetch the associated ticket from Jira
   - **If ticket is resolved**: Delete the silence
   - **If ticket is open and silence expires soon**: Extend the silence
   - **If ticket is open and silence has expired**: Extend the silence
3. **Check for refired alerts** (if enabled):
   - Retrieve all active alerts from Alertmanager
   - **If an alert has a ticket reference and the ticket is closed**: Reopen the ticket and create a new silence

### Ticket-Silence Coupling

The coupling between silences and tickets is maintained through:
- Silence comments contain ticket references: `Ticket: PROJECT-123`
- Ticket descriptions contain silence references: `Silence: <silence-id>`

## Extending the Application

### Adding a New Ticket System

1. Implement the `ticket.TicketSystem` interface in `pkg/ticket/`
2. Add configuration for the new system in `pkg/config/`
3. Update `cmd/silence-manager/main.go` to instantiate the new implementation

### Adding a New Alertmanager Implementation

1. Implement the `alertmanager.AlertManager` interface in `pkg/alertmanager/`
2. Add configuration for the new system in `pkg/config/`
3. Update `cmd/silence-manager/main.go` to instantiate the new implementation

## Troubleshooting

### Silence Not Being Extended

- Check that the ticket reference is in the correct format in the silence comment
- Verify the ticket exists and is accessible with the provided credentials
- Check the logs for any errors

### Ticket Not Being Reopened

- Ensure `SYNC_CHECK_ALERTS` is set to `true`
- Verify alerts have the `ticket` label set
- Check that the Jira workflow allows transitions from the ticket's current state

### Authentication Errors

- Verify Jira API token is valid
- Ensure the username matches the API token owner
- Check that the user has appropriate permissions in the Jira project

## Development

### Running Tests

```bash
go test ./...
```

### Running Locally

```bash
# Set environment variables
export JIRA_URL="https://yourcompany.atlassian.net"
export JIRA_USERNAME="admin@example.com"
export JIRA_API_TOKEN="your-api-token"
export JIRA_PROJECT_KEY="OPS"
export ALERTMANAGER_URL="http://localhost:9093"

# Run the application
go run ./cmd/silence-manager
```

## License

[Add your license here]

## Contributing

[Add contribution guidelines here]
