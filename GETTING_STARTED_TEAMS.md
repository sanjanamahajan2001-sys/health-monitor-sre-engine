# Getting Started for Teams

Welcome to Health Monitor's team onboarding guide! This comprehensive guide will help your team get up and running with production-grade monitoring in minutes, not hours.

## 🚀 Quick Start: Choose Your Team Type

Health Monitor provides **preset configurations** tailored for different team sizes and complexities. These presets include everything you need: optimized configurations, pre-defined service flows, alert rules, and team-specific best practices.

### Available Team Presets

| Preset | Team Size | Complexity | Best For |
|--------|-----------|------------|----------|
| **small-team** | 2-5 people | Simple | Startups, small teams with basic microservices |
| **medium-team** | 5-15 people | Moderate | Growing teams with multiple services |
| **large-team** | 15-50 people | Complex | Large teams with complex microservices |
| **enterprise-team** | 50+ people | Complex | Enterprise organizations with compliance needs |
| **devops-team** | 5-15 people | Moderate | DevOps teams focusing on CI/CD and infrastructure |
| **sre-team** | 5-20 people | Complex | SRE teams with advanced SLO requirements |

## 📋 One-Command Setup

### Step 1: Run the Team Setup Wizard

```bash
# Launch the interactive team setup wizard
sudo health-monitor --wizard-team
```

**What happens automatically:**
- ✅ Creates your team profile with optimized settings
- ✅ Sets up service flows for your architecture
- ✅ Configures alert rules for your services
- ✅ Creates isolated state directories
- ✅ Tests connectivity to your monitoring backends

### Step 2: Configure Your Monitoring Backend

The wizard will guide you through connecting your observability stack:

**Required:**
- **Prometheus** - Metrics collection
- **Service Discovery** - Auto-detect your services

**Optional (but recommended):**
- **Loki** - Log aggregation and analysis
- **Grafana** - Visualization dashboards
- **Tempo/Jaeger** - Distributed tracing
- **Slack/PagerDuty** - Team notifications

### Step 3: Start Monitoring

```bash
# Launch the interactive dashboard
sudo health-monitor

# Or check a specific profile
sudo health-monitor --profile your-team-name
```

## 🏗️ Understanding Profile Configuration

### What is a Profile?

A **profile** is a complete, isolated monitoring configuration that includes:

- **Backend URLs** - Prometheus, Loki, Grafana, Tempo endpoints
- **Service Discovery** - How to find and monitor your services
- **Alert Rules** - What constitutes an incident for your team
- **Flows** - Business-critical service workflows
- **Notifications** - How and when to alert your team
- **State Isolation** - Separate incidents, history, and caches

### Profile File Structure

```
/etc/health-monitor/
├── profiles/
│   └── your-team.yaml              # Main profile configuration
├── flows.d/
│   └── your-team.yaml              # Service flows and SLOs
├── alerts.d/
│   └── your-team.yaml              # Alert rules
└── state/
    └── your-team/
        ├── incidents/               # Your team's incidents
        ├── alerts/                  # Alert history
        └── history/                 # Operational history
```

### Key Configuration Fields

#### **Backend Configuration**
```yaml
config:
  prometheus_url: "http://prometheus.company.com:9090"
  loki_url: "http://loki.company.com:3100"
  grafana_url: "http://grafana.company.com:3000"
  grafana_trace_ds: "Tempo"          # or "Jaeger"
  
  # Adaptive Discovery Overrides (Optional)
  prometheus_metric_map:
    error_count: "custom_error_metric"
```

#### **Service Discovery**
```yaml
config:
  service_label: "service"           # How to identify services
  route_label: "route"               # How to identify endpoints
  auto_discover: true                # Automatically find services
  latency_threshold_seconds: 1.0     # Alert threshold
```

#### **Notification Settings**
```yaml
config:
  notifications:
    enabled: true
    slack:
      enabled: true
      webhook_url: "https://hooks.slack.com/..."
    pagerduty:
      enabled: true
      routing_key: "your-routing-key"
```

## 🎯 Team-Specific Workflows

### Small Teams (2-5 people)

**Daily Workflow:**
```bash
# Morning health check
sudo health-monitor

# Check for active incidents
sudo health-monitor incident list

# Weekly review
sudo health-monitor --profile small-team scorecard
```

**Best Practices:**
- Keep alert rules simple and actionable
- Focus on business-critical services first
- Use the TUI for quick daily checks
- Set up Slack notifications for critical alerts

### Medium Teams (5-15 people)

**Team Coordination:**
```bash
# Create separate environments (Advanced)
# 1. Create a generic profile
sudo health-monitor profile create --name staging
# 2. Or use the wizard to create specialized team profiles
sudo health-monitor --wizard-team

# Start team-specific alert listeners
sudo health-monitor --profile staging alert-listen --background --addr :9095
sudo health-monitor --profile production alert-listen --background --addr :9096

# Handover between shifts
sudo health-monitor --profile production incident list
```

**Best Practices:**
- Use separate profiles for different environments
- Set up on-call rotations with PagerDuty
- Configure business flow monitoring (checkout, auth, etc.)
- Document incidents with structured notes

### Large Teams (15-50 people)

**Multi-Team Setup:**
```bash
# Team Alpha setup
sudo health-monitor profile switch alpha-us
sudo health-monitor --profile alpha-us alert-listen --background --addr :9095

# Team Beta setup  
sudo health-monitor profile switch beta-eu
sudo health-monitor --profile beta-eu alert-listen --background --addr :9097

# Platform team oversight
sudo health-monitor --profile core-platform incident list
```

**Best Practices:**
- Implement role-based access control
- Use separate monitoring instances per team
- Configure automated runbook generation
- Set up integration with ticketing systems
- Monitor the monitoring system itself

### DevOps Teams

**CI/CD Monitoring:**
```bash
# Initialize DevOps-focused monitoring
sudo health-monitor --wizard-team

# Monitor build pipelines
sudo health-monitor flow list | grep cicd

# Track deployment success
sudo health-monitor incident start --service jenkins --severity P2 --title "Build pipeline failure"
```

**Key Metrics to Monitor:**
- Build success rates and duration
- Deployment frequency and lead time
- Infrastructure resource utilization
- Container and pod health
- Database performance

### SRE Teams

**Advanced SLO Management:**
```bash
# Initialize SRE-focused monitoring
sudo health-monitor --wizard-team

# Monitor error budgets
sudo health-monitor slo status

# Generate reliability reports
sudo health-monitor scorecard

# Conduct blameless postmortems
sudo health-monitor incident postmortem
```

**SRE Best Practices:**
- Define clear SLOs for all user-facing services
- Monitor error budgets and burn rates
- Use blameless postmortems for all incidents
- Track reliability metrics over time
- Automate toil reduction where possible

## 🔧 Common Setup Patterns

### Multi-Environment Setup

```bash
# Create environment-specific profiles using the wizard
sudo health-monitor --wizard-team

# Or clone an existing profile
sudo health-monitor profile create --name production --clone staging

# Configure different backends per environment
sudo health-monitor --profile dev profile show
# Edit: prometheus_url: http://dev-prometheus:9090

sudo health-monitor --profile production profile show  
# Edit: prometheus_url: http://prod-prometheus:9090
```

### Multi-Region Setup

```bash
# Team US-East
sudo health-monitor profile create --name us-east --from medium-team
sudo health-monitor --profile us-east alert-listen --background --addr :9095

# Team EU-West  
sudo health-monitor profile create --name eu-west --from medium-team
sudo health-monitor --profile eu-west alert-listen --background --addr :9096

# Configure region-specific endpoints
# Each profile points to its regional Prometheus/Grafana
```

### Service Ownership Model

```bash
# Payments team
sudo health-monitor profile create --name payments --from medium-team
# Focus on: billing_api, payment_service, wallet_service

# Platform team
sudo health-monitor profile create --name platform --from large-team  
# Focus on: auth_service, user_service, notification_service

# Infrastructure team
sudo health-monitor profile create --name infra --from devops-team
# Focus on: kubernetes, databases, networking
```

## 📊 Monitoring Best Practices

### 1. Start Simple, Iterate Fast

**Day 1:**
- Connect Prometheus
- Enable basic service discovery
- Set up Slack notifications for critical services

**Week 1:**
- Add service flows for business-critical paths
- Configure alert rules for key services
- Set up incident management workflow

**Month 1:**
- Add SLOs for user-facing services
- Implement log correlation with Loki
- Set up automated reporting

### 2. Alert Philosophy

**DO Alert:**
- User-impacting issues
- Service downtime or degradation
- Security incidents
- SLO burn rate alerts

**DON'T Alert:**
- Resource utilization without user impact
- Temporary blips that self-heal
- Debug logs without context
- Metrics without clear thresholds

### 3. Incident Management

**Incident Lifecycle:**
```bash
# 1. Start incident
sudo health-monitor incident start --service api_service --severity P1 --title "API latency spike"

# 2. Add investigation notes
sudo health-monitor incident note "Investigating database connection pool"
sudo health-monitor incident note "Found slow query on orders table"

# 3. Communicate with stakeholders
# (Automatic notifications sent to Slack/PagerDuty)

# 4. Resolve and learn
sudo health-monitor incident resolve --summary "Fixed slow query, added index"
sudo health-monitor incident postmortem
```

### 4. SLO Implementation

**Good SLO Characteristics:**
- User-facing and business-relevant
- Measurable with available metrics
- Achievable but ambitious
- Time-bound with clear windows

**Example SLOs:**
```yaml
flows:
  checkout:
    slos:
      - id: checkout_success
        objective: 99.9          # 99.9% success rate
        window: 30d              # Over 30 days
        type: ratio
        description: "Checkout completion rate"
      
      - id: checkout_latency  
        objective: 95            # 95th percentile
        window: 5m               # Over 5 minutes
        type: latency
        threshold: 2.0           # Under 2 seconds
        description: "Checkout response time"
```

## 🚨 Troubleshooting Common Issues

### "No Data Showing in Dashboard"

**Symptoms:**
- Empty metrics tables
- No services detected
- "No data available" messages

**Solutions:**
1. **Check Prometheus Connection:**
   ```bash
   sudo health-monitor --test-prom
   ```

2. **Verify Service Labels:**
   ```bash
   curl "http://prometheus:9090/api/v1/label/__name__/values"
   ```

3. **Check Network Connectivity:**
   ```bash
   curl -I "http://prometheus:9090/healthy"
   ```

**Prevention:**
- Test connections during initial setup
- Use the wizard's connectivity testing
- Verify Prometheus is scraping your services

### "Alerts Not Firing"

**Symptoms:**
- Services are down but no alerts created
- Webhook test succeeds but real alerts fail

**Solutions:**
1. **Check Alert Rules:**
   ```bash
   sudo health-monitor alert list-rules
   ```

2. **Verify Alert Listener:**
   ```bash
   sudo health-monitor alert status
   ```

3. **Test Webhook Manually:**
   ```bash
   curl -X POST http://localhost:9095/webhook -d '{"status":"firing","alerts":...}'
   ```

**Prevention:**
- Test alert rules during setup
- Use the wizard's alert generation
- Monitor the alert listener health

### "Profile Configuration Errors"

**Symptoms:**
- "Profile not found" errors
- Configuration not loading
- State directory issues

**Solutions:**
1. **List Available Profiles:**
   ```bash
   sudo health-monitor profile list
   ```

2. **Check Profile Files:**
   ```bash
   ls -la /etc/health-monitor/profiles/
   ```

3. **Validate Configuration:**
   ```bash
   sudo health-monitor profile validate
   ```

**Prevention:**
- Use preset configurations as starting points
- Validate profiles before switching
- Keep backup of working configurations

## 📚 Advanced Configuration

### Custom Alert Rules

Create `/etc/health-monitor/alerts.d/your-team.yaml`:

```yaml
alerts:
  # Specific alert for critical service
  - match:
      service: payment_service
      alertname: PaymentProcessingFailure
    severity: P1
    mode: auto
    title: "Payment Processing Failure"
    description: "Payment service is failing to process transactions"
    runbook: "https://wiki.company.com/runbooks/payments"

  # Generic alert for any service issues
  - match:
      service: user_service
    severity: P2
    mode: auto
    title: "User Service Alert"
    description: "Generic alert for user service issues"
```

### Business Flow Monitoring

Create `/etc/health-monitor/flows.d/your-team.yaml`:

```yaml
flows:
  checkout:
    name: "Checkout Process"
    description: "Complete checkout flow from cart to payment"
    services: [cart_service, payment_service, inventory_service, notification_service]
    slos:
      - id: checkout_success
        objective: 95
        window: 5m
        type: ratio
        error_query: 'http_requests_total{service="payment_service",status!~"2.."}'
        total_query: 'http_requests_total{service="payment_service"}'
        description: "Percentage of successful checkouts"

  user_onboarding:
    name: "User Onboarding"
    description: "New user registration and setup flow"
    services: [auth_service, user_service, email_service]
    slos:
      - id: onboarding_success
        objective: 90
        window: 10m
        type: ratio
        error_query: 'http_requests_total{service="auth_service",endpoint="/register",status!~"2.."}'
        total_query: 'http_requests_total{service="auth_service",endpoint="/register"}'
        description: "User registration success rate"
```

### Environment-Specific Overrides

Use environment variables for per-environment settings:

```bash
# Development
export PROMETHEUS_URL="http://dev-prometheus:9090"
export LOKI_URL="http://dev-loki:3100"
export HEALTH_MONITOR_PROFILE="dev"

# Production  
export PROMETHEUS_URL="http://prod-prometheus:9090"
export LOKI_URL="http://prod-loki:3100"
export HEALTH_MONITOR_PROFILE="production"
```

## 🔐 Security Considerations

### Secret Management

**Never store secrets in configuration files.** Use secure methods:

```bash
# Method 1: Environment variables (recommended for containers)
export HM_SECRET_PROMETHEUS_TOKEN="your-secret-token"
export HM_SECRET_SLACK_WEBHOOK="your-webhook-url"

# Method 2: Secure files (recommended for servers)
sudo echo "your-secret-token" > /etc/health-monitor/prometheus.token
sudo chmod 600 /etc/health-monitor/prometheus.token
```

### Network Security

**Webhook Security:**
```yaml
config:
  security:
    webhook:
      auth:
        enabled: true
        tokens:
          - name: "alertmanager"
            token: "Bearer your-secret-token"
      ip_whitelist:
        enabled: true
        allowed_ips: ["10.0.0.0/8", "192.168.0.0/16"]
      rate_limit:
        enabled: true
        requests_per_minute: 60
```

### Audit Logging

Enable comprehensive audit logging:

```yaml
config:
  security:
    audit:
      enabled: true
      log_file: "/var/log/health-monitor/audit.log"
      events:
        - "incident_created"
        - "incident_resolved" 
        - "config_changed"
        - "webhook_request"
      retention_days: 90
```

## 📈 Scaling Your Monitoring

### When to Upgrade Presets

**Small → Medium:**
- Team grows beyond 5 people
- Multiple services with dependencies
- Need for environment separation

**Medium → Large:**
- Team grows beyond 15 people
- Complex microservices architecture
- Multiple teams or regions

**Large → Enterprise:**
- Compliance and audit requirements
- Advanced security needs
- Multi-region deployment

### Performance Optimization

**High-Volume Environments:**
```yaml
config:
  prometheus_qps: 20              # Increase query rate
  loki_qps: 10                    # Increase log query rate
  route_cardinality_limit: 1000   # Handle more endpoints
  service_cardinality_limit: 500  # Handle more services
  correlation_max_logs: 1000      # Process more logs
```

**Resource Monitoring:**
```bash
# Monitor the monitoring system
sudo health-monitor alert-listen --background --addr :9095 --metrics-port :9096
curl http://localhost:9096/metrics
```

## 🎓 Training and Onboarding

### New Team Member Checklist

**Day 1 - Orientation:**
- [ ] Understand team's monitoring philosophy
- [ ] Get access to Health Monitor
- [ ] Read team's incident response playbook
- [ ] Join notification channels (Slack/PagerDuty)

**Week 1 - Hands-on:**
- [ ] Complete the preset setup wizard
- [ ] Practice starting and resolving incidents
- [ ] Review team's current SLOs and alert rules
- [ ] Shadow an experienced team member during incident

**Month 1 - Proficiency:**
- [ ] Lead an incident response
- [ ] Propose an improvement to alert rules
- [ ] Contribute to a postmortem
- [ ] Set up personal monitoring dashboard

### Team Training Exercises

**Exercise 1: Incident Simulation**
```bash
# Simulate a service outage
sudo health-monitor incident start --service api_service --severity P1 --title "Database connection failure"

# Practice investigation
sudo health-monitor incident note "Checking database logs"
sudo health-monitor incident note "Found connection pool exhaustion"

# Resolution and postmortem
sudo health-monitor incident resolve --summary "Increased connection pool size"
sudo health-monitor incident postmortem
```

**Exercise 2: SLO Definition**
```bash
# Analyze current service performance
sudo health-monitor --profile your-team slo status

# Define new SLOs based on data
sudo health-monitor --profile your-team flow list

# Create SLO alerts
sudo health-monitor --profile your-team alert list-rules
```

## 🤝 Community and Support

### Getting Help

**Documentation:**
- `USER_GUIDE.md` - Complete user manual
- `PROFILE_GUIDE_UPDATE.md` - Profile configuration details
- `RUNBOOK_USER_GUIDE.md` - Incident management guide

**Commands for Help:**
```bash
# General help
health-monitor --help

# Profile-specific help
sudo health-monitor profile --help

# Incident management help
sudo health-monitor incident --help

# List available presets
sudo health-monitor --list-presets
```

**Troubleshooting Commands:**
```bash
# Test connectivity
sudo health-monitor --test-prom

# Validate configuration
sudo health-monitor profile validate

# Check system status
sudo health-monitor alert status

# Debug mode
RUST_LOG=debug sudo health-monitor
```

### Contributing to Presets

If your team has a unique setup that could help others:

1. **Document your use case**
2. **Create a custom preset**
3. **Test thoroughly** 
4. **Share with the community**

```bash
# Export your working profile
sudo health-monitor profile show > my-team-preset.yaml

# Document your setup
echo "# My Team Setup" > my-team-guide.md
echo "## Team Size: X people" >> my-team-guide.md
echo "## Architecture: ..." >> my-team-guide.md
```

---

## 🎉 You're Ready!

Congratulations! Your team now has a production-grade monitoring setup tailored to your specific needs. Here's what to do next:

1. **Start Simple** - Begin with basic monitoring and add complexity gradually
2. **Monitor Daily** - Make Health Monitor part of your daily routine
3. **Learn from Incidents** - Use every incident as a learning opportunity
4. **Iterate and Improve** - Continuously refine your alerts and SLOs
5. **Share Knowledge** - Document learnings and train new team members

Remember: Good monitoring is a journey, not a destination. Start with the preset that matches your team, then evolve it as you grow.

**Happy Monitoring! 🚀**
