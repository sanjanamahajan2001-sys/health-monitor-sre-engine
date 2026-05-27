# Health-Monitor Agent: Comprehensive Feature Documentation & Command Reference

## Table of Contents
1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Core Commands](#core-commands)
4. [Terminal UI (TUI)](#terminal-ui-tui)
5. [Incident Management](#incident-management)
6. [Runbook Automation](#runbook-automation)
7. [Service Flow Management](#service-flow-management)
8. [SLO Monitoring](#slo-monitoring)
9. [Alert Management](#alert-management)
10. [Scorecard System](#scorecard-system)
11. [Machine Learning Features](#machine-learning-features)
12. [Security & Authentication](#security--authentication)
13. [Multi-Profile Configuration](#multi-profile-configuration)
14. [Sudo & User-Mode Execution](#sudo--user-mode-execution)
15. [Notification Integrations](#notification-integrations)
15. [Metrics & Monitoring](#metrics--monitoring)
18. [Diagnostics & Health Checks](#diagnostics--health-checks)
19. [Configuration Management](#configuration-management)
20. [Team Presets & Setup Wizard](#team-presets--setup-wizard)
21. [Advanced Features](#advanced-features)
22. [Predictive Incident Prevention](#predictive-incident-prevention)
23. [Collaborative SRE Sessions (SSH Tunnels)](#collaborative-sre-sessions-ssh-tunnels)
24. [Interactive Guided Tour & SRE Training System](#interactive-guided-tour--sre-training-system)

---

## Overview

Health-Monitor is a production-grade SRE toolkit designed for high-availability observability, automated incident response, and proactive reliability engineering. It provides a unified interface for monitoring, incident management, and operational intelligence across distributed systems.

### Key Capabilities
- **Real-time System Monitoring**: CPU, memory, disk, GPU, network metrics
- **Interactive Terminal UI**: Live dashboards with drill-down capabilities
- **Incident Lifecycle Management**: Creation, tracking, resolution, and RCA
- **Automated Runbook Generation**: Pattern-based troubleshooting procedures
- **Service Dependency Mapping**: End-to-end flow visualization
- **SLO Monitoring & Alerting**: Service level objective tracking
- **Multi-Profile Support**: Environment-specific configuration isolation
- **ML-Powered Analysis**: Anomaly detection and pattern recognition
- **Enterprise Security**: Authentication, authorization, and audit capabilities

---

## Architecture

### Modular Design
```
health-monitor/
├── cmd/                    # CLI entry points
├── internal/              # Core packages
│   ├── alert/            # AlertManager integration
│   ├── alerts/           # Profile-specific alerts
│   ├── config/           # Profile management
│   ├── doctor/           # Health diagnostics
│   ├── flow/             # Service flow mapping
│   ├── history/          # Profile-specific history
│   ├── incident/         # Incident management
│   ├── incidents/        # Profile-specific incidents
│   ├── metrics/          # Performance metrics
│   ├── ml/               # Machine learning
│   ├── notify/           # Notifications
│   ├── runbook/          # Automated runbooks
│   ├── scorecard/        # Reliability scoring
│   ├── security/         # Security features
│   ├── slo/              # SLO monitoring
│   └── traces/           # Distributed tracing
└── profiles/             # Environment configurations
```

### Data Flow
1. **Collection**: Metrics from Prometheus, logs from Loki, traces from Tempo
2. **Correlation**: Cross-system data correlation and analysis
3. **Detection**: Anomaly and pattern detection using ML algorithms
4. **Response**: Automated incident creation and runbook suggestions
5. **Notification**: Multi-channel alerting and escalation

---

## Core Commands

### Primary Command Structure
```bash
health-monitor [global-flags] <command> [subcommand] [options]
```

### Global Flags
- `--profile <name>`: Configuration profile to use
- `--profile-path <path>`: Path to specific profile file
- `--help`: Show help information
- `--version`: Display version information
- `--init`: Initialize configuration file

### Initialization Commands

#### Dynamic Discovery (`--init`)

**Purpose**: Scans actual infrastructure to create accurate, production-ready configurations using an adaptive discovery engine.

**Syntax**:
```bash
# EKS Clusters (CRITICAL: Use -E for AWS credentials)
sudo -E ./health-monitor --init --infra eks --kubeconfig ~/.kube/config

# Local Kubernetes
sudo ./health-monitor --init --infra kubernetes --kubeconfig ~/.kube/config

# With custom profile name
sudo -E ./health-monitor --init --infra eks --profile production-cluster

# Debug mode
sudo -E ./health-monitor --init --infra eks --debug
```

**AWS Environment Variables (Required for EKS)**:
```bash
export AWS_REGION=us-west-2
export AWS_ACCESS_KEY_ID=your-access-key
export AWS_SECRET_ACCESS_KEY=your-secret-key
export AWS_SESSION_TOKEN=your-session-token  # if using temporary credentials
```

**What `--init` Does**:
1. **Kubernetes Discovery**: Scans all namespaces for services and deployments.
2. **Adaptive Metric Probing**: Instead of guessing, the tool queries Prometheus active series (`/api/v1/series`) to find which metrics are actually reporting for each service.
3. **Intelligent Label Detection**: Automatically identifies the correct service identifier labels (e.g., `app`, `job`, `container`, `service`, `kubernetes_namespace`) by probing active data.
4. **Query Validation**: Tests that discovered metrics return actual data and calculates baselines.
5. **Flow Generation**: Creates SLO flows based on validated, working metrics.
6. **Configuration Building**: Generates production-ready configurations.

**Output Example**:
```
=== Service Discovery Complete ===
Services discovered: 21
Flows generated: 7
Services skipped: 14

Generated flows for: cart, frontend, payment, currency, shipping, kafka, postgresql
```

**Generated Files**:
- `/etc/health-monitor/profiles/{profile-name}.yaml` - Main configuration
- `/etc/health-monitor/flows.d/{profile-name}.yaml` - SLO flows
- `/etc/health-monitor/alerts.d/{profile-name}.yaml` - Alert rules
- `/etc/health-monitor/state/{profile-name}/` - Isolated state directory

#### Preset Configuration (`--wizard-team`)

**Purpose**: Uses predefined templates for quick, standardized setup.

**Syntax**:
```bash
# Interactive team setup
sudo ./health-monitor --wizard-team

# Non-interactive with preset
sudo ./health-monitor --wizard-team --preset microservices
```

**Available Presets**:
- `microservices` - Standard microservices with `http_requests_total` metrics
- `ecommerce` - E-commerce platform with payment and cart metrics
- `saas` - SaaS application with user and subscription metrics
- `api-gateway` - API gateway with request/response metrics

**What `--wizard-team` Does**:
1. **Template Selection**: Choose from predefined configuration templates
2. **Standard Metrics**: Uses common metric patterns (`http_requests_total`, `http_request_duration_seconds`)
3. **Quick Setup**: No infrastructure scanning required
4. **Team Standards**: Ensures consistent configurations across teams

#### Key Differences: `--init` vs `--wizard-team`

| Aspect | `--init` (Dynamic Discovery) | `--wizard-team` (Presets) |
|--------|------------------------------|---------------------------|
| **Setup Time** | 2-5 minutes | 10-30 seconds |
| **Infrastructure Access** | Required | Not required |
| **Metric Accuracy** | High (discovered) | Medium (standard) |
| **Customization** | High (based on actual) | Medium (template-based) |
| **Production Ready** | ✅ Yes | ⚠️ May need tuning |
| **Best For** | Production, custom metrics | Demos, testing, standard setups |
| **Query Validation** | ✅ Automatic | ❌ Manual verification needed |

#### Configuration Editing

Both methods generate editable YAML files:

**Profile Configuration** (`/etc/health-monitor/profiles/{name}.yaml`):
```yaml
name: my-cluster
provider: eks
config:
  prometheus_url: "http://localhost:9090"
  eks:
    enabled: true
    region: "us-west-2"
    kubeconfig: "/home/user/.kube/config"
    namespaces: ["default", "production"]
```

**Flow Configuration** (`/etc/health-monitor/flows.d/{name}.yaml`):
```yaml
flows:
  my-service:
    name: "My Service Flow"
    services: ["my-service"]
    slos:
      - id: "success_rate"
        objective: 99.0
        window: "5m"
        error_query: "sum(rate(my_requests_total{status!~\"2..\"}[5m]))"
        total_query: "sum(rate(my_requests_total[5m]))"
```

**Alert Configuration** (`/etc/health-monitor/alerts.d/{name}.yaml`):
```yaml
alerts:
  rules:
    - name: "High Error Rate"
      condition: "error_rate > 0.05"
      severity: "critical"
      notifications:
        slack:
          webhook_url: "https://hooks.slack.com/your-webhook"
```

#### Usage Examples

**Production EKS Setup**:
```bash
# Set AWS credentials
export AWS_REGION=us-west-2
export AWS_ACCESS_KEY_ID=AKIA...
export AWS_SECRET_ACCESS_KEY=...

# Initialize with discovery
sudo -E ./health-monitor --init --infra eks --profile production

# Start monitoring
sudo ./health-monitor --profile production
```

**Quick Demo Setup**:
```bash
# Quick preset setup
sudo ./health-monitor --wizard-team --preset microservices

# Start with default profile
sudo ./health-monitor
```

**Hybrid Approach**:
```bash
# 1. Quick start with presets
sudo ./health-monitor --wizard-team --profile demo

# 2. Test and validate
sudo ./health-monitor --profile demo slo list

# 3. Production setup with discovery
sudo -E ./health-monitor --init --infra eks --profile production

# 4. Manually merge configurations if needed
```

### Command Categories

#### 1. Core System Commands
| Command | Purpose | Description |
|---------|---------|-------------|
| `health-monitor` | TUI Dashboard | Launch interactive terminal UI with live metrics |
| `demo` | Sandbox Environment | Explore features with safe, simulated data (TUI or CLI) |
| `doctor` | Diagnostics | Comprehensive system health checks |
| `config` | Management | Configuration validation and management |
| `guide` | Training | Interactive onboarding and troubleshooting tour |

#### 2. Incident Management
| Command | Purpose | Description |
|---------|---------|-------------|
| `incident start` | Creation | Create new incident with automatic log correlation |
| `incident list` | Viewing | List all incidents with filtering options |
| `incident view` | Details | View detailed incident information |
| `incident resolve` | Resolution | Resolve incidents with RCA data |
| `incident note` | Updates | Add notes and updates to incidents |
| `incident export` | Reporting | Export incidents in various formats |
| `incident postmortem` | Reporting | Generate blameless postmortem report |

#### 3. Runbook Automation
| Command | Purpose | Description |
|---------|---------|-------------|
| `runbook suggest` | Analysis | Suggest runbooks based on incident patterns |
| `runbook generate` | Creation | Generate detailed runbooks |
| `runbook list` | Inventory | List available runbooks |
| `runbook patterns` | Management | Manage pattern definitions |

#### 4. Service Flow Management
| Command | Purpose | Description |
|---------|---------|-------------|
| `flow list` | Discovery | List all service flows and dependencies |
| `flow validate` | Validation | Validate flow configuration syntax and SLOs |
| `flow view` | Analysis | View detailed flow topology |

#### 5. SLO Monitoring
| Command | Purpose | Description |
|---------|---------|-------------|
| `slo list` | Viewing | List configured SLOs |
| `slo monitor` | Tracking | Real-time SLO monitoring and alerting |

#### 6. Alert Management
| Command | Purpose | Description |
|---------|---------|-------------|
| `alert listen` | Reception | Start alert listener for webhooks |
| `alert status` | Monitoring | Check alert system status |
| `alert list-rules` | Inventory | View all configured alert rules |
| `alert validate-config` | Validation | Validate alert rule definitions and notifications |
| `alert stop` | Control | Stop alert listener |

#### 7. Advanced Features
| Command | Purpose | Description |
|---------|---------|-------------|
| `scorecard` | Reliability | View service reliability scores |
| `ml predict` | Analysis | ML-powered pattern prediction |
| `ml sim` | Similarity | Find similar incidents |
| `security` | Audit | Security configuration validation |
| `metrics` | Performance | Query performance metrics |
| `notifications` | Integration | Test notification systems |

---

## Demo Mode

### `health-monitor demo`

The `demo` command provides a safe, fully functional sandbox environment designed to showcase the capabilities of Health-Monitor without requiring any backend infrastructure (Prometheus, Loki, Tempo) to be configured or accessible.

#### Purpose & Capabilities
- **Safe Exploration**: Safely interact with all application features (TUI dashboards, incident browsers, RCA analysis, Runbook generation, Markdown/JSON exports) using simulated data.
- **Production Isolation**: The demo environment uses a dedicated, temporary "demo" profile. It completely isolates its state from your actual production configuration, ensuring zero risk of accidental modifications or network calls to real backends.
- **Feature Showcase**: The demo populates the system with realistic frontend, backend, and database services, generating synthetic metrics, logs, slow traces, and pre-populated incidents complete with Root Cause Analysis (RCA) and Action Items.
- **Onboarding & Training**: An excellent tool for training team members on how to use the TUI, navigate incident lifecycles, and understand the tool's capabilities.

#### What it Does
1. Creates a temporary configuration and state directory specifically for the demo.
2. Generates fake but realistic system metrics (CPU, Memory, Disk, Network) and service API metrics (latency, error rates).
3. Pre-loads a simulated incident history with rich RCA data, correlated logs, trace latencies, and observability deep links.
4. Suppresses all real network calls to prevent attempting to connect to configured backends.
5. Launches the full Terminal UI (TUI) experience by default, loaded with the simulated data.

#### Command Flags

| Flag | Description |
|------|-------------|
| `--force-cli` | Bypasses the interactive Terminal UI (TUI) and instead outputs a rich, static CLI summary of the simulated system state and active incidents to standard output. Useful for scripting or environments where a TUI is not supported. |

#### Usage Examples

```bash
# Launch the fully interactive TUI populated with demo data
health-monitor demo

# Output a static CLI summary of the demo environment
health-monitor demo --force-cli
```

---

## Terminal UI (TUI)

### Main Dashboard Features

#### Real-time Metrics Display
- **System Health**: CPU, memory, disk, GPU utilization
- **Service Performance**: Latency percentiles (P90, P95, P99)
- **Error Rates**: 4xx vs 5xx breakdown with trends
- **Request Rates**: Real-time RPS monitoring
- **Resource Usage**: Top consumers and optimization suggestions

#### Interactive Controls
- **Navigation**: Arrow keys, Page Up/Down, Home/End
- **Refresh**: `R` key for manual refresh
- **Configuration**: `C` key to edit current profile
- **Help**: `?` key for contextual help
- **Quit**: `Q` or `Ctrl+C` to exit

#### Status Indicators
- **🟢 SAFE**: Operating within normal thresholds
- **🟡 CHECK**: Needs attention, not critical
- **🔴 RISK**: Immediate action recommended

#### Advanced Views
- **Service Details**: Drill-down into individual service metrics
- **Dependency Graph**: Visual representation of service dependencies
- **Incident Overlay**: Active incidents displayed on service metrics
- **Trend Analysis**: Historical performance comparisons

### TUI Keyboard Shortcuts
```
General:
  h, ?        Show help
  q, Q        Quit application
  R           Refresh data
  C           Configuration editor

Navigation:
  ↑, ↓        Navigate up/down
  ←, →        Navigate left/right
  PgUp, PgDn  Page navigation
  Home, End   Jump to start/end
  Enter       Select/expand
  Esc         Back/collapse

Views:
  1           System overview
  2           Service metrics
  3           Incidents
  4           Dependencies
  5           SLO status
  6           Alerts

Incident Actions views:
  a           Add incident
  r           Resolve incident
  n           Add note
  e           Export data
  m           Generate postmortem
  s           Switch profile
```

---

## Incident Management

### Incident Lifecycle

#### 1. Incident Creation
```bash
# Manual creation
sudo health-monitor incident start \
  --service billing_api \
  --title "Payment Processing Degradation" \
  --severity P1 \
  --description "High error rates in payment processing"

# Automatic creation from alerts
curl -X POST http://localhost:8080/webhook \
  -H "Content-Type: application/json" \
  -d '{"alerts": [{"labels": {"severity": "critical"}}]}'
```

#### 2. Automatic Log Correlation
- **Loki Integration**: Fetches relevant logs from time window
- **Pattern Extraction**: Identifies error patterns and signatures
- **Frequency Analysis**: Counts and categorizes error types
- **Sample Collection**: Preserves full log samples for analysis

#### 3. Incident Enrichment
- **Service Context**: Automatic service dependency mapping
- **Historical Analysis**: Similar incident detection
- **Impact Assessment**: Affected flows and user impact
- **Runbook Suggestions**: Pattern-based troubleshooting guidance

#### 4. Resolution & RCA
```bash
# Interactive resolution with RCA
sudo health-monitor incident resolve \
  --id INC-20260220-123456 \
  --summary "Database connection restored" \
  --root-cause "Connection pool exhaustion" \
  --fix "Increased pool size and added retry logic" \
  --category dependency \
  --component postgres_db \
  --dependency "billing_api->postgres_db" \
  --failure-type dependency \
  --prevention "Implement connection pool monitoring" \
  --well "Automated failover worked" \
  --better "Monitoring latency was high" \
  --lucky "Traffic was low during peak" \
  --downtime 15
```

#### Resolution Flags
- **RCA Flags**: 
  - `--root-cause`: Primary cause description
  - `--fix`: Summary of the resolution
  - `--category`: Incident classification (capacity, latency, dependency, etc.)
  - `--component`: Affected system component
  - `--dependency`: Relationship affected (e.g., service->db)
  - `--failure-type`: Type of failure (service, dependency, infra)
  - `--prevention`: Measures to avoid recurrence
  - `--pattern`: Error pattern signature
- **Blameless Flags**:
  - `--well`: What went well during response
  - `--better`: What could be better in tools/process
  - `--lucky`: Where the team got lucky
  - `--downtime`: Estimated downtime in minutes


### Incident Analysis Features

#### Similarity Matching
- **Pattern Recognition**: ML-powered incident similarity
- **Solution Suggestions**: Proven fixes from similar incidents
- **Confidence Scoring**: Reliability of similarity matches
- **Historical Context**: Time-based pattern analysis

#### Advanced Similarity Analysis
```bash
# Find similar incidents with detailed analysis
sudo ./health-monitor incident similar --id INC-20260225-122537

# Output when similar incidents are found:
# Similar past incidents (last 90d):
#
# 1) INC-20260225-115051  [25 Feb 2026 11:50:51]  ⭐ 50% match
#    Service: identity_service
#    Dependency: identity_service->jwt_provider
#    Pattern: authentication_issue
#    Root cause: JWT token signing key rotation issue
#    Fix: Updated token validation logic and key rotation process
#    Matched on: service
#
# 2) INC-20260224-132950  [24 Feb 2026 13:29:50]  ⭐ 50% match
#    Service: identity_service
#    Matched on: service, title, (+1 similar)
#
# 💡 Tip: Use 'health-monitor incident view --id <ID>' to see full details

# If no similar incidents found:
# No similar incidents found in last 90 days.
#
# 💡 Try lowering the confidence threshold:
#    health-monitor incident similar --min-confidence 0.5
#
# 💡 Try searching further back:
#    health-monitor incident similar --days 180
#
# 💡 This incident lacks RCA data, which reduces matching accuracy.
#    You can manually specify matching criteria:
#    health-monitor incident similar --component postgres_db --category capacity

# Advanced options for better matching:
sudo ./health-monitor incident similar \
  --id INC-20260225-122537 \
  --min-confidence 0.5 \
  --days 180 \
  --component postgres_db \
  --category capacity

# Interactive feedback system:
# Were these suggestions helpful? (y/n/sip)
# - y: Mark as helpful for future ML training
# - n: Mark as not helpful 
# - skip: Continue without feedback
```

#### Similarity Matching Features
- **Pattern Matching**: Analyzes error patterns and signatures
- **Service Context**: Matches based on service and component
- **RCA Analysis**: Uses root cause and fix data for matching
- **Time-based Search**: Configurable search window (default 90 days)
- **Confidence Scoring**: Adjustable confidence thresholds
- **Interactive Feedback**: User feedback improves future matching
- **Manual Criteria**: Override with specific component/category matching

#### Postmortem Generation (Blameless)
Automated generation of high-fidelity, blameless postmortem reports from incident metadata, RCA, and timeline.

```bash
# Generate postmortem for latest incident
health-monitor incident postmortem

# Generate for specific incident
health-monitor incident postmortem --id INC-20260226-111745
```

- **Output Isolation**: Reports are saved to `/tmp/health-monitor/` by default. Override this with the `HEALTH_MONITOR_POSTMORTEM_PATH` environment variable.
- **Rich Formatting**: Includes YAML Frontmatter (for automation), Table of Contents, Executive Summary, Root Cause Analysis, Technical Evidence (logs/traces), Lessons Learned, and Accountability Tables.
- **Audit Trail**: Generates a permanent record in the incident timeline whenever a postmortem is exported.

#### Action Items Management
- **Task Assignment**: Assign owners and due dates
- **Priority Tracking**: P1-P4 priority classification
- **Status Updates**: Track completion progress
- **Escalation**: Automatic escalation for overdue items

#### Export & Reporting
```bash
# Export in multiple formats
health-monitor incident export --format markdown
health-monitor incident export --format json --id INC-20260220-123456
health-monitor incident export --format csv --data-dir /custom/path
```

---

## Runbook Automation

### Pattern Recognition System

#### Built-in Pattern Categories
1. **Database Patterns**
   - `postgres_connection_timeout`
   - `mysql_connection_failed`
   - `redis_cache_timeout`
   - `database_connection_pool_exhausted`

2. **HTTP/API Patterns**
   - `http_5xx_errors`
   - `connection_timeout`
   - `connection_refused`
   - `rate_limit_exceeded`

3. **Resource Patterns**
   - `out_of_memory`
   - `cpu_exhaustion`
   - `disk_space_full`
   - `file_descriptor_limit`

4. **Security Patterns**
   - `authentication_failure`
   - `authorization_error`
   - `ssl_certificate_issue`

5. **Payment Patterns**
   - `payment_gateway_timeout`
   - `stripe_api_outage`
   - `payment_processing_error`

### Runbook Generation Process

#### 1. Pattern Analysis
```bash
# Suggest runbooks for incident
sudo health-monitor runbook suggest INC-20260220-123456

# Output:
# 🔍 Pattern Analysis for INC-20260220-123456
# Pattern: postgres_connection_timeout
# Confidence: 95.0%
# Suggested Runbook: rb-postgres-connection-timeout
```

#### 2. Dynamic Step Generation
- **Investigation Steps**: Service status, log review, resource checks
- **Diagnostic Steps**: Pattern-specific troubleshooting commands
- **Resolution Steps**: Common fixes and escalation procedures
- **Verification Steps**: Post-resolution validation

#### 3. Database-Specific Enhancements
```bash
# PostgreSQL specific steps
1. Test PostgreSQL Connection
   psql -h localhost -U postgres -c 'SELECT 1;'

2. Check Connection Pool
   ps aux | grep postgres | grep -v grep

3. Validate Configuration
   env | grep -i db
```

#### 4. Metrics & Monitoring Integration
- **Prometheus Queries**: Database-specific metrics
- **Loki Queries**: Pattern-specific log searches
- **Grafana Links**: Direct links to relevant dashboards

#### 5. RCA Data Integration in Runbooks
When generating runbooks for resolved incidents, the system automatically includes Root Cause Analysis (RCA) data from previous similar incidents:

```bash
# Generate runbook with RCA integration
sudo health-monitor runbook generate --incident INC-20260220-123456 --save

# Generated runbook includes RCA sections:
# postgres_db - Postgres Connection Timeout Troubleshooting
#
# ## Root Cause Analysis
#
# ### Root Cause
# Postgres connection pool exhausted
#
# ### Applied Fix
# Increased connection pool size and added retry logic
#
# **Failure Type**: dependency
#
# ### Historical Analysis
# Based on historical incident analysis:
#
# - **Pattern**: postgres_connection_timeout
# - **Frequency**: This pattern has occurred 3 times
# - **Related Incidents**:
#   - [INC-20260220-114048](/incidents/INC-20260220-114048) - Resolved in 15 min
#   - [INC-20260220-111942](/incidents/INC-20260220-111942) - Resolved in 22 min
#
# ## Prevention
#
# ### Recommended Prevention
# Implement connection pool monitoring and auto-scaling
#
# ### Prevention Strategies from Similar Incidents
# Based on analysis of 3 similar incidents:
# 1. **Connection Pool Monitoring** (used in 2/3 incidents)
#    - Set up alerts for pool utilization > 80%
#    - Implement auto-scaling for connection pools
#
# 2. **Database Performance Optimization** (used in 1/3 incidents)
#    - Regular query performance monitoring
#    - Implement read replicas for load distribution
#
# 3. **Infrastructure Hardening** (used in 1/3 incidents)
#    - Network health checks
#    - Redundant connection paths
```

#### 6. Complete Runbook Generation Example
```bash
# Full runbook generation process
sudo health-monitor runbook generate --incident INC-20260220-123456 --save

# Output shows comprehensive runbook:
# 📚 Generated Runbook
#
# ID: rb-20260220-124000-billing_api-postgres-connection-timeout
# Title: postgres_db - Postgres Connection Timeout Troubleshooting
# Service: billing_api
# Pattern: postgres_connection_timeout
# Severity: P3
# Steps: 11
# Metrics: 6
# Log Queries: 5
# Format: markdown
# Created: 2026-02-20 12:40:00
#
# 📄 Content Preview:
# # postgres_db - Postgres Connection Timeout Troubleshooting
#
# ## Overview
# **Service**: billing_api
# **Component**: postgres_db
# **Pattern**: postgres_connection_timeout
# **Category**: dependency
# **Severity**: P3
# **Frequency**: 3 occurrences
# **Last Seen**: 2026-02-20 12:26:52
# **Confidence**: 95.0%
#
# ## Root Cause Analysis
# ### Root Cause
# Postgres connection pool exhausted
#
# ### Applied Fix
# Increased connection pool size and added retry logic
#
# ### Historical Context
# Based on 3 similar incidents with average resolution time: 18 minutes
#
# ## Troubleshooting Steps
# 1. **Verify Service Status** 🔴
# 2. **Review Recent Logs** 🔴
# 3. **Test PostgreSQL Connection** 🔴
# 4. **Check PostgreSQL Connection Pool** 🔴
# 5. **Validate Database Configuration**
# 6. **Review Similar Incidents**
# 7. **Apply Proven Fixes** (based on historical data)
# 8. **Verify Resolution**
#
# ✅ Runbook saved to: /var/lib/health-monitor/runbooks/core-platform/rb-20260220-124000-billing_api-postgres-connection-timeout.md
```

### Custom Pattern Definition
```yaml
# profiles/custom.yaml
runbook_suggestions:
  custom_patterns:
    - name: "billing_service_error"
      pattern: "billing service unavailable"
      runbook: "rb-billing-service-recovery"
      confidence: 0.9
      description: "Billing service failure handling"
      steps:
        - "Check billing service health"
        - "Verify payment gateway connectivity"
        - "Review recent deployment changes"
```

---

## Service Flow Management

### Flow Discovery & Mapping

#### Automatic Service Discovery
- **Configuration-Based**: Service definitions from profiles
- **Dependency Mapping**: Service-to-service relationships
- **Flow Visualization**: End-to-end request paths
- **Health Correlation**: Service health impact analysis

#### Flow Configuration
```yaml
# profiles/core-platform-flows.yaml
flows:
  checkout:
    name: "Checkout Flow"
    services:
      - billing_api
      - identity_service
      - payments_service
      - inventory_service
    entry_point: "web_frontend"
    exit_point: "payment_gateway"
    critical_path: true
    sla_threshold: 2.0
```

#### Flow Analysis Features
- **Performance Metrics**: End-to-end latency tracking
- **Error Propagation**: Impact analysis across services
- **Bottleneck Identification**: Performance bottleneck detection
- **Health Scoring**: Overall flow health assessment

### Flow Commands
```bash
# List all flows
health-monitor flow list

# View specific flow
health-monitor flow view checkout

# JSON output for automation
health-monitor flow list --json
```

---

## SLO Monitoring

### SLO Configuration & Tracking

#### SLO Definition
```yaml
# profiles/slo-config.yaml
slos:
  billing_api_latency:
    name: "Billing API Latency"
    service: "billing_api"
    type: "latency"
    target: 0.95  # 95th percentile
    threshold: 1.0  # 1 second
    window: "28d"
    alerting:
      burn_rate: "slow"
      notification: "slack"
  
  billing_api_availability:
    name: "Billing API Availability"
    service: "billing_api"
    type: "availability"
    target: 0.999  # 99.9%
    window: "30d"
    alerting:
      burn_rate: "fast"
      notification: "pagerduty"
```

#### Real-time Monitoring
- **Error Budget Tracking**: Remaining error budget calculation.
- **Burn Rate Analysis**: Current vs. historical burn rates.
- **Breach Prediction**: Predictive breach detection.
- **Robust Status Handling**: Differentiates between actual logic errors (`ERR`) and missing metrics (`NO_DATA`), preventing false alarms in the TUI.
- **Alert Integration**: Automated alerting on SLO breaches.

#### SLO Commands
```bash
# List all SLOs
health-monitor slo list

# Start SLO monitoring
health-monitor slo monitor

# Check monitoring status
health-monitor slo monitor status

# Stop monitoring
health-monitor slo monitor stop

# Debugging PromQL (View exact queries being sent to Prometheus)
HEALTH_MONITOR_DEBUG=1 health-monitor slo list
```

---

## Alert Management

### AlertManager Integration

#### Alert Reception
- **Webhook Endpoint**: HTTP endpoint for AlertManager webhooks
- **Multi-Source Support**: Prometheus, Grafana, custom systems
- **Authentication**: Token-based authentication
- **Rate Limiting**: Configurable rate limits per source

#### Alert Processing
- **Deduplication**: Prevent duplicate alert processing
- **Correlation**: Link alerts to existing incidents
- **Enrichment**: Add service context and runbook suggestions
- **Escalation**: Multi-level escalation policies

#### Alert Configuration
```yaml
# profiles/alerts.yaml
alerts:
  enabled: true
  webhook:
    port: 8080
    path: "/webhook"
    auth:
      enabled: true
      tokens:
        - name: "prometheus"
          token: "Bearer prometheus-token"
        - name: "grafana"
          token: "Bearer grafana-token"
  
  rules:
    - match: "service=billing_api severity=P1"
      mode: "auto"
      title: "Billing Service Critical Alert"
    
    - match: "service=payments_service severity=P2"
      mode: "manual"
      title: "Payment Service Degradation"
```

#### Alert Commands
| Command | Description | Example |
|---|---|---|
| `alert status` | Check if the alert-listen daemon is currently running. | `sudo ./health-monitor alert status` |
| `alert list-rules` | View all active alert rules in a condensed list format. | `sudo ./health-monitor alert list-rules` |
| `alert validate-config` | Perform a syntax and integrity check on all alert rule files. | `sudo ./health-monitor alert validate-config` |
| `alert stop` | Stop the alert-listen daemon. | `sudo ./health-monitor alert stop` |

---

## Scorecard System

### Reliability Scoring Framework

The scorecard system provides a unified view of team and service reliability metrics, including SLOs, operational efficiency, and toil.

#### Scoring Dimensions
1. **Service Level Objectives (SLOs)**: Measured across all configured flows (Availability & Success Rate).
2. **Operational Efficiency**: 
   - **MTTR**: Mean Time To Resolve
   - **MTTD**: Mean Time To Detect
3. **Capacity & Toil**: Manual effort vs. team capacity targets.
4. **Trends**: Month-over-month performance delta.

#### Commands
```bash
# Launch interactive scorecard TUI for current profile
sudo health-monitor scorecard

# Launch for a specific profile
sudo health-monitor scorecard --profile prod-eks

# NEW: Organization-Level Scoreboard (Aggregated view across all profiles)
sudo health-monitor scorecard --org

# Filter Organization view by profile environment (e.g., prod, staging)
sudo health-monitor scorecard --org --env prod
```

### Organization-Level Scoreboard (Org View 2.0)

The Organization-Level Scoreboard provides a high-fidelity, aggregated view of reliability across the entire fleet of managed profiles.

#### 1. Weighted Global Scoring
Unlike a simple average, the Global Health Score is calculated as a **weighted average** based on **service density**.
- **Formula**: `Sum(Profile Score * Profile Service Count) / Total Organizational Services`
- **Result**: Large teams with many critical services have a proportional impact on the global score, preventing small low-traffic profiles from skewing the results.

#### 2. Global Error Budget Burn Rate
The dashboard calculates the aggregate **Error Budget Burn Rate** for the entire organization.
- **Calculation**: `Avg((100 - Actual Availability) / (100 - SLO Target))` across all active SLOs.
- **Thresholds**:
    - `> 1.0x`: Budget is being exhausted faster than the monthly allowance.
    - `> 2.0x`: Critical budget depletion; immediate remediation required.

#### 3. Global Operational Health (SRE Standard)
- **MTTA (Mean Time to Acknowledge)**: Tracks organizational responsiveness to new incidents.
- **MTTR (Mean Time to Recover)**: Tracks the aggregate speed of restoration across all teams.
- **Global Availability Index**: Aggregated P1/P2 downtime across the entire monitored fleet.
- **Global Toil Index**: Calculated as a strict percentage of **Total Organizational Capacity** (`Total Toil Hours / Total Team Capacity`). This provides a true "waste" metric that isn't diluted by the number of profiles.

#### 4. Organizational Hotspots
Automatically identifies the **Top At-Risk Services** across the whole organization by analyzing:
- P1 incident frequency.
- SLO breach severity.
- Historical volatility.

#### 5. Profile Health Index
A ranked view of every profile (excluding system/demo profiles) showing:
- Reliability Score & Health Status.
- P1 Incident counts.
- Availability Trends.
- Individual Profile SLO compliance.

---

## Toil Tracking & ROI Analytics

### Operational Work Attribution

Toil tracking allows teams to measure the "manual, repetitive, automatable" work spent on incident response.

#### Recording Toil (Resolution Hook)
Toil data is captured during the `incident resolve` phase.

```bash
health-monitor incident resolve \
  --toil-minutes 60 \
  --toil-category manual_restart \
  --summary "Resolved memory leak via restart"
```

#### Analyzing Toil & ROI Analysis
The toil engine analyzes incident data to provide ROI (Return on Investment) suggestions for automation.

```bash
# View toil summary and ROI recommendations (Organization-wide)
sudo health-monitor toil list --window 30d

# Generate a Leadership-Ready Executive Report
sudo health-monitor toil report

# Filter toil analytics by profile
sudo health-monitor toil list --profile staging-eks
```

#### Strategic Toil Reporting (`toil report`)
The `toil report` command generates a professional **Leadership Brief** (Markdown/Text) that summarizes the financial impact of operational waste:
- **Operational Waste (Est.)**: Total cost of manual labor in the period.
- **Recoverable Capacity**: Monthly dollar value recoverable through automation.
- **Projected Annual Savings**: Extrapolated annual SRE budget optimization.
- **Automation Priority List**: Ranked list of categories with the highest ROI, including "Primary Contributor" services and detected error patterns.

#### ROI Engine Logic
- **Evidence-Based Recommendations**: Matches high-toil categories with specific log patterns and services.
- **Financial Attribution**: Multiplies effort by `average_hourly_cost` (configurable per profile) to provide dollar-value impact.
- **Capacity Weighted Percentage**: Global toil is calculated against the sum of all profile `team_size` and `ops_hours_per_week` to ensure the "Big Picture" remains accurate even in large organizations.
- **Self-Healing Fallback**: If capacity is not configured, the system automatically calculates toil as a percentage of "Active Operational Time" (Toil vs MTTR), ensuring the dashboard never shows an empty 0% state.

---

### Pattern Recognition & Prediction

#### Predictive Incident Prevention (`prevent`)
The `prevent` command-line suite provides proactive, ML-powered system monitoring that identifies potential incidents *before* they manifest. It uses semantic matching against historical incident "fingerprints" to surface resolution intelligence when early warning signals are detected.

For detailed command reference, see [Predictive Incident Prevention](#predictive-incident-prevention).

#### ML-Powered Analysis
```bash
# Predict incident patterns
health-monitor ml predict --text "postgres connection timeout"

# Output:
# --- Semantic Analysis ---
# Category   : database
# Pattern    : postgres_connection_timeout
# Confidence : 92.50%
# Keywords   : postgres, connection, timeout
# Suggested Actions:
#   1. Check database connectivity
#   2. Verify connection pool status
#   3. Review resource utilization
```

#### Similarity Analysis
```bash
# Find similar incidents
health-monitor ml sim --incident-id INC-20260220-123456

# Output:
# Similar Incidents:
# 1. INC-20260220-114048 (95% match)
#    - Resolved in 15 minutes
#    - Fix: Increased connection pool size
# 2. INC-20260220-111942 (87% match)
#    - Resolved in 22 minutes
#    - Fix: Database restart and optimization
```

#### Vocabulary Analysis
```bash
# Analyze error patterns
health-monitor ml vocab

# Output:
# Pattern Vocabulary:
# - postgres_connection_timeout (45 occurrences)
# - payment_gateway_error (32 occurrences)
# - authentication_failure (28 occurrences)
# - connection_refused (19 occurrences)
```

---

## Security & Authentication

### Enterprise Security Features

#### Webhook Authentication
```yaml
# profiles/security-config.yaml
security:
  webhook:
    auth:
      enabled: true
      tokens:
        - name: "prometheus"
          token: "Bearer prometheus-secret-token"
          description: "Prometheus AlertManager"
        - name: "grafana"
          token: "Bearer grafana-secret-token"
          description: "Grafana Alerts"
  
  ssl:
    verify: true
    cert_file: "/etc/ssl/certs/health-monitor.crt"
    key_file: "/etc/ssl/private/health-monitor.key"
  
  audit:
    enabled: true
    log_file: "/var/log/health-monitor/audit.log"
```

#### Security Commands
```bash
# Validate security configuration
health-monitor security validate-config --profile core-platform

# Check security status
health-monitor security status --profile core-platform

# Test security features
health-monitor security test --profile core-platform
```

#### Security Features
- **Token-Based Authentication**: Bearer token validation
- **SSL/TLS Support**: Certificate verification and mTLS
- **Audit Logging**: Comprehensive audit trail
- **Permission Validation**: Role-based access control
- **Secret Management**: Secure credential storage

---

## Multi-Profile Configuration

### Profile-Based Architecture

#### Profile Structure
```
/etc/health-monitor/
├── default.yaml              # Default configuration
├── profiles/                  # Profile directory
│   ├── core-platform.yaml    # Production environment
│   ├── alpha-us.yaml         # Alpha US environment
│   ├── beta-eu.yaml          # Beta EU environment
│   └── development.yaml      # Development environment
├── .active-profile           # Current active profile
└── state/                     # Profile state directories
    ├── core-platform/
    ├── alpha-us/
    └── beta-eu/
```

#### Profile Isolation
- **Configuration**: Separate YAML files per environment
- **State Storage**: Isolated incidents, alerts, and caches
- **Alert Listeners**: Independent webhook endpoints
- **Authentication**: Profile-specific tokens and credentials

#### Profile Management

##### Interactive Team Wizard (--wizard-team) (Recommended)
The `--wizard-team` flag launches a high-level interactive wizard tailored for specific team roles and organizational sizes. It combines the technical setup of `--init` with pre-defined "Team Presets" to provide a production-ready configuration in minutes.

- **Role-Based Presets**: Choose from predefined templates like `sre-team`, `devops-team`, or `small-team`.
- **Automated Provisioning**: Automatically sets up relevant service flows, alert rules, and SLOs for the chosen preset.
- **Guided Configuration**: Simplifies complex backend setup (Prometheus, Loki, Slack).

```bash
# Launch the Team Wizard
health-monitor --wizard-team
```

##### Interactive Setup Wizard (--init) (Advanced)

##### Command-Line Profile Management
```bash
# Switch profiles permanently
health-monitor --profile core-platform

# Use specific profile file temporarily
health-monitor --profile-path /custom/path/profile.yaml

# List available profiles
health-monitor profile list

# Create new profile manually
health-monitor profile create --name staging --template core-platform

# Validate profile
health-monitor profile validate --name core-platform
```

#### Profile Configuration
```yaml
# profiles/core-platform.yaml
profile: core-platform
region: us-central
environment: production

config:
  prometheus_url: http://prometheus.production:9090
  loki_url: http://loki.production:3100
  grafana_url: http://grafana.production:3000
  grafana_trace_ds: Tempo          # or "Jaeger"
  data_dir: /var/lib/health-monitor/core-platform
  
  # Adaptive Discovery Overrides (Optional)
  prometheus_metric_map:
    payment_requests: "custom_payment_count_metric"
    payment_latency: "custom_payment_latency_bucket"
  
  prometheus_label_map:
    payment: "my_custom_service_label"

services:
  billing_api:
    enabled: true
    port: 8081
    health_check: /health
    dependencies:
      - postgres_db
      - redis_cache

flows:
  checkout:
    name: "Checkout Flow"
    services: [billing_api, identity_service, payments_service]

security:
  webhook:
    auth:
      enabled: true
      tokens:
        - name: "prometheus"
          token: "Bearer production-token"
```

---

## Sudo & User-Mode Execution

Health-Monitor is engineered for both privileged (`sudo`) and non-privileged execution. However, **`sudo` is the recommended mode for production SRE operations**.

### Technical Reasoning for `sudo` (Recommended)

1.  **Infrastructure Discovery**: The `--init` discovery engine often requires access to system-level Kubernetes configurations (e.g., `/etc/kubernetes/admin.conf`) or privileged network sockets to probe Prometheus/Loki endpoints that may be isolated within the cluster network.
2.  **State Persistence Consistency**: In `sudo` mode, the agent uses `/var/lib/health-monitor` for incident data and `/etc/health-monitor` for configurations. These are standard system paths that ensure data persists across reboots and remains accessible to all administrative users.
3.  **Low-Level Health Diagnostics**: Many `doctor` checks (e.g., zombie process detection, deep disk I/O analysis, or network interface profiling) require kernel-level access provided only via root.
4.  **Background Reliability**: Running as `sudo` allows the agent to correctly register as a `systemd` service, ensuring a "monitor the monitor" architecture where the agent is always active.
5.  **Multi-Profile Cohesion**: `sudo` mode enforces a single source of truth for team-shared profiles, preventing "split-brain" scenarios where different users have different versions of "production" configuration in their home directories.

### Comparison Table

| Feature | Sudo (System Mode) | User (Local Mode) |
|---------|--------------------|-------------------|
| **Config Path** | `/etc/health-monitor/` | `~/.health-monitor/` |
| **State Path** | `/var/lib/health-monitor/` | `~/.health-monitor/state/` |
| **Persistence** | System-wide (Reliable) | User-specific (Transient) |
| **Infra Access**| Full (Admin level) | Restricted |
| **Best For** | Production Cluster SRE | Local Dev / Read-only Testing |

> [!IMPORTANT]
> **Production Best Practice**: Always initialize and run Health-Monitor with `sudo` for full feature parity. For EKS clusters, use `sudo -E` to ensure your AWS credential environment persists into the privileged session.

---

## Notification Integrations

### Multi-Channel Notification System

#### Supported Channels
1. **Slack Integration**
   - Channel notifications
   - Threaded discussions
   - Interactive buttons
   - File attachments

2. **PagerDuty Integration**
   - Incident creation
   - Escalation policies
   - On-call scheduling
   - Acknowledgment tracking

3. **Email Notifications**
   - SMTP configuration
   - HTML templates
   - Attachment support
   - Delivery tracking

4. **Custom Webhooks**
   - Generic HTTP endpoints
   - Custom payloads
   - Retry logic
   - Authentication headers

#### Notification Configuration
```yaml
# profiles/notifications.yaml
notifications:
  slack:
    enabled: true
    webhook_url: "https://hooks.slack.com/services/T_MOCK_DEV/B_MOCK_DEV/MOCK_WEBHOOK_KEY_SAFE"
    channel: "#incidents"
    username: "Health Monitor"
    icon_emoji: ":siren:"
    
  pagerduty:
    enabled: true
    integration_key: "integration-key-here"
    severity_mapping:
      P0: "critical"
      P1: "critical"
      P2: "high"
      P3: "medium"
      P4: "low"
    
  email:
    enabled: true
    smtp:
      host: "smtp.company.com"
      port: 587
      username: "alerts@company.com"
      password: "smtp-password"
      tls: true
    from: "Health Monitor <alerts@company.com>"
    to: ["sre-team@company.com", "oncall@company.com"]
    
  webhooks:
    - name: "custom_system"
      url: "https://custom.company.com/webhook"
      headers:
        Authorization: "Bearer custom-token"
        Content-Type: "application/json"
```

#### Notification Commands
```bash
# Test all notifications
health-monitor notifications test

# Test specific channel
health-monitor notifications test --channel slack

# Test with custom payload
health-monitor notifications test --channel webhook --payload '{"test": true}'
```

#### Notification Features
- **Template System**: Customizable message templates
- **Severity Mapping**: Configurable severity transformations
- **Rate Limiting**: Prevent notification spam
- **Delivery Tracking**: Monitor notification delivery status
- **Fallback Logic**: Multiple channel failover

---

## Metrics & Monitoring

### Performance Metrics Collection

#### Query Performance Tracking
```bash
# View query metrics
health-monitor metrics

# Output:
# 📊 Query Performance Metrics
#
# 🔍 Loki Queries:
#   Total: 1,234
#   Errors: 12
#   Avg Duration: 245ms
#   Success Rate: 99.0%
#
# 📈 Prometheus Queries:
#   Total: 567
#   Errors: 3
#   Avg Duration: 123ms
#   Success Rate: 99.5%
#
# 📋 Overall:
#   Total Queries: 1,801
#   Total Errors: 15
#   Average Latency: 189ms
#   Last Query: 2026-02-20 12:34:56
```

#### Metrics Categories
1. **System Metrics**
   - CPU utilization
   - Memory usage
   - Disk I/O
   - Network traffic

2. **Application Metrics**
   - Request rates
   - Error rates
   - Response times
   - Throughput

3. **Infrastructure Metrics**
   - Database performance
   - Cache hit rates
   - Queue depths
   - Connection pools

#### Metrics Integration
- **Prometheus**: Primary metrics source
- **Custom Metrics**: Application-specific metrics
- **Rate Limiting**: QPS limits per backend
- **Caching**: Query result caching
- **Timeouts**: Configurable query timeouts

---

---

## Tracing & Observability

### Distributed Tracing Support
Health-Monitor integrates with distributed tracing backends to correlate incidents with actual request flows and identify latency bottlenecks.

#### Supported Backends
1. **Tempo (Grafana)**: Standard support for Tempo search and retrieval.
2. **Jaeger**: Full support for Jaeger backends using service-based correlation.

#### Configuration
```yaml
config:
  grafana_trace_ds: Tempo    # "Tempo" or "Jaeger"
  loki_service_label: service,app,container,job  # Labels used for correlation
```

#### Trace Correlation
When an incident is viewed, the system automatically:
1. Identifies the affected service and time window.
2. Probes the configured tracing backend for relevant traces.
3. Extracts trace IDs and generates deep links to your Grafana or Jaeger dashboard.
4. Calculates trace-level latency to identify "Slow Traces" (e.g., > 1s).

#### Tracing Commands
```bash
# Verify tracing connectivity (via doctor)
health-monitor doctor

# Traces are automatically fetched during incident viewing:
health-monitor incident view --id <INCIDENT_ID>
```

---

## Diagnostics & Health Checks

### Comprehensive System Diagnostics

#### Doctor Command
```bash
# Run comprehensive health checks
health-monitor doctor

# Output:
# 🔍 Health Monitor Doctor
# ========================
#
# ✅ Data Directory: /var/lib/health-monitor/core-platform
# ✅ Configuration: Valid YAML syntax
# ✅ Flows: 3 flows loaded
# ✅ Alerts: Webhook endpoint reachable
# ✅ Backends: All connections successful
#
# ✅ All checks passed!
```

#### Health Check Categories
1. **Configuration Validation**
   - YAML syntax checking
   - Required field validation
   - URL reachability testing
   - Authentication verification

2. **Backend Connectivity**
   - Prometheus connection
   - Loki log access
   - Grafana dashboard access
   - Tempo trace retrieval

3. **System Resources**
   - Disk space availability
   - Memory usage
   - File permissions
   - Network connectivity

4. **Service Dependencies**
   - Database connectivity
   - Cache accessibility
   - External API reachability
   - SSL certificate validity

#### Diagnostic Features
- **Auto-Repair**: Automatic configuration fixes
- **Performance Tuning**: Optimization suggestions
- **Security Audit**: Security configuration validation
- **Upgrade Readiness**: Pre-upgrade checks

---

## Collaborative SRE Sessions (SSH Tunnels)

Collaborative SRE Sessions (Tunnels) allow real-time, multi-user cooperation during incident response. By launching an embedded SSH server, the agent allows remote teammates to join the same investigation context as the host.

### Technical Architecture

- **Embedded SSH Server**: Uses the `wish` (Bubble Tea SSH) library to host a terminal-native collaborative session.
- **Commander Pattern**: Guests interact via a `RemoteCommander` proxy that communicates directly with the host's `Service` layer.
- **Real-time Synchronization**: A broadcast system ensures that any action taken by one user (Host or Guest) immediately triggers a viewport refresh for all other participants.

| --ssh-port | 9022 | The port on which the tunnel server listens. |

**Example Command**:
```bash
sudo health-monitor incident view --tui --collaborative --ssh-port 9022 --id <INC_ID>
```

**Discovery Banner**:
The Host TUI automatically detects the primary outbound IP address and displays a prominent **High-Contrast Banner** at the top with the ready-to-use joining command.

### Security Model

- **Token-Based Authentication**: Collaborative sessions are protected by a secure, 12-character **Join Token** generated at runtime. Guests must provide this token via the SSH password prompt to authenticate.
- **End-to-End Encryption**: Leverages standard SSH (ED25519/RSA) to encrypt all terminal streams and command interactions.
- **Data Locality**: All incident data, logs, and trace signatures remain on the host machine. Guests only receive a rendered TUI stream.
- **Data Integrity**: The host's `Service` instance acts as the single source of truth for all persistence operations (Resolution, Notes, Action Items).

### NAT Traversal & Connectivity Patterns

Collaborative sessions can be established across different network topologies.

#### Pattern 1: Direct Connect (Public IP)
Used when the host has a reachable public IP (e.g., EC2 with an open SG).
- **Host**: `sudo health-monitor incident view --collaborative --ssh-port 9022`
- **Guest**: `ssh <PUBLIC_IP> -p 9022`

#### Pattern 2: Reverse SSH Tunnel (Behind NAT)
Used when the host is behind a router (e.g., local laptop) and wants to share the session with a remote server/colleague.
1. **Local Host**: Starts collaborative session locally on port 9022.
2. **Reverse Tunnel**: `ssh -R 9022:localhost:9022 <REMOTE_USER>@<REMOTE_SERVER>`
3. **Guest (on Remote Server)**: `ssh localhost -p 9022`

#### Pattern 3: Manual Host Override
If automatic public IP discovery fails (e.g., complex VPN setups), use:
`sudo health-monitor incident view --collaborative --ssh-host <SPECIFIC_IP>`

### Collaborative Commands

| Command | Category | Description |
|---------|----------|-------------|
| `view` | Navigation | Refresh the incident overview and timeline. |
| `note` | Action | Add a timestamped update with your SSH username. |
| `ack` | Action | Officially acknowledge the incident as a participant. |
| `resolve` | Action | Launch the 15-field RCA and resolution wizard. |
| `action add`| Action | Add a follow-up task to the action item list. |
| `suggest` | Intel | Trigger AI-powered runbook and remediation suggestions. |
| `similar` | Intel | Find matching historical incidents based on current symptoms. |

### Operational Integrity
The collaborative session is non-destructive to the host's state. If a guest disconnects, the host TUI remains active. When the host resolves the incident, the collaborative session is gracefully terminated for all guests.

### Overview
Predictive Prevention is a background monitoring engine that continuously scans logs, metrics, and traces across all services. When it detects patterns similar to past outages, it generates a **Prevention Prediction** containing the root cause and fix summary from the historical record.

### Daemon Management
The predictor runs as a background daemon per profile.

```bash
# Start background monitoring (ticking every 5s)
sudo ./health-monitor prevent start --profile testing --interval 5s --background

# Bootstrap with historical data (e.g. check last 15 minutes of logs on start)
sudo ./health-monitor prevent start --profile testing --backfill 15m --background

# Stop the daemon
sudo ./health-monitor prevent stop --profile testing

# Check heartbeat status
sudo ./health-monitor prevent status --profile testing
```

### Analysis & Resolution Intelligence
When the daemon finds a match, it persists a JSON record in the state directory.

```bash
# List recent discoveries
health-monitor prevent list --profile testing

# View deep dive with shared "Lessons Learned" and "Action Items"
health-monitor prevent describe PRED-123456789
```

**Intelligence Surfaced**:
- **Confidence Score**: Re-mapped 0.0 - 1.0 range based on semantic similarity.
- **Resolution Advice**: Exact fix summary used in the matching historical incident.
- **Lessons Learned**: "What Went Well" and "Where We Got Lucky" from past postmortems.
- **Action Items**: Recommended preventative tasks to take right now.
- **Correlated Evidence**: Integrated log samples and P95 latency shifts.

### Maintenance
```bash
# Purge old predictions (e.g. older than 30 days)
health-monitor prevent purge --days 30 --profile testing
```

---

## Team Presets & Setup Wizard

Health Monitor provides a powerful "Team Presets" system designed to bridge the gap between initial installation and production-ready monitoring.

### Available Team Presets

| Preset | Target Audience | Focus Areas | Complexity |
|--------|----------------|-------------|------------|
| `small-team` | 1-5 people | Critical services, simple alerts | Low |
| `medium-team` | 5-15 people | Dependency mapping, multi-service flows | Medium |
| `large-team` | 15-50 people | RBAC, Audit, complex service chains | High |
| `enterprise-team`| 50+ people | Compliance, multi-region, advanced security | High |
| `devops-team` | DevOps/Platform | CI/CD health, infrastructure utilization | High |
| `sre-team` | SRE/Reliability | SLOs, Error Budgets, UX monitoring | High |

### The Team Wizard (`--wizard-team`)

The wizard provides an interactive experience to:
1. **Identify Team Identity**: Select the most appropriate preset.
2. **Infrastructure Validation**: Test connections to Prometheus, Loki, and Grafana.
3. **Automatic Resource Creation**:
    - Generates profile-specific alert rules (`alert.d/<name>.yaml`).
    - Creates relevant service flows (`flows.d/<name>.yaml`).
    - Configures baseline SLOs.
4. **Notification Onboarding**: Streamlined Slack/PagerDuty setup with test messages.

### Key Benefits
- **Best Practices by Default**: No need to design error budget policies or flow topologies from scratch.
- **Zero-Conf Dashboards**: TUI views are immediately relevant to the selected role.
- **Consistency**: Ensures all teams in an organization follow the same monitoring standards.

---

## Advanced Features

### Automation & Scripting

#### Batch Operations
```bash
# Export all incidents
for profile in core-platform alpha-us beta-eu; do
  health-monitor --profile $profile incident export --format json
done

# Bulk resolve incidents
health-monitor incident list --state open --format json | \
  jq '.incidents[].id' | \
  xargs -I {} health-monitor incident resolve --id {} --summary "Bulk resolution"
```

#### API Integration
- **REST Endpoints**: HTTP API for external integration
- **Webhook Support**: Incoming webhook processing
- **Event Streaming**: Real-time event streaming
- **GraphQL**: Query interface for complex data retrieval

### Extensibility

#### Plugin System
- **Custom Metrics**: User-defined metric collectors
- **Custom Processors**: Log and metric processing plugins
- **Custom Notifiers**: Notification channel plugins
- **Custom Analyzers**: ML model plugins

#### Integration APIs
- **Prometheus Remote Write**: Custom metric ingestion
- **Loki API**: Advanced log querying
- **Grafana API**: Dashboard automation
- **Tempo API**: Trace data access

### Performance Optimization

#### Caching Strategies
- **Query Result Caching**: Reduce backend load
- **Configuration Caching**: Fast profile switching
- **Metadata Caching**: Service metadata optimization
- **Session Caching**: User session persistence

#### Resource Management
- **Connection Pooling**: Efficient backend connections
- **Rate Limiting**: Backend protection
- **Memory Optimization**: Efficient data structures
- **CPU Optimization**: Concurrent processing

---

## Production Deployment Guide

### System Requirements

#### Minimum Requirements
- **CPU**: 2 cores
- **Memory**: 4GB RAM
- **Disk**: 10GB available space
- **Network**: Internet connectivity for backends

#### Recommended Requirements
- **CPU**: 4+ cores
- **Memory**: 8GB+ RAM
- **Disk**: 50GB+ SSD storage
- **Network**: Low-latency connection to observability stack

### Deployment Options

#### Single Node Deployment
```bash
# Download and install
curl -LO https://releases.health-monitor.com/v1.0.0/health-monitor-linux-amd64
chmod +x health-monitor-linux-amd64
sudo mv health-monitor-linux-amd64 /usr/local/bin/health-monitor

### Monitoring & Alerting

#### Health Monitoring
- **Service Health**: `/healthz` endpoint for webhook
- **Metrics Endpoint**: `/metrics` for Prometheus
- **Readiness Probe**: `/readyz` endpoint for webhook

---

## Troubleshooting Guide

### Common Issues

#### Configuration Problems
```bash
# Check configuration syntax
health-monitor config validate

# Verify profile loading
health-monitor --profile test --help

# Debug configuration loading
HEALTH_MONITOR_DEBUG=1 health-monitor --profile core-platform
```

#### Backend Connectivity
```bash
# Test Prometheus connection
curl -H "Authorization: Bearer $TOKEN" \
  "$PROMETHEUS_URL/api/v1/query?query=up"

# Test Loki connection
curl -H "Authorization: Bearer $TOKEN" \
  "$LOKI_URL/loki/api/v1/labels"

# Run full diagnostics
health-monitor doctor
```


## Best Practices

### Configuration Management
1. **Use Profiles**: Separate environments with profiles
2. **Version Control**: Store configurations in Git
3. **Secret Management**: Use secure credential storage
4. **Validation**: Always validate configuration changes

### Incident Management
1. **Standardize RCA**: Use consistent RCA fields
2. **Pattern Recognition**: Leverage automated pattern detection
3. **Documentation**: Keep runbooks updated
4. **Review Process**: Regular incident reviews

### Performance Optimization
1. **Caching**: Enable appropriate caching
2. **Rate Limiting**: Configure backend rate limits
3. **Resource Monitoring**: Monitor resource usage
4. **Regular Maintenance**: Periodic system cleanup

### Security
1. **Authentication**: Enable webhook authentication
2. **SSL/TLS**: Use secure connections
3. **Audit Logging**: Enable audit trails
4. **Regular Updates**: Keep system updated

---

### Webhook API

#### Alert Webhook
```
POST /webhook
Content-Type: application/json
Authorization: Bearer <token>

{
  "alerts": [
    {
      "labels": {
        "alertname": "HighErrorRate",
        "service": "billing_api",
        "severity": "critical"
      },
      "annotations": {
        "summary": "High error rate detected",
        "description": "Error rate is 25% over last 5 minutes"
      }
    }
  ]
}
```

### Flow Validation (`flow validate`)

The `validate` command provides a deep sanity check for all configured flows in the current profile.

**What is Validated:**
1. **File Access**: Verifies that the loader can read all flow files (checks for permission errors).
2. **Schema Integrity**: Ensures the YAML structure matches the internal `Flow` and `SLO` models.
3. **ID Uniqueness**: Detects duplicate flow IDs across multiple files.
4. **SLO Objectives**: Validates that objectives are between 0 and 100%.
5. **Window Duration**: Parses the `window` string to ensure it's a valid Go duration (e.g., `1h`, `10m`).
6. **Query Presence**: Checks that necessary PromQL queries are defined based on the SLO type.

**Example Usage**:
```bash
sudo ./health-monitor flow validate
```
*Output*: `Flow config OK (12 flow(s) from 3 file(s))`

---

## Interactive Guided Tour & SRE Training System

The `health-monitor guide` feature is a terminal-native, interactive education and diagnostic platform designed to bridge the gap between novice users and expert SREs.

### Architecture

The guide system is built on a modular "Scenario-Chapter-Step" architecture powered by the `bubbletea` TUI framework:

- **Scenario**: A high-level learning track (e.g., Novice Onboarding, SRE Labs).
- **Chapter**: A logical grouping of related steps within a scenario.
- **Step**: An individual interactive unit containing content, keywords, hints, and optional automated actions.
- **Seeder Interface**: Dynamically modifies the local sandbox environment to simulate real-world failure modes (e.g., "Label Drift" or "Cascading Latency").
- **Unified Engine**: Manages state persistence, contextual search, and interactive remediation pipelines.

### Key Learning Tracks

#### 1. 🆕 The Novice: Getting Started
Focused on the critical first 5 minutes of the user journey.
- **Deep Dive**: Explains the technical differences between `--init` (Discovery Engine) and `--wizard-team` (O11y Templates).
- **Validation Loop**: Teaches users how to use `doctor`, `flow validate`, and `alert validate-config` to ensure a healthy initial state.

#### 2. 🧪 SRE Training Labs
"Learn-by-doing" simulations that use the internal seeder to create safe, reproducible outages.
- **Label Mystery**: Simulates metric label drift where Prometheus metrics use `app` while the config expects `service`. Users must use the Troubleshooter to identify and fix the mismatch.
- **Cascading Failure**: Injects latency into downstream dependencies (e.g., `payment-svc`) to teach users how to trace ripple effects through the API gateway.
- **Incident Replay**: Loads historical high-severity snapshots into the active session for practice under pressure.

#### 3. 📚 Command Encyclopedia
A comprehensive, searchable index of all 40+ commands and flags, classified for quick reference:
- **Setup & Discovery**: `profile`, `init`, `doctor`.
- **Incident & Reliability**: `incident`, `flow`, `slo`, `monitoring`.
- **Governance**: `scorecard`, `toil`, `prevent`.
- **Collaboration**: `tunnel`, `feedback`.

### Dynamic Troubleshooter & Remediation

The Troubleshooter is a proactive diagnostic engine that analyzes the host environment in real-time.

**Diagnostic Pipeline:**
- **Permissions**: Analyzes current UID and directory access (e.g., verifying `/etc` read/write).
- **Backend Probing**: Performed deep HTTP probes on Prometheus and Loki, including querying for recent errors to provide context.
- **Daemon Vitality**: Cross-references port occupancy (9095/9098) with PID file integrity in `/run/`.
- **Configuration Drift**: Compares the active profile against "Golden Signal" templates to find missing reliability safeguards.

**Remediation Modes:**
- **Instructional**: Provides the exact CLI command to fix the issue.
- **Automated**: Offers a one-click "Apply Fix" within the TUI (e.g., automatically updating a URL or enabling notifications).
- **Interactive**: Prompts for required parameters (e.g., Slack Webhook URL) and securely persists them to the profile state.

### User Value Proposition

- **For New Users**: Reduces the "blank slate" anxiety by providing a curated tour of features and immediate validation of their setup.
- **For Intermediate Users**: Provides a safe "shooting range" to practice complex SRE skills like trace analysis and root cause identification without risking production uptime.

---

## Conclusion

Health-Monitor provides a comprehensive SRE toolkit that combines observability, incident management, and automated response capabilities. Its modular architecture, multi-profile support, and extensive integration options make it suitable for organizations of all sizes.

Key strengths include:
- **Unified Interface**: Single tool for multiple SRE functions
- **Automation**: ML-powered pattern recognition and runbook generation
- **Flexibility**: Multi-profile configuration and extensive integrations
- **Enterprise Ready**: Security, audit, and compliance features
- **Performance**: Optimized for large-scale deployments

The system is designed to grow with your organization's needs, from single-service monitoring to complex multi-environment deployments. With proper configuration and following the best practices outlined in this guide, Health-Monitor can significantly improve your operational efficiency and incident response capabilities.

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


