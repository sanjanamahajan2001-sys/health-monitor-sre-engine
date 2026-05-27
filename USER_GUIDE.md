# Export latest incident (uses HEALTH_MONITOR_EXPORT_PATH, defaults to /tmp)
health-monitor incident export --format markdown

# Export specific incident (with data-dir)
health-monitor incident export --format markdown --id INC-20260204-172300 --data-dir /var/lib/health-monitor

# Health-Monitor User Guide

Complete guide for installing, configuring, and using the Health-Monitor agent with multi-profile support.

## Table of Contents

1. [Overview](#overview)
2. [Installation](#installation)
3. [Multi-Profile Configuration](#multi-profile-configuration)
4. [Profile Management](#profile-management)
5. [Usage](#usage)
6. [Postmortem Reports](#postmortem-reports)
7. [Team-Based Operations](#team-based-operations)
8. [AlertManager Integration](#alertmanager-integration)
9. [SLO Monitoring](#slo-monitoring)
10. [Auto-Update Setup](#auto-update-setup)
11. [Troubleshooting](#troubleshooting)
12. [Feedback & Community](#feedback--community)
13. [Predictive Incident Prevention](#predictive-incident-prevention)
14. [Collaborative SRE Sessions (Tunnels)](#collaborative-sre-sessions-tunnels)

---

## Overview

Health-Monitor is a lightweight, read-only system health check tool with an interactive terminal UI and multi-profile support. It provides:

- **System Metrics**: Disk, Memory, CPU, GPU, Load Average, Uptime
- **Health Checks**: SSH access, Disk I/O, Zombie processes
- **Optimization Suggestions**: Cleanup recommendations
- **Multi-Profile Support**: Isolated configurations for different environments/teams
- **Incident Management**: Profile-isolated incident creation and tracking
- **Alert Integration**: Team-specific alert listeners and webhooks
- **Auto-Updates**: Automatic version updates from cloud storage

### Features

- ✅ Interactive scrollable TUI with profile awareness
- ✅ Real-time system metrics
- ✅ Multi-profile configuration isolation
- ✅ Team-based incident management
- ✅ Profile-specific alert listeners
- ✅ AlertManager webhook support
- ✅ SLO Monitoring: Proactive flow-based breach detection
- ✅ Interactive Feedback: Star-based ratings and notes with premium TUI
- ✅ Automatic updates
- ✅ Zero configuration (works out of the box)
- ✅ Read-only (no system changes)
- ✅ Complete state isolation per profile

---

## Initialization Commands

Health-Monitor provides two main initialization methods: `--init` for dynamic discovery and `--wizard-team` for preset configurations.

### Quick Start Commands

#### For EKS Clusters (Recommended)
```bash
# IMPORTANT: Use -E to preserve AWS environment variables
sudo -E ./health-monitor --init --infra eks --kubeconfig ~/.kube/config

# Create profile with specific name
sudo -E ./health-monitor --init --infra eks --kubeconfig ~/.kube/config --profile my-eks-cluster
```

#### For Local Kubernetes
```bash
sudo ./health-monitor --init --infra kubernetes --kubeconfig ~/.kube/config
```

#### For Team Presets
```bash
sudo ./health-monitor --wizard-team
```

---

## `--init` vs `--wizard-team`: Key Differences

### `--init` (Dynamic Discovery)

**Purpose**: Scans your actual infrastructure and creates configurations based on discovered services and metrics using an adaptive discovery engine.

**When to Use**:
- First-time setup with existing infrastructure
- When you want configurations based on actual services
- For production environments with custom metrics
- When you need accurate service discovery

**What It Does**:
1. **Discovers Kubernetes Services**: Scans all namespaces for services.
2. **Adaptive Metric Probing**: Queries Prometheus active series (`/api/v1/series`) to find which metrics are actually reporting for each service.
3. **Intelligent Label Detection**: Automatically probes for the correct service identifier labels (e.g., `app`, `job`, `container`, `service`).
4. **Tests Metric Validity**: Validates that queries return data and calculates baselines.
5. **Generates Working Flows**: Creates SLO flows based on discovered metrics.
6. **Creates Profile Configuration**: Builds config based on your infrastructure.

**Example Output**:
```
=== Service Discovery Complete ===
Services discovered: 21
Flows generated: 7
Services skipped: 14

Generated flows for: cart, frontend, payment, currency, shipping, kafka, postgresql
```

**Generated Files**:
- `/etc/health-monitor/profiles/{profile-name}.yaml`
- `/etc/health-monitor/flows.d/{profile-name}.yaml`
- `/etc/health-monitor/alerts.d/{profile-name}.yaml`
- `/etc/health-monitor/state/{profile-name}/`

**Advantages**:
- ✅ Accurate service discovery
- ✅ Real metric patterns
- ✅ Working queries validated
- ✅ Production-ready configurations
- ✅ No manual metric specification needed

**Limitations**:
- ❌ Requires access to infrastructure
- ❌ Takes longer to complete
- ❌ Depends on metric availability

### `--wizard-team` (Preset Configuration)

**Purpose**: Uses predefined templates and configurations for quick setup.

**When to Use**:
- Quick demonstrations
- Testing and development
- Standardized team setups
- When infrastructure isn't available
- For consistent team configurations

**What It Does**:
1. **Uses Preset Templates**: Predefined service configurations
2. **Standard Metrics**: Common metric patterns (`http_requests_total`, etc.)
3. **Quick Setup**: No discovery required
4. **Team Standards**: Consistent configurations across teams

**Example Presets**:
- Microservices with standard metrics
- E-commerce platforms
- SaaS applications
- API gateways

**Generated Files**:
- Same file structure as `--init`
- Uses preset metric patterns
- Standard SLO configurations

**Advantages**:
- ✅ Fast setup (seconds vs minutes)
- ✅ No infrastructure access needed
- ✅ Consistent team standards
- ✅ Good for demos/testing
- ✅ Predictable configurations

**Limitations**:
- ❌ May not match your actual metrics
- ❌ Generic configurations
- ❌ May require manual adjustments
- ❌ Less accurate for custom setups

---

## Environment Variables and AWS Integration

### Critical for EKS: `-E` Flag

When working with EKS clusters, **always use the `-E` flag** to preserve AWS environment variables:

```bash
# ❌ WRONG - AWS credentials lost
sudo ./health-monitor --init --infra eks

# ✅ CORRECT - AWS credentials preserved
sudo -E ./health-monitor --init --infra eks
```

### Required AWS Environment Variables

```bash
# Set these before running health-monitor
export AWS_REGION=us-west-2
export AWS_ACCESS_KEY_ID=your-access-key
export AWS_SECRET_ACCESS_KEY=your-secret-key
export AWS_SESSION_TOKEN=your-session-token  # if using temporary credentials

# Then run with -E
sudo -E ./health-monitor --init --infra eks --kubeconfig ~/.kube/config
```

### AWS IAM Permissions

Required permissions for EKS integration:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "eks:DescribeCluster",
                "eks:ListClusters",
                "eks:ListNodegroups",
                "cloudwatch:GetMetricData",
                "cloudwatch:ListMetrics"
            ],
            "Resource": "*"
        }
    ]
}
```

---

## Configuration Editing Guide

Both `--init` and `--wizard-team` generate editable configuration files. Here's how to customize them:

### Profile Configuration

**Location**: `/etc/health-monitor/profiles/{profile-name}.yaml`

**Key Sections to Edit**:

```yaml
name: my-eks-cluster
provider: eks
config:
  prometheus_url: "http://localhost:9090"        # Change if needed
  prometheus_token: ""                          # Add if auth required
  grafana_trace_ds: "Tempo"                     # or "Jaeger"
  
  # Adaptive Discovery Overrides (Optional)
  prometheus_metric_map:
    payment_requests: "custom_payment_count"
    payment_latency: "custom_payment_latency_bucket"
  
  prometheus_label_map:
    payment: "my_custom_service_label"

  eks:
    enabled: true
    region: "us-west-2"                         # Update your region
    kubeconfig: "/home/user/.kube/config"       # Update path
    namespaces: ["default", "production"]       # Specify namespaces
```

### Flow Configuration

**Location**: `/etc/health-monitor/flows.d/{profile-name}.yaml`

**Customizing SLOs**:

```yaml
flows:
  my-service:
    name: "My Service Custom Flow"
    services: ["my-service"]
    slos:
      - id: "custom_success"
        service: "my-service"
        objective: 99.5                          # Adjust objective
        window: "5m"                            # Adjust window
        type: "ratio"
        error_query: "sum(rate(my_requests_total{status!~\"2..\"}[5m]))"
        total_query: "sum(rate(my_requests_total[5m]))"
```

### Alert Configuration

**Location**: `/etc/health-monitor/alerts.d/{profile-name}.yaml`

**Custom Notifications**:

```yaml
alerts:
  rules:
    - name: "High Error Rate"
      condition: "error_rate > 0.05"
      duration: "5m"
      severity: "critical"
      notifications:
        slack:
          webhook_url: "https://hooks.slack.com/your-webhook"
          channel: "#alerts"
        pagerduty:
          routing_key: "your-routing-key"
```

---

## Choosing the Right Initialization Method

### Use `--init` When:

- ✅ You have existing infrastructure
- ✅ You need accurate service discovery
- ✅ Your metrics have custom patterns
- ✅ You're setting up production monitoring
- ✅ You want validated working queries

### Use `--wizard-team` When:

- ✅ You need quick setup for demos
- ✅ You're testing the application
- ✅ Your team uses standard metrics
- ✅ Infrastructure isn't accessible yet
- ✅ You want consistent team configurations

### Hybrid Approach

1. **Start with `--wizard-team`** for quick setup
2. **Test and validate** the configuration
3. **Run `--init`** for production accuracy
4. **Merge configurations** manually if needed

---

## Troubleshooting Initialization

### Common Issues

#### AWS Credentials Not Found
```bash
# Error: "AWS credentials not found"
# Solution: Ensure AWS credentials are set and use -E
export AWS_REGION=us-west-2
export AWS_ACCESS_KEY_ID=your-key
export AWS_SECRET_ACCESS_KEY=your-secret
sudo -E ./health-monitor --init --infra eks
```

#### Kubernetes Connection Failed
```bash
# Error: "Failed to connect to Kubernetes"
# Solution: Check kubeconfig path and permissions
kubectl cluster-info
sudo -E ./health-monitor --init --infra eks --kubeconfig /full/path/to/config
```

#### Prometheus Connection Failed
```bash
# Error: "Prometheus connection failed"
# Solution: Verify Prometheus URL and accessibility
curl http://localhost:9090/api/v1/query?query=up
```

#### No Services Discovered
```bash
# Issue: "Services discovered: 0"
# Solutions:
# 1. Check namespaces: kubectl get namespaces
# 2. Check services: kubectl get services --all-namespaces
# 3. Verify Prometheus has metrics: curl http://localhost:9090/api/v1/label/__name__/values
```

### Debug Mode

Add debug output for troubleshooting:

```bash
# Enable debug logging
sudo -E ./health-monitor --init --infra eks --debug
```

---

## Installation

### Quick Installation

```bash
# Download latest binary
curl -LO https://pub-0cf86c67f6dd456087607707a5a37f73.r2.dev/health-monitor-linux-amd64-v0.3

# Make executable
chmod +x health-monitor-linux-amd64-v0.3

# Install to system path
sudo mv health-monitor-linux-amd64-v0.3 /usr/local/bin/health-monitor

# Verify installation
health-monitor --version
```

### Manual Installation

1. Download the binary for your architecture
2. Make it executable: `chmod +x health-monitor-linux-amd64-v{VERSION}`
3. Move to a directory in your PATH: `/usr/local/bin/` or `~/bin/`
4. Test: `health-monitor --version`

---

## Multi-Profile Configuration

Health-Monitor supports multiple configuration profiles for complete isolation between different environments, teams, or use cases. Each profile has its own:

- **Configuration**: Separate YAML files with different URLs, services, and settings
- **State Directory**: Isolated incidents, alerts, history, and caches
- **Alert Listeners**: Independent webhook endpoints and notification channels
- **TUI Settings**: Profile-aware configuration editing

### Profile Directory Structure

```
/etc/health-monitor/
├── default.yaml              # Default profile configuration
├── profiles/                  # Additional profiles directory
│   ├── alpha-us.yaml         # Alpha US environment
│   ├── beta-eu.yaml          # Beta EU environment
│   └── core-platform.yaml    # Core platform environment
├── .active-profile           # Current active profile (persisted)
└── state/                     # Profile state directories
    ├── default/               # Default profile state
    ├── alpha-us/              # Alpha US profile state
    ├── beta-eu/               # Beta EU profile state
    └── core-platform/        # Core platform state
        ├── incidents/         # Profile-specific incidents
        ├── alerts/            # Profile-specific alerts
        └── history/           # Profile-specific history
```

### Creating Profiles

#### Method 1: Interactive Setup Wizard (--init) (Recommended)

The easiest and most robust way to create a fully functional profile is using the interactive setup wizard. It automatically configures metrics, logs, traces, alerts, and notifications while verifying connectivity to your backends.

```bash
# Launch the interactive setup wizard
sudo ./health-monitor --init
```

**The Wizard will guide you through:**
1. **Profile Naming**: Choose a name for your environment
2. **Backend Configuration**: Set up Prometheus, Loki, Grafana, and Tempo URLs
3. **Connectivity Testing**: Automatically verifies your URLs and Tokens work
4. **Service Discovery**: Automatically detects your services and generates default flow maps
5. **Alerts & Notifications**: Configures Slack and PagerDuty integrations securely
6. **Smart Defaults**: Generates default `alerts.yaml` and `flows.d/<profile>.yaml` specific to your detected services

#### Method 2: Manual Profile Creation

```bash
# Create a new profile file
sudo nano /etc/health-monitor/profiles/production.yaml

# Profile template (copy and modify)
name: production
config:
    prometheus_url: http://prod-prometheus.company.com:9090
    loki_url: http://prod-loki.company.com:3100
    grafana_url: http://prod-grafana.company.com:3001
    trace_url: http://prod-tempo.company.com:3200
    api_service: production_service
    api_route: /api/v1
    service_label: service
    route_label: route
    latency_metric: http_request_duration_seconds
    request_count_metric: http_requests_total
    error_label: status
    error_regex: 5..
    auto_discover: true
    latency_threshold_seconds: 1
    top_endpoints: 5
    window: 5m
    notifications:
        enabled: true
        slack:
            enabled: true
            webhookurl: https://hooks.slack.com/services/YOUR/PROD/WEBHOOK
        pagerduty:
            enabled: true
            routingkey: YOUR_PROD_ROUTING_KEY
    # ... other configuration options
```

#### Method 3: Using Profile Create Command

```bash
# Create a new profile based on existing one
sudo ./health-monitor profile create --name production --from default

# Create a new profile with default configuration
sudo ./health-monitor profile create --name staging
```

### Profile Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `HEALTH_MONITOR_PROFILE` | Current profile name | `production` |
| `HEALTH_MONITOR_PROFILE_PATH` | Explicit profile file path | `/etc/health-monitor/custom.yaml` |
| `HEALTH_MONITOR_CONFIG_DIR` | Override config directory | `/custom/config/path` |

---

## Sudo vs. User-Mode Execution

Health-Monitor is designed to be safe for both `sudo` (root) and regular user execution. However, **`sudo` is strongly recommended for production environments**.

### Why use `sudo`? (Recommended)

1.  **State Persistence**: By default, `sudo` uses `/etc/health-monitor/` and `/var/lib/health-monitor/`. These paths are persistent across user sessions and ideal for team-shared monitoring.
2.  **Infrastructure Access**: Discovering Kubernetes metadata or querying local container runtimes often requires elevated privileges to read system-level configurations (e.g., `/etc/kubernetes/admin.conf`).
3.  **Privileged Metrics**: Some health checks (like raw socket probes or deep system diagnostics) require root to provide accurate data.
4.  **Team Collaboration**: Using system-wide paths ensures all team members see the same incident history and SLO configurations, avoiding "isolated" data in individual home directories.

### System Mode (Sudo) - **Recommended**
- **Base Directory**: `/etc/health-monitor/`
- **Data Directory**: `/var/lib/health-monitor/`
- **Use Case**: Production clusters, shared team profiles, system-wide alert listeners.
- **Permission**: Requires `sudo` to create or modify system-wide state.

### Local Mode (User)
- **Base Directory**: `~/.health-monitor/`
- **Use Case**: Private development, quick local testing, read-only exploration.
- **Permission**: Does not require `sudo`.

> [!TIP]
> **Better Usage**: Always use `sudo` when setting up a persistent monitoring node. This ensures that the agent can survive reboots (via systemd) and that incident data is safely persisted in `/var/lib`.

---

## Profile Management

### Listing Profiles

```bash
# List all available profiles
sudo ./health-monitor profile list

# Output:
# Available profiles:
#   alpha-us
#   beta-eu
#   core-platform
#   default (active)
```

### Switching Profiles

#### Persistent Profile Switching

```bash
# Switch to a profile (persists across commands)
sudo ./health-monitor profile switch production

# Verify current profile
sudo ./health-monitor profile current

# Output:
# Current active profile: production
```

#### Temporary Profile Usage

```bash
# Use a profile for a single command (doesn't change persisted profile)
sudo ./health-monitor --profile staging profile show

# Use profile for incident management
sudo ./health-monitor --profile production incident list

# Use profile for TUI
sudo ./health-monitor --profile beta-eu
```

### Profile Information

```bash
# Show current profile configuration
sudo ./health-monitor profile show

# Show specific profile configuration
sudo ./health-monitor --profile production profile show

# Validate profile configuration
sudo ./health-monitor profile validate
sudo ./health-monitor --profile staging profile validate

### Configuration & Flow Validation

Ensure your configurations are production-ready before launching daemons or switching profiles.

#### 1. Validate Flows
Check for syntax errors, invalid SLO objectives, or malformed time windows in your `flows.yaml` or `flows.d/*.yaml` files.
```bash
sudo ./health-monitor flow validate
sudo ./health-monitor --profile prod-eks flow validate
```
*Tip: If run without sudo, it will warn you about permission issues for system-wide configs.*

#### 2. Validate Alert Rules
Verify your alert definitions, rule syntax, and notification integrations.
```bash
sudo ./health-monitor alert validate-config
```

#### 3. List Alert Rules
View all active alert rules for the current profile in a condensed format.
```bash
sudo ./health-monitor alert list-rules
```

#### 4. Validate System Config
Check general settings like Prometheus/Loki URLs and observability lookback periods.
```bash
sudo ./health-monitor config validate
```
```

### Profile Workflow Examples

#### Development Workflow

```bash
# Switch to development profile
sudo ./health-monitor profile switch dev

# All subsequent commands use dev profile
sudo ./health-monitor profile show          # Shows dev config
sudo ./health-monitor incident list         # Shows dev incidents
sudo ./health-monitor alert status          # Checks dev alert listener
sudo ./health-monitor                       # Launches TUI with dev config

# Temporarily use production for comparison
sudo ./health-monitor --profile production incident list

# Still in dev profile
sudo ./health-monitor profile current        # Shows: dev
```

#### Team-Based Workflow

```bash
# Team Alpha switches to their profile
sudo ./health-monitor profile switch alpha-us

# Team member works with their isolated environment
sudo ./health-monitor incident start --service billing_api --severity P1 --title "Alpha US Issue"
sudo ./health-monitor incident list

# Team Beta works independently
sudo ./health-monitor --profile beta-eu incident start --service auth_service --severity P2 --title "Beta EU Issue"
sudo ./health-monitor --profile beta-eu incident list
```

### Reliability Scorecard

Generate an interactive monthly review of your team's reliability metrics.

```bash
# Launch the interactive scorecard TUI
sudo health-monitor scorecard

# Skip profile selection and launch for a specific profile
sudo health-monitor scorecard --profile test-eks

# Launch the Organization-Level Scoreboard (Aggregated view)
sudo health-monitor scorecard --org

# Filter Organization view by environment
sudo health-monitor scorecard --org --env prod
```

#### Team Scorecard Features:
- **Availability & SLOs**: View success rates across all monitored flows.
- **Operational Efficiency**: Track MTTR (Mean Time To Resolve) and MTTD (Mean Time To Detect) trends.
- **Toil & Capacity**: Monitor manual operational effort against team targets (e.g., target <50% toil).
- **Trend Analysis**: Compare current metrics against the previous month with visual indicators (📈/📉).
- **ML Insights**: View AI-detected failure patterns and recurring issues.

#### Organization-Level Features (Org View 2.0):
- **Executive Summary**: High-level reliability scorecard with global SLO compliance and incident velocity.
- **Global Burn Rate**: Real-time tracking of error budget exhaustion across the entire fleet.
- **Organizational Hotspots**: Automatically identifies the top at-risk services across all teams.
- **Profile Health Index**: A ranked leaderboard of all teams/profiles based on their reliability score.
- **MTTA & MTTR**: Professional SRE metrics for Acknowledge and Recovery times at scale.
- **Global Toil Index**: Accurate % of organizational capacity spent on manual work, weighted by team size.

**Exporting**: Press `d` in the TUI to download the report as Markdown. In `--org` mode, this generates a comprehensive organizational state-of-the-union report.

---

## Collaborative SRE Sessions (Tunnels)

The Collaborative Tunnel feature allows multiple SREs to join a single incident investigation session from different terminals or systems. This is ideal for war-rooms, handovers, and pair-debugging.

### Starting a Collaborative Session

To enable collaborative mode, use the `--collaborative` flag when starting or viewing an incident:

```bash
# Start an incident with a tunnel enabled
sudo health-monitor incident start --service payment --title "Database Saturation" --collaborative

# Enable tunnel for an existing incident (Full TUI mode)
sudo health-monitor incident view --tui --collaborative --id INC-20260323-074321

# Enable tunnel with a specific port
sudo health-monitor incident view --tui --collaborative --ssh-port 9022 --id INC-20260323-074321
```

### The Connection Banner
When collaborative mode is active, the Host TUI displays a prominent **header banner** with:
- **Host IP**: The auto-discovered public or primary network IP of the host machine.
- **Port**: The active SSH port (default 9022).
- **Join Token**: A secure 12-character token required for guests to authenticate.
- **Join Command**: A ready-to-copy `ssh` command for teammates.

### Security and Authentication
- **Token-Based Access**: Collaborative sessions are protected by a secure **Join Token**. When a guest connects via SSH, they are prompted for a password—enter the **Join Token** shown on the host's screen.
- **Single Source of Truth**: All actions taken by tunnel guests are processed by the host's agent and stored directly in the host's incident files. SSH provides full end-to-end encryption.
- **Safe Transmission**: Only the terminal UI and command interactions are transmitted. No raw data files are exposed over the network.

### Connectivity & NAT Traversal
If your host machine is a public server (like EC2), teammates can connect directly using the public IP. However, if your host machine is **behind a router/NAT** (local laptop), you must use a **Reverse SSH Tunnel**.

#### Guide: Hosting from a Local Machine (Behind NAT)
If you want to share a session from your local laptop with a colleague on a remote server:

1.  **Start the session locally**:
    ```bash
    sudo health-monitor incident view --tui --collaborative --ssh-port 9022 --id <INCIDENT_ID>
    ```
2.  **Create a Reverse Tunnel** (from a new local terminal):
    ```bash
    # Replace <REMOTE_USER>@<REMOTE_HOST> with your EC2/Server details
    ssh -R 9022:localhost:9022 <REMOTE_USER>@<REMOTE_HOST>
    ```
3.  **Guest Connects**: Your colleague on the remote server can now join by running:
    ```bash
    ssh localhost -p 9022
    ```
    *(They will be prompted for the **Join Token** displayed on your laptop)*

#### Advanced: Manual Host Override
If the automatic public IP discovery is incorrect for your network environment, use the `--ssh-host` flag to force a specific IP or hostname in the connection header:
```bash
sudo health-monitor incident view --collaborative --ssh-host my-custom-vpn-ip.com
```

### Participant Actions
Tunnel guests have a dedicated command-driven interface to interact with the incident:
- `view`: Refresh the incident state and timeline.
- `note`: Add a progress update or theory.
- `ack`: Acknowledge the incident as a guest.
- `resolve`: Trigger the 15-step resolution wizard.
- `action add`: Create a new follow-up action item.
- `suggest`: Get AI-powered remediation suggestions.
- `similar`: Find matching historical incidents.


### Toil Tracking

Track and analyze manual operational work spent on incidents to identify automation opportunities.

#### Recording Toil
Toil is recorded during incident resolution. You can specify it via flags or the interactive prompt.

```bash
# Record toil during resolution
sudo health-monitor incident resolve --summary "Restored DB" --toil-minutes 45 --toil-category manual_restart
```

**Valid Toil Categories**: `manual_restart`, `alert_ack`, `config_change`, `manual_scaling`, `database_maintenance`, `security_patch`, `unknown`.

#### Analyzing Toil

View toil analytics and ROI suggestions for automation.

```bash
# List all toil for the last 30 days (Organization-wide)
sudo health-monitor toil list

# Generate a Leadership-Ready Strategic Report
sudo health-monitor toil report

# Filter by a specific profile
sudo health-monitor toil --profile test-eks list

# Filter by team and window
sudo health-monitor toil --team core-platform list --window 90d
```

**ROI Recommendations & Leadership Reports**:
The `toil list` command identifies high-toil categories and provides specific recommendations for automation, estimating potential monthly savings in hours and dollars.

The `toil report` command generates a **Leadership Brief** summarizing the financial impact of operational waste, projected annual savings, and the top 3 automation priority areas across the entire organization. Reports are automatically saved to `/etc/health-monitor/state/reports/` for archival.

---

## Guided Interactive Tour (`health-monitor guide`)

The `guide` command launches a terminal-native, interactive onboarding and training system. It's designed to help everyone from first-time users to advanced SREs master the platform's capabilities through guided scenarios and live troubleshooting.

### Features

- **🆕 Novice Onboarding**: Step-by-step guidance for initializing profiles (dynamic vs. wizard), connecting backends, and running your first health checks.
- **🧪 SRE Training Labs**: Practice solving real-world reliability mysteries in a safe sandbox. Scenarios include:
    - **The Label Mystery**: Fix "No Data" issues caused by metric label drift.
    - **Cascading Failure**: Trace root causes through complex service dependencies.
    - **Incident Replay**: Practice resolving historical high-severity outages (e.g., INC-742).
- **📚 Command Encyclopedia**: A searchable, classified reference for every command and global flag in the system—organized by use-case (Setup, Incident, Admin).
- **🛠️ Dynamic Troubleshooter**: Proactive diagnostics for your current environment. It analyzes:
    - **Connectivity & Auth**: Probes Prometheus and Loki for reachability and authentication.
    - **Daemon Vitality**: Verifies that `alert-listen` and `slo monitor` background processes are healthy.
    - **Profile Drift**: Identifies missing configurations or deviations from standard templates.
    - **Interactive Fixes**: Apply one-click remediations directly from the UI (e.g., fixing permissions or updating URLs).

### Usage

```bash
# Launch the interactive guide
sudo ./health-monitor guide

# Search within the guide
# Press '/' to open the search bar and find specific commands or topics.

# Navigation:
# - ENTER: Advance to next step / execute action
# - ESC / B: Go back to category list
# - H: Toggle contextual hints
# - F: Apply suggested fix (in Troubleshooter)
```

---

## Configuration

### Environment Variables

| Variable | Default | Description | Required |
|----------|---------|-------------|----------|
| `UPDATE_BASE_URL` | `https://pub-0cf86c67f6dd456087607707a5a37f73.r2.dev` | Base URL for auto-updates | No (optional) |
| `VERSION_URL` | (derived from UPDATE_BASE_URL) | Direct URL to version.txt | No (optional) |
| `HEALTH_MONITOR_INCIDENTS_PATH` | `~/.health-monitor/incidents` | Custom incident storage directory | No (optional) |
| `HEALTH_MONITOR_DATA_DIR` | `/var/lib/health-monitor` | Base data dir (incidents + alert history) | No (optional) |

**Note:** Environment variables are **optional**. The agent works out-of-the-box with the default URL. You only need to set `UPDATE_BASE_URL` if:
- You want to use a different storage domain than the default
- You're migrating to a new storage domain
- You want to override the default for testing

### Setting Environment Variables (Optional)

**For current session:**
```bash
export UPDATE_BASE_URL="https://your-bucket.r2.dev"
export HEALTH_MONITOR_DATA_DIR="/var/lib/health-monitor"
```

**For current user (permanent):**
```bash
echo 'export UPDATE_BASE_URL="https://your-bucket.r2.dev"' >> ~/.bashrc
source ~/.bashrc
```

**For all users (system-wide):**
```bash
sudo sh -c 'echo "UPDATE_BASE_URL=https://your-bucket.r2.dev" >> /etc/environment'
```

### Secret Management (Secret Provider)

Health-Monitor features a flexible **Secret Provider** system that abstracts how sensitive information (tokens, passwords) is retrieved. This allows for both secure local file storage and modern environment-based secret injection.

#### How it Works

The Secret Provider tries to resolve secrets in the following order:
1.  **Environment Variables**: Prefixed with `HM_SECRET_`.
2.  **Local Files**: Searched in system paths and user home directories (context-aware for `sudo`).

#### Using Environment-based Secrets

You can avoid creating files by exporting secrets directly:

```bash
# Token for Prometheus
export HM_SECRET_PROMETHEUS_TOKEN="your-secret-token"

# Basic Auth (format: user:pass)
export HM_SECRET_PROMETHEUS_BASIC="admin:password123"

# Other components
export HM_SECRET_LOKI_TOKEN="loki-token"
export HM_SECRET_GRAFANA_TOKEN="grafana-token"
```

#### Secret Storage Locations (Files)

If environment variables are not set, the system looks for these files:

| Secret Key | File Name | `HM_SECRET_` Mapping |
|------------|-----------|--------------------|
| `prometheus.token` | `prometheus.token` | `HM_SECRET_PROMETHEUS_TOKEN` |
| `prometheus.basic` | `prometheus.basic` | `HM_SECRET_PROMETHEUS_BASIC` |
| `loki.token` | `loki.token` | `HM_SECRET_LOKI_TOKEN` |

**Search Paths:**
- **System**: `/etc/health-monitor/`
- **User**: `~/.health-monitor/`
- **Sudo**: When running with `sudo`, the system also checks the original user's home directory.

#### Running with Secrets

No special flags are required. The agent automatically uses the Secret Provider for all internal configuration loading.

```bash
# Simply run the agent
health-monitor
```

### Operational Resilience (Circuit Breaker & Retries)

To ensure the agent remains stable even when external notification services (Slack, PagerDuty, Jira) are down or slow, Health-Monitor includes a built-in resilience middleware.

#### Features

- **Circuit Breaker**: Automatically "trips" and stops sending notifications to a failing provider if a threshold of errors is reached. This prevents the agent from hanging while waiting for timeouts.
- **Exponential Backoff**: When a transient error occurs, the agent will retry with increasing delays (e.g., 1s, 2s, 4s).

#### Configuration

Resilience is enabled by default. You can tune it in your profile:

```yaml
notifications:
    resilience:
        enabled: true
        threshold: 3              # Failures before tripping the circuit
        reset_timeout_seconds: 60 # How long to wait before trying again
        max_retries: 2            # Number of retries per notification
```

#### How it works in practice

If Slack starts returning errors 3 times in a row:
1.  The **Circuit Breaker trips** (Status: OPEN).
2.  Any further notification attempts to Slack are **immediately skipped** without blocking the agent.
3.  After 60 seconds, the circuit enters a **Half-Open** state, allowing one request through to test if the service is back.
4.  If it succeeds, the circuit **Closes** and resume normal operations.

### Semantic Analysis (ML Hints)

Health-Monitor uses a **Semantic Analysis** layer to provide diagnostic "hints" that go beyond simple rule-based matching. It uses vector embeddings and cosine similarity to identify patterns in logs and incident data.

#### Key Commands

```bash
# Get a semantic prediction for a log sample
health-monitor ml predict --text "database connection refused after 3 retries"

# Compare semantic similarity between two error messages
health-monitor ml sim --source "connection timeout" --target "db connection expired"

# View the SRE/Ops vocabulary used for feature extraction
health-monitor ml vocab
```

#### How it works

The ML agent tokenizes input text and maps it to a high-dimensional vector space using a curated vocabulary of SRE concepts. It then calculates the distance between these vectors to provide:
- **Category**: Classifies the issue (connectivity, latency, capacity, etc.).
- **Semantic Hints**: Shown alongside RCA suggestions to give more context.
- **Resolution Actions**: Recommended steps based on the identified pattern.
- **Pattern Matching**: Automatically identifying the "true" nature of a failure even if the exact string matches fail.

---

---

## Team-Based Operations

Multi-profile support enables complete team isolation for monitoring operations. Each team can work independently with their own:

- **Configuration**: Different monitoring endpoints and services
- **Incidents**: Isolated incident management per team
- **Alert Listeners**: Separate webhook endpoints and notification channels
- **State**: Independent caches, history, and operational data

### Team Setup Examples

#### Team Alpha-US Setup

```bash
# Team Alpha creates their profile
sudo ./health-monitor profile switch alpha-us

# Start their alert listener on port 9095
sudo ./health-monitor --profile alpha-us alert-listen \
    --background \
    --addr :9095 \
    --pid-file /run/health-monitor-alert-alpha.pid \
    --data-dir /var/lib/health-monitor/state/alpha-us

# Team Alpha works with their isolated environment
sudo ./health-monitor incident start --service billing_api --severity P1 --title "Alpha US Production Issue"
sudo ./health-monitor incident list
sudo ./health-monitor alert status --pid-file /run/health-monitor-alert-alpha.pid
```

#### Team Beta-EU Setup

```bash
# Team Beta works independently
sudo ./health-monitor --profile beta-eu alert-listen \
    --background \
    --addr :9096 \
    --pid-file /run/health-monitor-alert-beta.pid \
    --data-dir /var/lib/health-monitor/state/beta-eu

# Team Beta incidents are completely isolated
sudo ./health-monitor --profile beta-eu incident start --service auth_service --severity P2 --title "Beta EU Auth Issue"
sudo ./health-monitor --profile beta-eu incident list
```

### Team Isolation Benefits

1. **No Cross-Team Interference**: Team Alpha's incidents never appear in Team Beta's views
2. **Separate Notification Channels**: Each team gets their own Slack/PagerDuty integrations
3. **Different Monitoring Targets**: Teams can monitor different Prometheus/Grafana instances
4. **Independent Scaling**: Each team can scale their alert processing independently
5. **Security Isolation**: Teams only see their own incidents and configurations

### Team Coordination

```bash
# Team lead can check all team profiles
sudo ./health-monitor --profile alpha-us incident list
sudo ./health-monitor --profile beta-eu incident list
sudo ./health-monitor --profile core-platform incident list

# Compare configurations across teams
sudo ./health-monitor --profile alpha-us profile show
sudo ./health-monitor --profile beta-eu profile show
```

---

## AlertManager Integration

Health-Monitor provides complete AlertManager webhook support with profile-aware incident creation. Each profile can have its own webhook endpoint and automatically creates incidents in the correct profile's state directory.

### Webhook Configuration

#### Profile-Specific Webhook Endpoints

Each profile's alert listener provides a dedicated webhook endpoint:

```bash
# Alpha-US Team Webhook: http://localhost:9095/webhook
# Beta-EU Team Webhook: http://localhost:9096/webhook
# Core-Platform Webhook: http://localhost:9097/webhook
```

#### AlertManager Configuration

**Team Alpha-US AlertManager.yml:**
```yaml
global:
  smtp_smarthost: 'localhost:587'
  smtp_from: 'alerts@company.com'

route:
  group_by: ['alertname', 'cluster', 'service']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 1h
  receiver: 'health-monitor-alpha'

receivers:
  - name: 'health-monitor-alpha'
    webhook_configs:
      - url: 'http://localhost:9095/webhook'
        send_resolved: true
        http_config:
          basic_auth:
            username: 'alertmanager'
            password: 'password'
```

**Team Beta-EU AlertManager.yml:**
```yaml
receivers:
  - name: 'health-monitor-beta'
    webhook_configs:
      - url: 'http://localhost:9096/webhook'
        send_resolved: true
```

### Webhook Payload Format

Health-Monitor accepts standard AlertManager webhook payloads:

```json
{
  "receiver": "health-monitor-alpha",
  "status": "firing",
  "alerts": [
    {
      "status": "firing",
      "labels": {
        "alertname": "HighErrorRate",
        "service": "billing_api",
        "severity": "critical",
        "team": "alpha-us"
      },
      "annotations": {
        "summary": "High error rate detected in billing_api",
        "description": "Error rate is above 5% for the last 5 minutes"
      },
      "startsAt": "2026-02-18T09:30:00Z",
      "endsAt": "0001-01-01T00:00:00Z",
      "generatorURL": "http://prometheus:9090/graph?g0.expr=rate...",
      "fingerprint": "abc123def456"
    }
  ]
}
```

### Auto-Incident Creation

When AlertManager sends a webhook to Health-Monitor:

1. **Profile Detection**: The webhook is routed to the correct profile based on the listener port
2. **Incident Creation**: Automatically creates an incident in the profile's state directory
3. **Correlation**: Performs log and trace correlation using the profile's configuration
4. **Notifications**: Sends notifications using the profile's notification settings
5. **Isolation**: The incident is only visible within that profile

#### Webhook to Incident Mapping

```bash
# AlertManager webhook to :9095/webhook
# → Creates incident in /etc/health-monitor/state/alpha-us/incidents/
# → Uses alpha-us profile configuration for correlation
# → Sends notifications to alpha-us team's Slack/PagerDuty

# AlertManager webhook to :9096/webhook  
# → Creates incident in /etc/health-monitor/state/beta-eu/incidents/
# → Uses beta-eu profile configuration for correlation
# → Sends notifications to beta-eu team's channels
```

### Testing Webhook Integration

```bash
# Test webhook for alpha-us profile
curl -X POST http://localhost:9095/webhook \
  -H "Content-Type: application/json" \
  -d '{
    "receiver": "health-monitor-alpha",
    "status": "firing",
    "alerts": [{
      "status": "firing",
      "labels": {
        "alertname": "TestAlert",
        "service": "test_service",
        "severity": "warning"
      },
      "annotations": {
        "summary": "Test alert from alpha-us"
      }
    }]
  }'

# Check incident was created in correct profile
sudo ./health-monitor --profile alpha-us incident list
```

### Advanced Webhook Features

#### Custom Alert Routing

```yaml
# Route different alerts to different profiles
receivers:
  - name: 'health-monitor-alpha-critical'
    webhook_configs:
      - url: 'http://localhost:9095/webhook'
  - name: 'health-monitor-beta-warning'
    webhook_configs:
      - url: 'http://localhost:9096/webhook'

route:
  routes:
  - match:
      severity: critical
      team: alpha-us
    receiver: 'health-monitor-alpha-critical'
  - match:
      severity: warning
      team: beta-eu
    receiver: 'health-monitor-beta-warning'
```

#### Webhook Authentication

```yaml
receivers:
  - name: 'health-monitor-secure'
    webhook_configs:
      - url: 'http://localhost:9095/webhook'
        send_resolved: true
        http_config:
          bearer_token: 'your-webhook-token'
          basic_auth:
            username: 'webhook-user'
            password: 'webhook-password'
```

---

## Usage

### Basic Usage

```bash
# Run health check with current active profile
health-monitor

# Run health check with specific profile (temporary)
health-monitor --profile production

# Show version
health-monitor --version

# Show help
health-monitor --help

# Explore features safely with simulated data in the interactive TUI
health-monitor demo

# Explore features safely using a static CLI output instead of the TUI
health-monitor demo --force-cli

# Profile-aware basic usage
sudo ./health-monitor profile switch production    # Switch to production profile
sudo ./health-monitor                               # Uses production profile
sudo ./health-monitor --profile staging             # Temporarily use staging
sudo ./health-monitor                               # Back to production profile
```

### Profile-Aware Incident Management

Manage incidents with complete profile isolation:

```bash
# Start an incident in current active profile
sudo ./health-monitor incident start --service order_api --severity P1 --title "Checkout timeouts"

# Start an incident in specific profile (temporary)
sudo ./health-monitor --profile production incident start --service payment_api --severity P1 --title "Payment failures"

# List incidents in current profile
sudo ./health-monitor incident list

# List incidents in specific profile
sudo ./health-monitor --profile staging incident list

# Add a note to current profile's active incident
sudo ./health-monitor incident note "Scaled postgres pool 50->100"

# Add a note to specific incident in current profile
sudo ./health-monitor incident note --id INC-20260204-172300 "Investigated database logs"

# Resolve incident in current profile
sudo ./health-monitor incident resolve --summary "Fixed memory leak, redeployed service"

# Resolve incident in specific profile
sudo ./health-monitor --profile production incident resolve --id INC-20260204-172300 --summary "Production fix deployed"

# Generate a blameless postmortem report
sudo ./health-monitor incident postmortem

# Generate postmortem for a specific incident
sudo ./health-monitor incident postmortem --id INC-20260204-172300
```

### Profile-Aware Alert Management

```bash
# Start alert listener for current profile
sudo ./health-monitor alert-listen --background --addr :9095

# Start alert listener for specific profile
sudo ./health-monitor --profile alpha-us alert-listen --background --addr :9095 --pid-file /run/health-monitor-alert-alpha.pid

# Check alert status in current profile
sudo ./health-monitor alert status

# Check alert status with custom PID file
sudo ./health-monitor alert status --pid-file /run/health-monitor-alert-alpha.pid

# Check alert status for specific profile
sudo ./health-monitor --profile beta-eu alert status --pid-file /run/health-monitor-alert-beta.pid
```

### Complete Profile Workflow Examples

#### Development Team Workflow

```bash
# 1. Switch to development profile
sudo ./health-monitor profile switch dev

# 2. Start dev alert listener
sudo ./health-monitor alert-listen --background --addr :9095

# 3. Create incidents as needed
sudo ./health-monitor incident start --service auth_api --severity P2 --title "Dev auth latency"

# 4. List dev incidents
sudo ./health-monitor incident list

# 5. Check dev alert status
sudo ./health-monitor alert status

# 6. All commands use dev profile automatically
sudo ./health-monitor profile show      # Shows dev config
sudo ./health-monitor                  # TUI with dev config
```

#### Production Incident Response

```bash
# 1. Switch to production profile
sudo ./health-monitor profile switch production

# 2. Check current production incidents
sudo ./health-monitor incident list

# 3. Start new incident for production issue
sudo ./health-monitor incident start --service payment_api --severity P1 --title "Production payment failures"

# 4. Add investigation notes
sudo ./health-monitor incident note "Investigating payment gateway timeouts"
sudo ./health-monitor incident note "Payment provider reports API degradation"

# 5. Check production alert status
sudo ./health-monitor alert status

# 6. Resolve when fixed
sudo ./health-monitor incident resolve --summary "Payment provider restored, monitoring stable"
```

#### Multi-Team Coordination

```bash
# Team lead checks all team statuses
echo "=== Alpha-US Team Status ==="
sudo ./health-monitor --profile alpha-us incident list
sudo ./health-monitor --profile alpha-us alert status

echo "=== Beta-EU Team Status ==="
sudo ./health-monitor --profile beta-eu incident list
sudo ./health-monitor --profile beta-eu alert status

echo "=== Core Platform Status ==="
sudo ./health-monitor --profile core-platform incident list
sudo ./health-monitor --profile core-platform alert status

# Compare configurations
sudo ./health-monitor --profile alpha-us profile show | grep prometheus_url
sudo ./health-monitor --profile beta-eu profile show | grep prometheus_url
sudo ./health-monitor --profile core-platform profile show | grep prometheus_url
```

# Acknowledge ownership (records both system user and passed user)
health-monitor incident ack --user sanjana

# Acknowledge a specific incident
health-monitor incident ack --id INC-20260204-172300 --user sanjana

# Resolve with summary
health-monitor incident resolve --summary "DB pool exhaustion"

# Resolve a specific incident
health-monitor incident resolve --id INC-20260204-172300 --summary "DB pool exhaustion"

# Resolve with structured RCA (Root Cause Analysis) metadata
health-monitor incident resolve \
  --summary "Fixed DB connection pool" \
  --root-cause "DB connection pool exhaustion" \
  --fix "Increased pool from 50 to 100" \
  --category capacity \
  --component postgres_db \
  --dependency billing_api->postgres_db \
  --failure-type dependency

# Resolve with blameless lessons learned
health-monitor incident resolve \
  --summary "Fixed Redis timeout" \
  --root-cause "Redis connection timeout" \
  --category latency \
  --well "Monitoring caught it fast" \
  --better "The failover took too long" \
  --lucky "Happened during low traffic" \
  --downtime 5

# RCA with pattern and error signature
health-monitor incident resolve \
  --summary "Fixed" \
  --root-cause "Network timeout" \
  --category infra \
  --pattern "connection refused" \
  --error-signature "ECONNREFUSED"


# Promote a suggested incident to active
health-monitor incident promote --id INC-20260204-172300

# View the latest incident timeline (active or resolved)
health-monitor incident view

# View a specific incident by ID
health-monitor incident view --id INC-20260204-172300

# List all incidents
health-monitor incident list

# Export latest incident (uses HEALTH_MONITOR_EXPORT_PATH, defaults to /tmp)
health-monitor incident export --format markdown

# Export latest incident as JSON (automation friendly)
health-monitor incident export --format json

# Export specific incident by ID
health-monitor incident export --format markdown --id INC-20260204-172300

# Export specific incident as JSON
health-monitor incident export --format json --id INC-20260204-172300
# Export specific incident as JSON
health-monitor incident export --format json --id INC-20260204-172300
```

### Action Item Management

Track follow-up tasks directly within incidents to ensure continuous improvement and prevent recurrence.

```bash
# Add an action item (interactive)
health-monitor incident action add

# Add to a specific incident
health-monitor incident action add --id INC-20260204-172300

# List action items for an incident
health-monitor incident action list --id INC-20260204-172300

# List ALL action items across all incidents
health-monitor incident action list

# Update an action item (status, owner, etc.)
health-monitor incident action update

# Delete an action item
health-monitor incident action delete
```

**Workflow Tips:**
- **Auto-Prompt:** When you resolve an incident, the CLI will automatically ask if you want to add action items.
- **Smart Defaults:** The system intelligently suggests owners and due dates based on incident severity and history.
- **Status Tracking:** Use `TODO`, `IN_PROGRESS`, `DONE`, or `WONT_FIX` to track progress.


### RCA (Root Cause Analysis) Metadata

When resolving incidents, you can provide structured RCA metadata for better incident tracking and future similar-incident detection.

#### Available RCA Flags

| Flag | Description | Example | Required |
|------|-------------|---------|----------|
| `--root-cause` | Primary root cause | `"DB connection pool exhaustion"` | Recommended |
| `--fix` | Fix/resolution summary | `"Increased pool from 50 to 100"` | Recommended |
| `--category` | Incident category | `capacity`, `latency`, `dependency`, etc. | Recommended |
| `--component` | Affected component | `postgres_db`, `redis`, `api_gateway` | Recommended |
| `--dependency` | Dependency relationship | `billing_api->postgres_db` | Optional |
| `--failure-type` | Type of failure | `service`, `dependency`, `infra` | Optional |
| `--pattern` | Error pattern | `"connection refused"` | Optional |
| `--error-signature` | Error signature | `"ECONNREFUSED"` | Optional |
| `--well` | What went well | `"Monitoring caught it fast"` | Optional |
| `--better` | What could be better | `"The failover took too long"` | Optional |
| `--lucky` | Where we got lucky | `"Happened during low traffic"` | Optional |
| `--downtime` | Estimated downtime (min) | `15` | Optional |

#### Valid Categories

- `capacity` - Resource exhaustion (CPU, memory, connections, disk)
- `latency` - Performance degradation, slow responses
- `dependency` - External service failures
- `configuration` - Config errors, misconfigurations
- `infra` - Infrastructure issues (network, hardware)
- `deployment` - Deployment-related failures
- `unknown` - Unclassified incidents

#### Valid Failure Types

- `service` - Service-level failure
- `dependency` - Dependency failure (external service)
- `infra` - Infrastructure failure

#### Interactive Prompts

If you resolve without critical RCA fields, the system will prompt:

```
⚠  Structured RCA fields missing.

Similar-Incident detection works best with:
  • --root-cause
  • --fix
  • --category
  • --component

You can still resolve, but similarity confidence will be lower.

Continue without RCA fields? (y/n):
```

#### Auto-Extraction Suggestions

The system may suggest RCA values based on incident data:

```
💡 Suggested RCA fields (based on incident data):
  --component postgres_db
  --pattern "connection refused"
  --error-signature "connection refused"
  --dependency "billing_api->postgres_db"
  (Add these flags to your command if accurate)
```

These are **suggestions only** - you must explicitly add them to your command if accurate.

#### Validation

Invalid categories or failure types are rejected immediately:

```bash
$ health-monitor incident resolve --summary "Test" --category invalid
Resolve failed: invalid category "invalid". Allowed: capacity, latency, dependency, configuration, infra, deployment, unknown
```

#### Best Practices

1. **Always provide critical fields**: `--root-cause`, `--fix`, `--category`, `--component`
2. **Use consistent naming**: Use the same component names across incidents
3. **Be specific**: "DB connection pool exhaustion" is better than "DB issue"
4. **Review suggestions**: Auto-extracted values may not always be accurate
5. **Document dependencies**: Use `--dependency` to track service relationships

#### Viewing RCA Data

RCA metadata appears in:

1. **CLI View**:
```
Analysis
────────
Component: postgres_db
Category: capacity
Root Cause: DB connection pool exhaustion
Fix: Increased pool from 50 to 100
```

2. **Markdown Export**: Includes full Analysis section
3. **JSON Export**: Includes `analysis` object with all fields

---

### Similar-Incident Detection

The system automatically finds historical incidents similar to the current one, helping you:
- Identify recurring issues
- Learn from past resolutions
- Validate your fix against previous solutions
- Reduce MTTR (Mean Time To Resolution)

#### How It Works

The similarity engine uses **multi-factor confidence scoring**:

| Factor | Weight | Description |
|--------|--------|-------------|
| Component | 30% | Affected system component (e.g., `postgres_db`) |
| Category | 25% | Incident type (capacity, latency, etc.) |
| Pattern | 20% | Error pattern similarity |
| Dependency | 15% | Service dependency relationship |
| Error Signature | 10% | Normalized error signature |

**Time Decay:** Recent incidents score higher than old ones (30% reduction over 180 days)

**Confidence Tiers:**
- ⭐⭐⭐ 80-100%: Strong match (same component, category, pattern)
- ⭐⭐ 60-79%: Moderate match (component + category OR pattern match)
- ⭐ 40-59%: Weak match (category only OR service match)

#### Command: `incident similar`

Find similar historical incidents:

```bash
# Show similar incidents for active incident
health-monitor incident similar

# Show similar incidents for specific incident
health-monitor incident similar --id INC-20260216-060007

# Custom time window (default: 90 days)
health-monitor incident similar --days 180

# Adjust confidence threshold (default: 0.6 = 60%)
health-monitor incident similar --min-confidence 0.7

# Get more results (default: 5)
health-monitor incident similar --limit 10

# Combine options
health-monitor incident similar --days 30 --min-confidence 0.8 --limit 3
```

**Example Output:**

```
Similar past incidents (last 90d):

1) INC-20260210-143022  [10 Feb 2026 14:30:22]  ⭐⭐⭐ 92% match
   Service: billing_api
   Dependency: billing_api->postgres_db
   Pattern: postgres: query timeout
   Root cause: DB connection pool exhaustion
   Fix: Increased pool 50->100, added index on orders table
   Matched on: component, category, pattern

2) INC-20260205-091544  [05 Feb 2026 09:15:44]  ⭐⭐ 78% match
   Service: billing_api
   Dependency: billing_api->postgres_db
   Pattern: postgres: connection timeout
   Root cause: DB connection pool exhaustion
   Fix: Increased pool 30->50
   Matched on: component, category, dependency

💡 Tip: Use 'health-monitor incident view --id <ID>' to see full details
```

#### Automatic Integration

Similar incidents are shown automatically at key points:

**1. After `incident start`:**
```bash
$ health-monitor incident start --service billing_api --severity P1 --title "DB timeout"
Incident started: INC-20260216-091734
Impacts flows: Checkout

💡 Similar past incidents found:
  1) INC-20260216-055012 (⭐⭐⭐ 100%) - 3 hours ago
     Root cause: DB connection pool exhaustion
     Fix: Increased pool from 50 to 100

💡 Tip: Use 'health-monitor incident similar' for more details
```

**2. After `incident resolve`:**
```bash
$ health-monitor incident resolve --summary "Fixed" --root-cause "Pool exhaustion" --category capacity
Incident resolved: INC-20260216-091734

💡 Similar past incidents (for validation):
  1) INC-20260216-055012 (⭐⭐⭐ 95%) - 3 hours ago
     Their fix: Increased pool from 50 to 100
```

**3. In `incident view` (for resolved incidents):**
```bash
$ health-monitor incident view --id INC-20260216-055012
[... incident details ...]

💡 Similar past incidents:
  1) INC-20260213-130343 (⭐⭐ 75%) - 3 days ago
     Fix: Increased pool from 30 to 50
```

#### Confidence Scoring Details

**Strong Match (80-100%)** - High confidence, likely same root cause:
- Exact component + category + pattern match
- Exact dependency + category match
- Error signature + component + category match

**Moderate Match (60-79%)** - Good confidence, worth investigating:
- Component + category match
- Pattern similarity >60% + category match
- Same service + category + similar error

**Weak Match (40-59%)** - Lower confidence, use with caution:
- Category match only + same service
- Pattern similarity >40%
- Component match only

**Below 40%** - Not shown (too unreliable)

#### Fallback Matching

When RCA data is missing (older incidents or unclassified), the system uses:
- Service name match (40%)
- Title similarity (20%)
- Log pattern match (30%)
- Severity match (10%)

**Note:** Fallback matches are capped at lower confidence to indicate uncertainty.

#### Best Practices

1. **Always provide RCA data** when resolving incidents
   - Improves future similarity matching
   - Helps team learn from incidents
   - Enables better pattern detection

2. **Use consistent naming**
   - Same component names across incidents
   - Standardized service names
   - Clear, descriptive titles

3. **Review suggestions critically**
   - High confidence (80%+) is usually accurate
   - Medium confidence (60-79%) needs validation
   - Low confidence (<60%) use as hints only

4. **Adjust thresholds as needed**
   - Start with default (60%)
   - Lower for exploratory searches (50%)
   - Raise for high-precision needs (70-80%)

5. **Use time windows appropriately**
   - 30 days: Recent patterns only
   - 90 days: Default, good balance
   - 180 days: Long-term trend analysis

#### Troubleshooting

**No matches found:**
- Lower confidence: `--min-confidence 0.5`
- Extend time window: `--days 180`
- Check if historical incidents have RCA data
- Ensure consistent service/component naming

**Too many low-quality matches:**
- Raise confidence: `--min-confidence 0.7`
- Reduce time window: `--days 30`
- Improve RCA data quality

**Matches seem wrong:**
- Check "Matched on" field to understand why
- Review historical incident RCA data
- Consider if component names changed
- Report feedback for tuning

---

**Note on users:** Event lines include both the system user and the passed user (if provided). For example:
`admin (system: root, sudo: sanjana)` or `sanjana (system: ubuntu)`

**Note on sudo:** If you run incident commands with `sudo`, the tool now reassigns file ownership to the real user (from `SUDO_USER`) to avoid permission issues when running later without sudo.

---

## Postmortem Reports

Health-Monitor can generate high-fidelity, blameless postmortem reports from your incident data. These reports are designed to be shared with stakeholders and used for continuous improvement.

### Generating a Postmortem

```bash
# Generate for the latest incident
health-monitor incident postmortem

# Generate for a specific incident
health-monitor incident postmortem --id INC-20260226-111745
```

### Report Features
- **Directory Isolation**: Reports are saved to `/tmp/health-monitor/` by default. This can be customized using the `HEALTH_MONITOR_POSTMORTEM_PATH` environment variable.
- **Rich Formatting**: Reports include YAML frontmatter for searchability, a clickable Table of Contents, and detailed technical evidence (logs and traces).
- **Audit Trail**: Every time a postmortem is generated, a corresponding note is automatically added to the incident's timeline.
- **Smart Enrichment**: MTTR is automatically calculated, and relevant historical context from similar incidents is embedded.

---

### Data Storage

Incident data is stored at (override with `HEALTH_MONITOR_INCIDENTS_PATH` or `HEALTH_MONITOR_DATA_DIR`):

```
~/.health-monitor/incidents/
```

To use a persistent system location:

```
health-monitor incident list --data-dir /var/lib/health-monitor
```

---

### Flow Management

Flows provide service grouping and impact status. Configure flows in:

```
/etc/health-monitor/flows.yaml
/etc/health-monitor/flows.d/*.yaml
~/.health-monitor/flows.yaml
~/.health-monitor/flows.d/*.yaml
```

Example flow file:

```yaml
flows:
  checkout_flow:
    name: "Checkout"
    services: [billing_api, cart_api, mysql]
```

Commands:

```bash
# List flows
health-monitor flow list

# View a flow
health-monitor flow checkout_flow

# JSON output
health-monitor flow list --json
health-monitor flow checkout_flow --json
```

### Alert Ingestion (Alertmanager Webhook)

Alerts create incidents automatically or as suggestions. Configure alert rules in:

```
/etc/health-monitor/alerts.yaml
/etc/health-monitor/alerts.d/*.yaml
~/.health-monitor/alerts.yaml
~/.health-monitor/alerts.d/*.yaml
```

Example alert rules:

```yaml
alerts:
  - match:
      alertname: HighLatency
      service: billing_api
    severity: P1
    mode: auto
    title: "Checkout latency spike"

  - match:
      alertname: AuthErrors
      service: identity_service
    severity: P2
    mode: suggest
    title: "Auth failures spike"
```

Listener commands:

```bash
# Start listener (foreground)
health-monitor alert listen --addr :9095

# Start in background (uses systemd if unit exists, else background)
health-monitor alert listen --background --addr :9095 --pid-file /tmp/alert.pid

# Short aliases
health-monitor alert-listen --addr :9095
health-monitor alert-status --pid-file /tmp/alert.pid
health-monitor alert-stop --pid-file /tmp/alert.pid
health-monitor alert reload --pid-file /tmp/alert.pid

# Check status
health-monitor alert status --pid-file /tmp/alert.pid

# Stop listener
health-monitor alert stop --pid-file /tmp/alert.pid

# Validate config
health-monitor alert validate-config

# List rules
health-monitor alert list-rules

# Dry-run a payload
health-monitor alert test payload.json
```

Listener security:

```bash
# TLS
health-monitor alert listen --addr :9095 --tls-cert /path/cert.pem --tls-key /path/key.pem

# Bearer token
health-monitor alert listen --auth-token-file /path/token

# Basic auth
health-monitor alert listen --auth-user user --auth-pass pass
```

Operational flags:

```bash
# Persistent incident storage for alert-driven incidents
health-monitor alert listen --data-dir /var/lib/health-monitor

# Log format
health-monitor alert listen --log-format json

# Rate limit (requests per second)
health-monitor alert listen --max-rps 100

# Concurrency for alert batches
health-monitor alert listen --max-workers 4

# Reload alert rules without restart
kill -HUP $(cat /tmp/alert.pid)
health-monitor alert reload --pid-file /tmp/alert.pid
```

Metrics endpoint:

```
GET http://localhost:9095/metrics
```

Health endpoints:

```
GET http://localhost:9095/healthz
GET http://localhost:9095/readyz
```

Alert history log (JSONL):

```
/var/lib/health-monitor/history.log
```

### Config File (Optional)

You can set a default data directory in `/etc/health-monitor/config.json`:

```json
{
  "data_dir": "/var/lib/health-monitor"
}
```

### Interactive TUI

The health-monitor opens in a full-screen interactive terminal UI:

- **Scroll**: Use arrow keys (↑/↓) or `j`/`k` (vim-style)
- **Jump to top**: Press `Home` or `g`
- **Jump to bottom**: Press `End` or `G`
- **Exit**: Press `q`, `Q`, `Esc`, or `Ctrl+C`
- **Incidents**: Press `i` to view the incident timeline/history

### Output Sections

1. **Summary Cards**: Quick overview (Disk, Memory, GPU, SSH)
2. **Metrics**: Detailed system metrics
3. **/var Top Consumers**: Disk usage breakdown
4. **Optimization Suggestions**: Cleanup recommendations
5. **Banner**: Version and contact information

---

## SLO Monitoring

Health-Monitor features a proactive SLO Monitoring engine that periodically evaluates your Service Level Objectives and automatically creates incidents when thresholds are breached.

### How it Works

1.  **Proactive Evaluation**: The monitor runs in the background, querying your Prometheus/metrics source every interval.
2.  **Flow-Aware**: SLOs are grouped into "Flows" (e.g., Checkout, Authentication) to provide business-level context.
3.  **Automated Incident Creation**: When an SLO breaches (e.g., Error Rate > 5%), the monitor creates a "Suggested" incident.
4.  **Consolidation (Anti-Spam)**: If an **Active** incident already exists for a service, new SLO breaches or alerts for that service are attached as **detailed notes** to the existing incident instead of creating new duplicates.
5.  **Freshness Check**: "Suggested" incidents older than 4 hours are ignored for deduplication, ensuring new issues aren't buried in stale data.

### Flow Configuration

Flows are defined in profile-specific YAML files located in `/etc/health-monitor/flows.d/<profile_name>.yaml`.

**Example: `/etc/health-monitor/flows.d/core-platform.yaml`**
```yaml
flows:
  checkout:
    name: Checkout Flow
    services: [billing_api, payments_service]
    slos:
      - id: checkout_success
        service: billing_api    # Maps breach to billing_api alert rules
        objective: 95            # 95% success rate
        window: 5m               # 5-minute evaluation window
        type: ratio
        error_query: http_requests_total{service="billing_api",status!="OK"}
        total_query: http_requests_total{service="billing_api"}

  authentication:
    name: Authentication Flow
    services: [identity_service]
    slos:
      - id: auth_success
        service: identity_service
        objective: 99
        ...
```

### CLI Commands

```bash
# List current SLO status
health-monitor slo list

# Debugging PromQL (View exact queries being sent to Prometheus)
HEALTH_MONITOR_DEBUG=1 health-monitor slo list

# Start the monitor in the background for core-platform profile
sudo ./health-monitor --profile core-platform slo monitor \
  --interval 30s \
  --background \
  --pid-file /run/health-monitor-slo-core.pid \
  --log-file /var/log/health-monitor/slo-core.log
```

### SLO Status Meanings

| Status | Meaning | Action |
|--------|---------|--------|
| `🟢 OK` | Meeting objective | None |
| `🔴 ERR` | Logic or connection error | Check `doctor`, use `HEALTH_MONITOR_DEBUG=1` |
| `🟡 NO_DATA` | Prometheus returned no data for query | Verify metrics exist and service labels are correct |
| `🔥 BREACH` | Objective not met | Investigative active incident |

### Incident Consolidation Logic

| Scenario | Original Behavior | New Behavior (with Consolidation) |
|----------|-------------------|-----------------------------------|
| No existing incident | Creates Suggested INC | Creates Suggested INC |
| Active incident exists | Creates NEW Suggested INC | Adds Detailed Note to Active INC |
| Suggested incident < 4h | Adds Note | Adds Note |
| Suggested incident > 4h | Adds Note | Creates FRESH Suggested INC |
| Higher severity alert | Adds Note | Escalates Incident Severity + Adds Note |

---

## Tracing & Observability

Health-Monitor correlates incidents with distributed traces to simplify root cause analysis.

### Supported Backends
- **Tempo**: Native integration with Grafana Tempo.
- **Jaeger**: Full support for Jaeger backends using service-based correlation.

### Configuration
In your profile (`/etc/health-monitor/profiles/<name>.yaml`), configure the tracing provider:
```yaml
config:
  grafana_trace_ds: "Tempo"  # or "Jaeger"
  loki_service_label: "service,app,container" # Labels used to find traces
```

### How it Works
1. When viewing an incident (`incident view`), the agent automatically searches the tracing backend.
2. It uses the identified service and incident time window to find relevant traces.
3. Links to the external UI (Grafana/Jaeger) are displayed for instant deep-diving.
4. Latency analysis is performed on found traces to highlight potential bottlenecks.

---

## Auto-Update Setup

### How Auto-Update Works

1. **Checks for updates** on each run (before health checks)
2. **Downloads new version** if available
3. **Installs automatically** (replaces current binary)
4. **Shows notification** when update completes
5. **Continues normally** on subsequent runs

### Cloud Storage Setup

#### Step 1: Upload Files to R2

**Required files:**

1. **`version.txt`** - Contains latest version number
   ```
   v0.3
   ```

2. **Binary files** - Named `health-monitor-linux-amd64-v{VERSION}`
   - Example: `health-monitor-linux-amd64-v0.3`

#### Step 2: Enable Public Access

In Cloudflare R2 Dashboard:
1. Go to your bucket
2. Settings → Public Access → Enable
3. Verify files are accessible via HTTP/HTTPS

#### Step 3: Verify URLs

```bash
# Test version.txt
curl https://your-bucket.r2.dev/version.txt
# Should return: v0.3

# Test binary
curl -I https://your-bucket.r2.dev/health-monitor-linux-amd64-v0.3
# Should return: HTTP 200 OK
```

### File Structure

```
bucket/
├── version.txt                                    # v0.3
├── health-monitor-linux-amd64-v0.1               # Binary
├── health-monitor-linux-amd64-v0.2               # Binary
└── health-monitor-linux-amd64-v0.3               # Binary (latest)
```

### Updating to New Version

1. **Build new binary** (see Developer Guide)
2. **Upload binary** to R2: `health-monitor-linux-amd64-v{VERSION}`
3. **Update version.txt** to new version
4. **Upload version.txt** to R2
5. **Agents auto-update** on next run

---

## Troubleshooting

### Update Not Working

**Symptoms:**
- No update message shown
- Version doesn't change
- Error messages in output

**Solutions:**

1. **Check UPDATE_BASE_URL:**
   ```bash
   echo $UPDATE_BASE_URL
   # Should show your R2 URL
   ```

2. **Verify version.txt is accessible:**
   ```bash
   curl $UPDATE_BASE_URL/version.txt
   # Should return version number
   ```

3. **Verify binary is accessible:**
   ```bash
   curl -I $UPDATE_BASE_URL/health-monitor-linux-amd64-v{VERSION}
   # Should return HTTP 200 OK
   ```

4. **Check file permissions:**
   ```bash
   ls -lh /usr/local/bin/health-monitor
   # Should be executable (rwxr-xr-x)
   ```

5. **Check network connectivity:**
   ```bash
   ping -c 1 your-bucket.r2.dev
   ```

### Common Issues

**Issue: "Update check failed: 404"**
- **Cause**: Wrong URL or file not found
- **Fix**: Verify UPDATE_BASE_URL matches your R2 bucket URL

**Issue: "Update check failed: timeout"**
- **Cause**: Network issue or slow connection
- **Fix**: Check network connectivity, update continues normally

**Issue: Binary replacement fails**
- **Cause**: Insufficient permissions or disk space
- **Fix**: Ensure `/usr/local/bin/` is writable, check disk space

**Issue: Version not updating in banner**
- **Cause**: version.txt not accessible or network issue
- **Fix**: Verify version.txt URL, check network

### Diagnostic Commands

```bash
# Check current version
health-monitor --version

# Check environment variables
env | grep UPDATE

# Test version.txt access
curl $UPDATE_BASE_URL/version.txt

# Test binary access
curl -I $UPDATE_BASE_URL/health-monitor-linux-amd64-v{VERSION}

# Run with error output
health-monitor 2>&1 | tee /tmp/health-monitor.log

# Check for errors
grep -i "error\|fail\|update" /tmp/health-monitor.log
```

## End-to-End Testing (Incidents)

Run the following commands in order to validate the incident workflow:

```bash
# 1) Start a new incident
./health-monitor incident start --service order_api --severity P1 --title "Checkout timeouts"

# 2) Add a note
./health-monitor incident note "Scaled postgres pool 50->100"

# 3) Acknowledge ownership
./health-monitor incident ack --user sanjana

# 4) View the latest incident timeline
./health-monitor incident view

# 5) Resolve the incident
./health-monitor incident resolve --summary "DB pool exhaustion"

# 6) Export the markdown report (default /tmp unless HEALTH_MONITOR_EXPORT_PATH is set)
./health-monitor incident export --format markdown

# 7) Export JSON report (default /tmp unless HEALTH_MONITOR_EXPORT_PATH is set)
./health-monitor incident export --format json

# 8) List all incidents
./health-monitor incident list

# 9) (Optional) Open TUI and press 'i' to verify the Incidents panel
./health-monitor
```

**Tip:** Use the same binary for all commands (either `./health-monitor` or the one found by `which health-monitor`). Mixing binaries can cause “missing output” if an older version is on your PATH.

### Getting Help

- Check error messages in output
- Verify all URLs are accessible
- Ensure files are publicly accessible in R2
- Check network connectivity
- Verify file permissions

---

## Quick Reference

### Installation
```bash
curl -LO https://your-bucket.r2.dev/health-monitor-linux-amd64-v{VERSION}
chmod +x health-monitor-linux-amd64-v{VERSION}
sudo mv health-monitor-linux-amd64-v{VERSION} /usr/local/bin/health-monitor
```

### Configuration
```bash
export UPDATE_BASE_URL="https://your-bucket.r2.dev"
```

### Usage
```bash
health-monitor          # Run health check
health-monitor --version # Show version
health-monitor --help   # Show help
```

### TUI Controls
- `↑/↓` or `j/k` - Scroll
- `Home/g` - Top
- `End/G` - Bottom
- `i` - Incidents
- `q` - Exit

---

## Storage Migration

If you need to change the storage domain (e.g., migrating from one R2 bucket to another, or from R2 to AWS S3), see **[STORAGE_MIGRATION.md](STORAGE_MIGRATION.md)** for complete migration strategies that ensure zero downtime and no disruption to existing agents.

**Quick options:**
1. **Update environment variable** on each server
2. **Set up redirect** from old domain to new domain
3. **Dual storage period** - keep both active during migration

## Support

For issues or questions:
- Check this guide's troubleshooting section
- Verify all configuration settings
- Test URLs manually with curl
- Check server logs for errors
- See [STORAGE_MIGRATION.md](STORAGE_MIGRATION.md) for migration help

---

## Feedback & Community

Help improve the Health-Monitor agent by providing direct feedback on features and usability.

### Providing Feedback

The `feedback` command launches a premium, interactive TUI for submitting ratings and notes.

```bash
# Launch interactive feedback TUI
health-monitor feedback
```

**Features:**
- **Star-based Ratings**: Rate key areas (Reliability, Diagnostics, UI/UX) on a 1-5 scale.
- **Detailed Notes**: Add context for your ratings to help the development team.
- **Privacy First**: Your machine ID and username are automatically hashed (SHA-256) before submission.
- **Cloud Delivery**: Feedback is sent directly to the agent maintainers via a secure API.

### Advanced: API Authentication

For organization-wide deployments, you can validate feedback submissions using an API token:

```bash
export HEALTH_MONITOR_FEEDBACK_TOKEN="your-secure-token"
health-monitor feedback
```
