# Profile-Based Configuration Guide

## � Profile Creation Methods

Health Monitor provides **two primary methods** to create profiles - each generates the same file structure but with different approaches:

### Method 1: Dynamic Discovery (`--init`) - **Production Recommended**

**Creates profiles based on your actual infrastructure**

```bash
# EKS Clusters (CRITICAL: Use -E for AWS credentials)
sudo -E ./health-monitor --init --infra eks --kubeconfig ~/.kube/config --profile my-production-cluster

# Local Kubernetes
sudo ./health-monitor --init --infra kubernetes --kubeconfig ~/.kube/config --profile my-local-cluster

# Default profile name (timestamp-based)
sudo -E ./health-monitor --init --infra eks
```

**Generated Files**:
```
/etc/health-monitor/profiles/my-production-cluster.yaml    # Main configuration
/etc/health-monitor/flows.d/my-production-cluster.yaml      # Discovered SLO flows
/etc/health-monitor/alerts.d/my-production-cluster.yaml     # Alert rules
/etc/health-monitor/state/my-production-cluster/            # Isolated state
```

**Key Advantages**:
- ✅ **Accurate Service Discovery**: Scans your actual Kubernetes services
- ✅ **Real Metric Patterns**: Discovers your Prometheus metrics (`app_*`, `http_*`, etc.)
- ✅ **Validated Queries**: Tests that queries return actual data
- ✅ **Production Ready**: No manual tuning required
- ✅ **Custom Metrics**: Supports any metric pattern in your infrastructure

**Process**:
1. **Kubernetes Scan**: Discovers all services in your namespaces
2. **Metric Analysis**: Finds actual metric names and patterns
3. **Query Testing**: Validates each metric returns data
4. **Flow Generation**: Creates SLO flows based on working metrics
5. **Profile Creation**: Builds complete configuration

### Method 2: Team Presets (`--wizard-team`) - **Quick Setup**

**Creates profiles from predefined templates**

```bash
# Interactive team setup
sudo ./health-monitor --wizard-team --profile my-team-preset

# Non-interactive with specific preset
sudo ./health-monitor --wizard-team --preset medium-team --profile my-medium-team

# Quick default setup
sudo ./health-monitor --wizard-team
```

**Generated Files**:
```
/etc/health-monitor/profiles/my-team-preset.yaml    # Template-based configuration
/etc/health-monitor/flows.d/my-team-preset.yaml      # Standard SLO flows
/etc/health-monitor/alerts.d/my-team-preset.yaml     # Template alert rules
/etc/health-monitor/state/my-team-preset/            # Isolated state
```

**Key Advantages**:
- ⚡ **Lightning Fast**: 10-30 seconds vs 2-5 minutes
- 🚀 **No Infrastructure Required**: Works without cluster access
- 📋 **Standardized**: Consistent configurations across teams
- 🎯 **Team Optimized**: Pre-configured for common patterns
- 🔧 **Easy to Customize**: Start with templates, modify as needed

**Available Presets**:
- `small-team` - 2-5 people, essential monitoring
- `medium-team` - 5-15 people, multi-service flows
- `large-team` - 15-50 people, advanced security
- `enterprise-team` - 50+ people, compliance focus
- `devops-team` - CI/CD and infrastructure focus
- `sre-team` - Advanced SLOs and error budgets

### 📊 Method Comparison

| Aspect | `--init` (Discovery) | `--wizard-team` (Presets) |
|--------|----------------------|---------------------------|
| **Setup Time** | 2-5 minutes | 10-30 seconds |
| **Infrastructure Access** | Required | Not required |
| **Metric Accuracy** | High (discovered) | Medium (standard) |
| **Custom Metrics Support** | ✅ Automatic | ⚠️ Manual setup |
| **Production Ready** | ✅ Yes | 🔧 May need tuning |
| **Query Validation** | ✅ Automatic | ❌ Manual verification |
| **Best For** | Production, custom setups | Demos, testing, standards |

### 🎯 Choosing the Right Method

#### Use `--init` When:
- ✅ **Production Environments**: You need accurate, reliable monitoring
- ✅ **Custom Metrics**: Your services use non-standard metric patterns
- ✅ **Existing Infrastructure**: You have Kubernetes services to discover
- ✅ **Accuracy Required**: You can't afford query failures or missing data
- ✅ **Complex Setups**: Multi-service, multi-namespace architectures

#### Use `--wizard-team` When:
- ✅ **Quick Demos**: You need fast setup for presentations
- ✅ **Team Standardization**: You want consistent configurations
- ✅ **Testing/Evaluation**: You're trying out the tool
- ✅ **Standard Architectures**: You use common metric patterns
- ✅ **Infrastructure Limitations**: You can't access the cluster yet

### 🔄 Hybrid Workflow (Recommended for Teams)

**Step 1: Quick Start with Presets**
```bash
# Fast initial setup
sudo ./health-monitor --wizard-team --preset medium-team --profile demo
```

**Step 2: Test and Validate**
```bash
# Verify the setup works
sudo ./health-monitor --profile demo slo list
sudo ./health-monitor --profile demo
```

**Step 3: Production Upgrade with Discovery**
```bash
# Create production-ready profile
sudo -E ./health-monitor --init --infra eks --profile production
```

**Step 4: Compare and Merge**
```bash
# Review both profiles
sudo cat /etc/health-monitor/profiles/demo.yaml
sudo cat /etc/health-monitor/profiles/production.yaml

# Manually merge customizations if needed
```

### 🛠️ Profile Customization

Both methods generate **editable YAML files** that you can customize:

#### Profile Configuration (`/etc/health-monitor/profiles/{name}.yaml`)
```yaml
name: production-cluster
provider: eks
config:
  prometheus_url: "http://localhost:9090"        # Update your Prometheus URL
  eks:
    enabled: true
    region: "us-west-2"                         # Update your region
    kubeconfig: "/home/user/.kube/config"       # Update path
    namespaces: ["default", "production"]       # Specify namespaces
```

#### Flow Configuration (`/etc/health-monitor/flows.d/{name}.yaml`)
```yaml
flows:
  my-service:
    name: "My Service Flow"
    services: ["my-service"]
    slos:
      - id: "success_rate"
        objective: 99.5                          # Adjust objective
        window: "5m"                            # Adjust window
        error_query: "sum(rate(my_requests_total{status!~\"2..\"}[5m]))"
        total_query: "sum(rate(my_requests_total[5m]))"
```

#### Alert Configuration (`/etc/health-monitor/alerts.d/{name}.yaml`)
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

### 🚨 Critical AWS Integration Note

**For EKS clusters, ALWAYS use the `-E` flag**:

```bash
# ❌ WRONG - AWS credentials lost
sudo ./health-monitor --init --infra eks

# ✅ CORRECT - AWS credentials preserved
sudo -E ./health-monitor --init --infra eks
```

**Required AWS Environment Variables**:
```bash
export AWS_REGION=us-west-2
export AWS_ACCESS_KEY_ID=your-access-key
export AWS_SECRET_ACCESS_KEY=your-secret-key
export AWS_SESSION_TOKEN=your-session-token  # if using temporary credentials
```

---

## �🚨 CRITICAL: File Naming Conventions

**The profile system requires STRICT naming consistency** across all configuration files. The system uses the profile name to construct file paths, so inconsistent naming will cause configuration loading failures.

### 📋 **Required Naming Convention**

For a profile named `alpha-us`, the files MUST be named exactly as follows:

#### **Option 1: profiles/ Directory (Recommended)**
```
/etc/health-monitor/profiles/
├── alpha-us.yaml              # Profile definition
├── alpha-us-flows.yaml        # Flow configuration  
├── alpha-us-alerts.yaml       # Alert configuration
├── beta-eu.yaml               # Profile definition
├── beta-eu-flows.yaml         # Flow configuration
├── beta-eu-alerts.yaml        # Alert configuration
├── core-platform.yaml         # Profile definition
├── core-platform-flows.yaml   # Flow configuration
└── core-platform-alerts.yaml  # Alert configuration
```

#### **Option 2: Separate directories (Alternative)**
```
/etc/health-monitor/profiles/           # Profile definitions only
├── alpha-us.yaml
├── beta-eu.yaml
└── core-platform.yaml

/etc/health-monitor/flows.d/            # Flow configurations
├── alpha-us.yaml
├── beta-eu.yaml
└── core-platform.yaml

/etc/health-monitor/alerts.d/           # Alert configurations
├── alpha-us.yaml
├── beta-eu.yaml
└── core-platform.yaml
```

### 🎯 **File Resolution Priority**

The system searches for files in this order:

#### **Flows Configuration:**
1. `profiles/{profile-name}-flows.yaml` (highest priority)
2. `flows.d/{profile-name}.yaml` (secondary)
3. `flows.yaml` (fallback - used for all profiles)

#### **Alerts Configuration:**
1. `profiles/{profile-name}-alerts.yaml` (highest priority)
2. `alerts.d/{profile-name}.yaml` (secondary)
3. `alerts.yaml` (fallback - used for all profiles)

#### **Profile Definition:**
1. `profiles/{profile-name}.yaml` (only location)

### ⚠️ **Critical Rules**

✅ **Profile name must be identical** across all file types  
✅ **Case sensitivity matters**: `Alpha-US` ≠ `alpha-us`  
✅ **Use hyphens, not underscores**: `alpha-us` not `alpha_us`  
✅ **All files must be `.yaml` extension**  
✅ **No spaces in profile names**: `prod-env` not `prod env`  

### ❌ **Common Mistakes to Avoid**

```
# WRONG - Inconsistent naming
profiles/alpha-us.yaml
flows.d/alpha.yaml          # Should be alpha-us.yaml
alerts.d/alpha-us-alerts.yaml

# WRONG - Mixed case
profiles/Alpha-US.yaml
flows.d/alpha-us-flows.yaml

# WRONG - Wrong extension
profiles/alpha-us.yml
flows.d/alpha-us-flows.yml
```

## Overview

The health-monitor now supports **profile-based configuration** for complete multi-tenant isolation. Each profile maintains its own:
- Flows configuration
- Alert rules  
- Incidents
- Alert listeners
- Data directories

## 🚀 Quick Start

### 1. Create Profile Configuration Files

#### **Profile Definition Structure**
Create `/etc/health-monitor/profiles/{profile-name}.yaml`:

```yaml
# Profile definition for alpha-us
name: "Alpha US Environment"
description: "Production environment for US Alpha region"
base_url: "https://alpha-us.monitor.local"
timezone: "UTC"
grafana_url: "https://alpha-us.grafana.local"
prometheus_url: "https://alpha-us.prometheus.local"
loki_url: "https://alpha-us.loki.local"
tempo_url: "https://alpha-us.tempo.local"

# Optional: Profile-specific settings
settings:
  default_severity: "P2"
  auto_acknowledge: false
  notification_channels: ["slack", "pagerduty"]
```

#### **Flow Configuration Structure**
Create `/etc/health-monitor/profiles/{profile-name}-flows.yaml`:

```yaml
flows:
  checkout:
    name: "Checkout Process"
    description: "Complete checkout flow from cart to payment"
    services: 
      - billing_api
      - payment_service
      - inventory_service
      - notification_service
    slos:
      - id: checkout_success
        objective: 90
        window: 5m
        type: ratio
        error_query: 'http_requests_total{service="billing_api",status!~"2.."}'
        total_query: 'http_requests_total{service="billing_api"}'
        description: "Percentage of successful checkout requests"
      
      - id: checkout_latency
        objective: 95
        window: 5m
        type: latency
        latency_query: 'histogram_quantile(0.95, rate(http_request_duration_seconds_bucket{service="billing_api"}[5m]))'
        threshold: 2.0
        description: "95th percentile latency under 2 seconds"

  user_auth:
    name: "User Authentication"
    description: "User login and authentication flow"
    services: 
      - identity_service
      - auth_service
      - session_service
    slos:
      - id: auth_success
        objective: 95
        window: 5m
        type: ratio
        error_query: 'http_requests_total{service="identity_service",status!~"2.."}'
        total_query: 'http_requests_total{service="identity_service"}'
        description: "Percentage of successful authentication requests"

  order_processing:
    name: "Order Processing"
    description: "Order creation and processing workflow"
    services:
      - order_service
      - inventory_service
      - shipping_service
    slos:
      - id: order_success
        objective: 92
        window: 10m
        type: ratio
        error_query: 'http_requests_total{service="order_service",status!~"2.."}'
        total_query: 'http_requests_total{service="order_service"}'
        description: "Percentage of successful orders"
```

#### **Alert Configuration Structure**
Create `/etc/health-monitor/profiles/{profile-name}-alerts.yaml`:

```yaml
alerts:
  # Specific alert rules (require exact alertname match)
  - match:
      service: billing_api
      alertname: BillingAPIHighErrorRate
    severity: P1
    mode: auto
    title: "Billing API High Error Rate"
    description: "Billing API is experiencing elevated error rates"
    runbook: "https://runbooks.company.com/billing-api-errors"
    
  - match:
      service: payment_service
      alertname: PaymentServiceDown
    severity: P1
    mode: auto
    title: "Payment Service Down"
    description: "Payment service is not responding"
    runbook: "https://runbooks.company.com/payment-service-down"

  - match:
      service: identity_service
      alertname: IdentityServiceLatency
    severity: P2
    mode: auto
    title: "Identity Service High Latency"
    description: "Identity service response times are elevated"
    runbook: "https://runbooks.company.com/identity-service-latency"

  # Generic alert rules (match any alertname for the service)
  - match:
      service: billing_api
    severity: P2
    mode: auto
    title: "Billing Service Alert"
    description: "Generic alert for billing service issues"

  - match:
      service: payment_service
    severity: P2
    mode: auto
    title: "Payment Service Alert"
    description: "Generic alert for payment service issues"

  - match:
      service: identity_service
    severity: P2
    mode: auto
    title: "Identity Service Alert"
    description: "Generic alert for identity service issues"
```

### **🚨 CRITICAL: Alert Rule Requirements**

**Every alert rule MUST include:**

1. **match section** - Defines matching criteria
2. **alertname in match** - Required for specific rules (optional for generic)
3. **severity** - P1, P2, P3, or P4
4. **mode** - "auto" or "manual"
5. **title** - Human-readable title

### **📋 Complete Alert Rule Syntax**

```yaml
alerts:
  # Specific rule - matches exact alertname + service
  - match:
      service: "billing_api"        # Required: Service name
      alertname: "BillingAPIIssue"  # Required: Alert name
      # Optional additional labels:
      # environment: "production"
      # team: "payments"
    severity: "P1"                   # Required: P1, P2, P3, P4
    mode: "auto"                     # Required: "auto" or "manual"
    title: "Billing API Issue"       # Required: Human-readable title
    description: "Optional detailed description"
    runbook: "https://runbooks.company.com/billing"
    
  # Generic rule - matches any alertname for the service
  - match:
      service: "billing_api"         # Only service required
      # No alertname = matches any alertname for this service
    severity: "P2"
    mode: "auto"
    title: "Billing Service Alert"
    description: "Generic alert for billing service"
```

### **📊 Complete Flow Rule Syntax**

```yaml
flows:
  flow_id:                          # Required: Unique flow identifier
    name: "Human Readable Name"     # Required: Display name
    description: "Detailed description of what this flow does"
    services:                       # Required: List of services in this flow
      - service_name_1
      - service_name_2
      - service_name_3
    slos:                          # Required: At least one SLO
      - id: "slo_identifier"       # Required: Unique SLO ID within flow
        objective: 90              # Required: Target percentage (0-100)
        window: "5m"               # Required: Time window (1m, 5m, 15m, 1h)
        type: "ratio"              # Required: "ratio" or "latency"
        
        # For ratio type:
        error_query: 'prometheus_error_query'
        total_query: 'prometheus_total_query'
        
        # For latency type:
        latency_query: 'histogram_quantile_query'
        threshold: 2.0              # Required for latency: threshold value
        
        description: "What this SLO measures"
```

### **🏗️ Complete Profile Definition Syntax**

```yaml
# Required fields
name: "Profile Display Name"        # Required: Human-readable name
description: "Profile description"  # Required: What this profile represents

# Optional observability URLs
base_url: "https://monitor.company.com"
grafana_url: "https://grafana.company.com"
prometheus_url: "https://prometheus.company.com"
loki_url: "https://loki.company.com"
tempo_url: "https://tempo.company.com"

# Optional settings
timezone: "UTC"                    # Default: UTC
settings:
  default_severity: "P2"           # Default severity for manual incidents
  auto_acknowledge: false          # Auto-acknowledge webhook incidents
  notification_channels:           # Default notification channels
    - "slack"
    - "pagerduty"
    - "email"
```

## 2. Profile Management Commands

```bash
# List available profiles
sudo ./health-monitor profile list

# Switch active profile
sudo ./health-monitor profile switch alpha-us

# View current profile
sudo ./health-monitor profile current

# Use temporary profile override
sudo ./health-monitor --profile beta-eu incident list
```

### 3. Profile-Aware Operations

#### Incident Management (One Per Profile)
```bash
# Create incident in specific profile
sudo ./health-monitor --profile alpha-us incident start --service billing_api --severity P1 --title "Alpha US Issue"

# List incidents in specific profile  
sudo ./health-monitor --profile alpha-us incident list

# View incident details with profile info
sudo ./health-monitor --profile alpha-us incident view --id INC-20260219-053504
```

#### Flow Management
```bash
# List flows for specific profile
sudo ./health-monitor --profile alpha-us flow list

# Each profile shows different flows based on its configuration
sudo ./health-monitor --profile beta-eu flow list
sudo ./health-monitor --profile core-platform flow list
```

#### Alert Management
```bash
# List alert rules for specific profile
sudo ./health-monitor --profile alpha-us alert list-rules

# Validate alert configuration
sudo ./health-monitor --profile alpha-us alert validate-config

# Start profile-specific alert listener
sudo ./health-monitor --profile alpha-us alert listen --background --addr :9095 --pid-file /run/health-monitor-alert-alpha.pid --data-dir /var/lib/health-monitor/state/alpha-us
```

## 🔧 Advanced Configuration

### File Location Priority

Profile configuration files are searched in this order:

1. **Primary**: `/etc/health-monitor/profiles/<profile-name>-flows.yaml`
2. **Secondary**: `/etc/health-monitor/flows.d/<profile-name>.yaml`
3. **Fallback**: Default flow paths

Same pattern applies to alerts (`<profile-name>-alerts.yaml`).

### Multi-Listener Setup

Each profile can run its own alert listener:

```bash
# Start multiple profile listeners
sudo ./health-monitor --profile alpha-us alert-listen --background --addr :9095 --pid-file /run/health-monitor-alert-alpha.pid --data-dir /var/lib/health-monitor/state/alpha-us
sudo ./health-monitor --profile beta-eu alert-listen --background --addr :9097 --pid-file /run/health-monitor-alert-beta.pid --data-dir /var/lib/health-monitor/state/beta-eu
sudo ./health-monitor --profile core-platform alert-listen --background --addr :9098 --pid-file /run/health-monitor-alert-core.pid --data-dir /var/lib/health-monitor/state/core-platform

# Check listener status
sudo ./health-monitor alert-status --pid-file /run/health-monitor-alert-alpha.pid
sudo ./health-monitor alert-status --pid-file /run/health-monitor-alert-beta.pid
sudo ./health-monitor alert-status --pid-file /run/health-monitor-alert-core.pid
```

### Webhook Integration

Send webhooks to specific profile listeners:

```bash
# Alpha US webhook (port 9095)
curl -X POST http://localhost:9095/webhook \
  -H "Content-Type: application/json" \
  -d '{
    "receiver": "test",
    "status": "firing",
    "alerts": [{
      "status": "firing",
      "labels": {
        "alertname": "BillingAPIIssue",
        "service": "billing_api"
      },
      "annotations": {
        "summary": "Billing API alert from Alpha US"
      }
    }]
  }'

# Beta EU webhook (port 9097)
curl -X POST http://localhost:9097/webhook \
  -H "Content-Type: application/json" \
  -d '{
    "receiver": "test", 
    "status": "firing",
    "alerts": [{
      "status": "firing",
      "labels": {
        "alertname": "OrderServiceIssue",
        "service": "order_service"
      }
    }]
  }'
```

## 📊 Monitoring & Metrics

### Health Checks
```bash
# Check listener health
curl http://localhost:9095/readyz  # Alpha US
curl http://localhost:9097/readyz  # Beta EU
curl http://localhost:9098/readyz  # Core Platform
```

### Metrics
```bash
# Get detailed metrics for each listener
curl http://localhost:9095/metrics
curl http://localhost:9097/metrics  
curl http://localhost:9098/metrics
```

Metrics include:
- `alerts_received_total` - Total webhooks received
- `alerts_created_total` - Incidents created from alerts
- `alerts_failed_total` - Failed alert processing
- `incidents_active_total` - Active incidents per profile
- `listener_uptime_seconds` - Listener uptime
- `flows_loaded` - Flow configuration loaded successfully
- `alerts_loaded` - Alert configuration loaded successfully

## 🎯 Profile Isolation Features

### Incident Isolation
- **One active incident per profile** (not global)
- Manual incidents: One per profile
- Webhook incidents: Follow alert rules per profile
- Complete data separation

### Configuration Isolation  
- Separate flow definitions per profile
- Separate alert rules per profile
- Independent validation per profile
- No cross-profile interference

### Operational Isolation
- Independent alert listeners
- Separate data directories
- Individual metrics endpoints
- Profile-specific webhook routing

## 🛠️ Troubleshooting

### Common Issues

#### "invalid alert config — skipping"
**Cause**: Missing `alertname` field in alert rule match section
**Fix**: Add `alertname` to each alert rule:
```yaml
- match:
    service: billing_api
    alertname: BillingAPIIssue  # Required!
```

#### "Flow not found"  
**Cause**: Profile context not initialized or flow file missing
**Fix**: Ensure flow file exists and profile context is set:
```bash
sudo ./health-monitor --profile your-profile flow list
```

#### "an active incident already exists"
**Cause**: Profile-specific incident limit (this is correct behavior)
**Fix**: Resolve existing incident or use different profile

#### Webhook not creating incidents
**Cause**: Alert rule mismatch or wrong port
**Fix**: 
1. Check alert rules: `sudo ./health-monitor --profile profile-name alert list-rules`
2. Match webhook labels exactly with alert rule match criteria
3. Use correct listener port

#### Configuration file not found
**Cause**: Incorrect file naming convention
**Fix**: Ensure files follow the naming convention:
```
profiles/alpha-us.yaml           # Profile definition
profiles/alpha-us-flows.yaml     # Flow configuration
profiles/alpha-us-alerts.yaml    # Alert configuration
```

### Validation Commands

```bash
# Validate all profile configurations
for profile in alpha-us beta-eu core-platform; do
  echo "=== $profile ==="
  sudo ./health-monitor --profile $profile alert validate-config
  sudo ./health-monitor --profile $profile flow list
done

# Check profile isolation
sudo ./health-monitor --profile alpha-us incident list
sudo ./health-monitor --profile beta-eu incident list  
sudo ./health-monitor --profile core-platform incident list
```

## 📁 Complete File Structure Example

```
/etc/health-monitor/
├── profiles/
│   ├── alpha-us.yaml              # Profile definition
│   ├── alpha-us-flows.yaml        # Flow configuration
│   ├── alpha-us-alerts.yaml       # Alert configuration
│   ├── beta-eu.yaml               # Profile definition
│   ├── beta-eu-flows.yaml         # Flow configuration
│   ├── beta-eu-alerts.yaml        # Alert configuration
│   ├── core-platform.yaml         # Profile definition
│   ├── core-platform-flows.yaml   # Flow configuration
│   ├── core-platform-alerts.yaml  # Alert configuration
│   └── default.yaml               # Default profile
├── flows.d/                       # Alternative location for flows
│   ├── alpha-us.yaml
│   ├── beta-eu.yaml
│   └── core-platform.yaml
├── alerts.d/                      # Alternative location for alerts
│   ├── alpha-us.yaml
│   ├── beta-eu.yaml
│   └── core-platform.yaml
└── config.yaml                    # Global configuration

/var/lib/health-monitor/state/
├── alpha-us/
│   └── incidents/
├── beta-eu/
│   └── incidents/
├── core-platform/
│   └── incidents/
└── default/
    └── incidents/
```

## 🔄 Migration from Global Configuration

To migrate existing setup to profile-based:

1. **Create profile directories**:
   ```bash
   sudo mkdir -p /etc/health-monitor/profiles
   sudo mkdir -p /var/lib/health-monitor/state/alpha-us
   ```

2. **Move existing configurations**:
   ```bash
   sudo cp /etc/health-monitor/flows.yaml /etc/health-monitor/profiles/alpha-us-flows.yaml
   sudo cp /etc/health-monitor/alerts.yaml /etc/health-monitor/profiles/alpha-us-alerts.yaml
   ```

3. **Update alert rules** to include `alertname` field
4. **Test profile functionality**:
   ```bash
   sudo ./health-monitor --profile alpha-us flow list
   sudo ./health-monitor --profile alpha-us alert list-rules
   ```

## 🚀 Production Deployment

### Environment Setup
```bash
# Set profile paths
export HEALTH_MONITOR_PROFILE_PATH=/etc/health-monitor/profiles

# Initialize profile system
sudo ./health-monitor profile switch alpha-us
```

### Service Configuration
```bash
# Systemd service for each profile listener
sudo systemctl enable health-monitor-alert-alpha-us
sudo systemctl enable health-monitor-alert-beta-eu
sudo systemctl enable health-monitor-alert-core-platform

# Start services
sudo systemctl start health-monitor-alert-alpha-us
sudo systemctl start health-monitor-alert-beta-eu  
sudo systemctl start health-monitor-alert-core-platform
```

This profile system enables true multi-tenant operation with complete isolation between teams, environments, or business units while maintaining a single health-monitor installation.

## 📚 Quick Reference

### **File Naming Summary**
| Profile Name | Profile File | Flow File | Alert File |
|--------------|--------------|-----------|------------|
| alpha-us | `alpha-us.yaml` | `alpha-us-flows.yaml` | `alpha-us-alerts.yaml` |
| beta-eu | `beta-eu.yaml` | `beta-eu-flows.yaml` | `beta-eu-alerts.yaml` |
| prod | `prod.yaml` | `prod-flows.yaml` | `prod-alerts.yaml` |

### **Required Fields Summary**
- **Profile**: `name`, `description`
- **Flow**: `name`, `services`, `slos` (with `id`, `objective`, `window`, `type`)
- **Alert**: `match`, `severity`, `mode`, `title`
- **Alert Match**: `service` (required), `alertname` (optional for generic rules)

### **Severity Levels**
- `P1` - Critical (business impact)
- `P2` - High (significant impact)
- `P3` - Medium (limited impact)
- `P4` - Low (minimal impact)

### **SLO Types**
- `ratio` - Success/error ratio (requires error_query, total_query)
- `latency` - Latency percentile (requires latency_query, threshold)

## 🔔 **Notification Setup & Management**

Health-Monitor supports **Slack and PagerDuty notifications** for real-time incident alerts. Notifications are sent automatically for incident lifecycle events.

### **🎯 Supported Notification Channels**

#### **1. Slack Notifications**
- **Type**: Incoming Webhooks
- **Events**: start, suggest, acknowledge, resolve
- **Format**: Professional messages with severity emojis
- **Behavior**: Non-blocking, async with retry logic

#### **2. PagerDuty Notifications**
- **Type**: Events API v2
- **Events**: start, suggest, acknowledge, resolve
- **Format**: Standard PagerDuty incident format
- **Behavior**: Non-blocking, async with retry logic

### **📋 Notification Configuration Structure**

#### **Profile-Based Notification Config**
Add to your profile definition (`/etc/health-monitor/profiles/{profile-name}.yaml`):

```yaml
# Profile definition with notifications
name: "Alpha US Environment"
description: "Production environment for US Alpha region"
base_url: "https://alpha-us.monitor.local"
grafana_url: "https://alpha-us.grafana.local"
prometheus_url: "https://alpha-us.prometheus.local"

# Notification settings
notifications:
  enabled: true
  notify_on: ["start", "suggest", "ack", "resolve"]  # Events to notify on
  
  # Slack configuration
  slack:
    enabled: true
    webhook_url: "https://hooks.slack.com/services/T_MOCK_DEV/B_MOCK_DEV/MOCK_WEBHOOK_KEY_SAFE"
    timeout_seconds: 3
    max_retries: 3
  
  # PagerDuty configuration
  pagerduty:
    enabled: true
    routing_key: "R0XXXXXXXXXXXXX"  # Events API integration key
    severity_map:
      P1: "critical"
      P2: "high"
      P3: "medium"
      P4: "low"
    timeout_seconds: 3
    max_retries: 3
```

#### **Global Notification Config**
Or use global config (`/etc/health-monitor/config.json`):

```json
{
  "notifications": {
    "enabled": true,
    "notify_on": ["start", "suggest", "ack", "resolve"],
    "slack": {
      "enabled": true,
      "webhook_url": "https://hooks.slack.com/services/T_MOCK_DEV/B_MOCK_DEV/MOCK_WEBHOOK_KEY_SAFE",
      "timeout_seconds": 3,
      "max_retries": 3
    },
    "pagerduty": {
      "enabled": true,
      "routing_key": "R0XXXXXXXXXXXXX",
      "severity_map": {
        "P1": "critical",
        "P2": "high", 
        "P3": "medium",
        "P4": "low"
      },
      "timeout_seconds": 3,
      "max_retries": 3
    }
  }
}
```

### **🔧 Notification Management Commands**

#### **Setup Commands**
```bash
# Enable Slack notifications (interactive setup)
sudo ./health-monitor notifications enable slack
# Prompts for webhook URL, tests connection, saves config

# Enable PagerDuty notifications (interactive setup)
sudo ./health-monitor notifications enable pagerduty
# Prompts for routing key, tests connection, saves config

# Set PagerDuty routing key directly
sudo ./health-monitor notifications pagerduty set-key R0XXXXXXXXXXXXX

# Profile-specific notification setup
sudo ./health-monitor --profile alpha-us notifications enable slack
sudo ./health-monitor --profile beta-eu notifications enable pagerduty
```

#### **Management Commands**
```bash
# Disable notifications (keeps configuration)
sudo ./health-monitor notifications disable slack
sudo ./health-monitor notifications disable pagerduty

# Profile-specific disable
sudo ./health-monitor --profile alpha-us notifications disable slack

# Check notification status
sudo ./health-monitor notifications status

# Profile-specific status
sudo ./health-monitor --profile alpha-us notifications status
```

#### **Testing Commands**
```bash
# Send test notification
sudo ./health-monitor notifications test slack
sudo ./health-monitor notifications test pagerduty

# Profile-specific test
sudo ./health-monitor --profile alpha-us notifications test slack
```

### **📊 Status Check Output**

#### **Example Status Output**
```bash
$ sudo ./health-monitor notifications status

Profile: alpha-us
Notifications Status:
  Slack: enabled
    Webhook: configured
  PagerDuty: disabled
    Routing Key: not configured
  Events: start, suggest, ack, resolve
```

#### **Profile-Specific Status**
```bash
$ sudo ./health-monitor --profile beta-eu notifications status

Profile: beta-eu
Notifications Status:
  Slack: disabled
    Webhook: not configured
  PagerDuty: enabled
    Routing Key: configured
  Events: start, suggest, ack, resolve
```

### **🔔 Notification Events**

#### **Automatic Notifications**
Notifications are sent automatically for these incident events:

1. **incident.started** - New incident created (manual or webhook)
2. **incident.suggested** - Suggested incident created from alert
3. **incident.acknowledged** - Incident acknowledged by user
4. **incident.resolved** - Incident resolved with summary

#### **No Notification Events**
- **Deduped alerts** - Duplicate webhooks don't trigger notifications
- **Incident updates** - Only state changes trigger notifications
- **Failed notifications** - Errors are logged but don't block operations

### **📱 Slack Message Format**

#### **Example Slack Notification**
```
🚨 Incident Started: INC-20260219-054214
Service: billing_api | Severity: P1 | Profile: alpha-us
Title: Alpha US - Checkout Incident

📊 Observability Links:
• Grafana: https://alpha-us.grafana.local/d/...
• Prometheus: https://alpha-us.prometheus.local/...
• Logs: https://alpha-us.loki.local/...
• Traces: https://alpha-us.tempo.local/...
```

#### **Severity Emojis**
- 🚨 P1 - Critical (business impact)
- ⚠️ P2 - High (significant impact)  
- ℹ️ P3 - Medium (limited impact)
- 📝 P4 - Low (minimal impact)

### **🚨 PagerDuty Message Format**

#### **PagerDuty Event Structure**
```json
{
  "routing_key": "R0XXXXXXXXXXXXX",
  "event_action": "trigger",
  "payload": {
    "summary": "Alpha US - Checkout Incident",
    "source": "health-monitor",
    "severity": "critical",
    "component": "billing_api",
    "group": "alpha-us",
    "class": "incident",
    "custom_details": {
      "incident_id": "INC-20260219-054214",
      "service": "billing_api",
      "profile": "alpha-us",
      "severity": "P1",
      "grafana_url": "https://alpha-us.grafana.local/...",
      "prometheus_url": "https://alpha-us.prometheus.local/..."
    }
  }
}
```

### **🔧 Setup Instructions**

#### **Slack Setup**
1. **Create Slack App**:
   - Go to https://api.slack.com/apps
   - Create new app → "From scratch"
   - Add "Incoming Webhooks" feature
   - Enable webhooks and add to workspace

2. **Get Webhook URL**:
   - In Slack app settings → "Incoming Webhooks"
   - Click "Add New Webhook to Workspace"
   - Select channel and copy webhook URL

3. **Configure in Health-Monitor**:
   ```bash
   sudo ./health-monitor notifications enable slack
   # Paste webhook URL when prompted
   ```

#### **PagerDuty Setup**
1. **Create PagerDuty Integration**:
   - Go to PagerDuty → Configuration → Integrations
   - Add "Events API v2" integration
   - Copy Integration Key (starts with "R0...")

2. **Configure in Health-Monitor**:
   ```bash
   sudo ./health-monitor notifications enable pagerduty
   # Paste routing key when prompted
   ```

### **🛠️ Troubleshooting Notifications**

#### **Common Issues**
```bash
# Check if notifications are enabled
sudo ./health-monitor notifications status

# Test notification delivery
sudo ./health-monitor notifications test slack
sudo ./health-monitor notifications test pagerduty

# Check profile-specific configuration
sudo ./health-monitor --profile alpha-us notifications status

# Verify configuration file
sudo cat /etc/health-monitor/profiles/alpha-us.yaml | grep -A 20 notifications
```

#### **Debug Steps**
1. **Check Status**: Verify notifications are enabled for the profile
2. **Test Connection**: Use test command to verify webhook/routing key
3. **Check Events**: Ensure incident events are in `notify_on` list
4. **Review Logs**: Check health-monitor logs for notification errors
5. **Verify URLs**: Ensure webhook URLs and routing keys are correct

#### **Validation Commands**
```bash
# Validate all profile notifications
for profile in alpha-us beta-eu core-platform; do
  echo "=== $profile Notifications ==="
  sudo ./health-monitor --profile $profile notifications status
  sudo ./health-monitor --profile $profile notifications test slack
done

# Test incident creation with notifications
sudo ./health-monitor --profile alpha-us incident start --service billing_api --severity P1 --title "Test Notification"
```

### **🔒 Security Considerations**

#### **Secure Configuration**
- **File Permissions**: Config files saved with 640 permissions
- **URL Protection**: Webhook URLs are redacted from logs and outputs
- **Test Before Save**: Connections are tested before saving configuration
- **Profile Isolation**: Each profile has separate notification settings

#### **Best Practices**
- Use dedicated Slack channels for each profile/environment
- Use different PagerDuty services for different teams
- Regularly test notification delivery
- Monitor notification failures in logs
- Keep webhook URLs and routing keys secure

### **📋 Notification Quick Reference**

| Command | Purpose | Example |
|---------|---------|---------|
| `enable slack` | Enable Slack notifications | `sudo ./health-monitor notifications enable slack` |
| `enable pagerduty` | Enable PagerDuty notifications | `sudo ./health-monitor notifications enable pagerduty` |
| `disable slack` | Disable Slack notifications | `sudo ./health-monitor notifications disable slack` |
| `test slack` | Send test Slack notification | `sudo ./health-monitor notifications test slack` |
| `status` | Show notification status | `sudo ./health-monitor notifications status` |
| `pagerduty set-key` | Set PagerDuty routing key | `sudo ./health-monitor notifications pagerduty set-key R0XXX` |

**Profile-Specific**: Add `--profile <name>` to any command for profile-specific operations.
