# Team Presets - Quick Start Guide

Health Monitor's **Team Presets** provide production-ready monitoring configurations tailored for different team sizes and specializations. Each preset includes optimized settings, pre-configured service flows, intelligent alert rules, and team-specific best practices.

## � Quick Start: Choose Your Setup Method

Health Monitor offers **two initialization methods** - choose based on your needs:

### Method 1: Dynamic Discovery (`--init`) - **Recommended for Production**

**Best for**: Production environments, existing infrastructure, custom metrics

```bash
# EKS Clusters (CRITICAL: Use -E to preserve AWS credentials)
sudo -E ./health-monitor --init --infra eks --kubeconfig ~/.kube/config

# Local Kubernetes
sudo ./health-monitor --init --infra kubernetes --kubeconfig ~/.kube/config

# With custom profile name
sudo -E ./health-monitor --init --infra eks --profile production-cluster
```

**What it does**:
- ✅ Scans your actual Kubernetes services.
- ✅ **Adaptive Probing**: Discover real Prometheus metric patterns by querying active series.
- ✅ **Intelligent Label Detection**: Automatically finds the correct labels (e.g., `app`, `service`) used by your data.
- ✅ Validates queries work with your data.
- ✅ Generates production-ready configurations.
- ✅ Creates accurate SLO flows based on your metrics.

**Time**: 2-5 minutes | **Accuracy**: High | **Production Ready**: ✅ Yes

### Method 2: Team Presets (`--wizard-team`) - **Quick Setup**

**Best for**: Demos, testing, standard architectures, quick start

```bash
# Interactive team setup
sudo ./health-monitor --wizard-team

# Quick non-interactive setup
sudo ./health-monitor --wizard-team --preset small-team
```

**What it does**:
- ⚡ Uses predefined templates (instant setup)
- 📋 Standard metric patterns (`http_requests_total`, etc.)
- 🎯 Team-optimized configurations
- 🚀 No infrastructure access required

**Time**: 10-30 seconds | **Accuracy**: Medium | **Production Ready**: ⚠️ May need tuning

### 📊 Comparison Table

| Feature | `--init` (Discovery) | `--wizard-team` (Presets) |
|---------|----------------------|---------------------------|
| **Setup Speed** | 2-5 minutes | 10-30 seconds |
| **Infrastructure Access** | Required | Not required |
| **Metric Accuracy** | High (discovered) | Medium (standard) |
| **Custom Metrics** | ✅ Supported | ⚠️ Manual setup |
| **Production Ready** | ✅ Yes | 🔧 May need tuning |
| **Best For** | Production, custom setups | Demos, standard setups |
| **Query Validation** | ✅ Automatic | ❌ Manual verification |

### 🎯 When to Use Which Method?

#### Use `--init` When:
- ✅ You have existing Kubernetes infrastructure
- ✅ You need accurate service discovery
- ✅ Your services use custom metric patterns
- ✅ You're setting up production monitoring
- ✅ You want validated, working queries

#### Use `--wizard-team` When:
- ✅ You need quick setup for demos
- ✅ You're testing or evaluating
- ✅ Your team uses standard metric patterns
- ✅ Infrastructure isn't accessible yet
- ✅ You want consistent team standards

### 🔄 Hybrid Approach

**Recommended workflow for teams**:

1. **Start with presets** for quick setup:
   ```bash
   sudo ./health-monitor --wizard-team --preset medium-team --profile demo
   ```

2. **Test and validate** the configuration:
   ```bash
   sudo ./health-monitor --profile demo slo list
   ```

3. **Upgrade to discovery** for production:
   ```bash
   sudo -E ./health-monitor --init --infra eks --profile production
   ```

4. **Merge configurations** manually if needed

---

## �🚀 One-Command Setup

```bash
# List all available presets
sudo health-monitor --list-presets

# Recommended for new teams (Interactive)
sudo health-monitor --wizard-team

# List all available presets
sudo health-monitor --list-presets

# Create additional profiles
sudo health-monitor profile create --name production
# Then use the wizard or manual config
```

## 📋 Available Presets

| Preset | Team Size | Complexity | Best For | Key Features |
|--------|-----------|------------|----------|--------------|
| **small-team** | 2-5 people | Simple | Startups, small teams | Quick setup, essential monitoring |
| **medium-team** | 5-15 people | Moderate | Growing teams | Multi-service flows, team coordination |
| **large-team** | 15-50 people | Complex | Large organizations | Multiple teams, advanced security |
| **enterprise-team** | 50+ people | Complex | Enterprise | Compliance, audit, multi-region |
| **devops-team** | 5-15 people | Moderate | DevOps teams | CI/CD monitoring, infrastructure focus |
| **sre-team** | 5-20 people | Complex | SRE teams | Advanced SLOs, error budgets, reliability |

## 🎯 Choosing the Right Preset

### Small Teams (2-5 people)
**Choose `small-team` if:**
- You're just starting with monitoring
- You have a simple microservices architecture
- You need quick setup with essential features
- You want to focus on business-critical services first

**What you get:**
- 1-2 pre-configured service flows
- Essential alert rules
- Simple notification setup
- Basic SLOs for key services

### Medium Teams (5-15 people)
**Choose `medium-team` if:**
- You have multiple services with dependencies
- You need environment separation (dev/staging/prod)
- You want team coordination features
- You're growing and need scalable monitoring

**What you get:**
- 3-5 service flows including business processes
- Expanded alert rules with severity levels
- Multi-environment support
- Team notification workflows

### Large Teams (15-50 people)
**Choose `large-team` if:**
- You have complex microservices architecture
- You have multiple teams or regions
- You need advanced security features
- You require comprehensive monitoring coverage

**What you get:**
- 5+ service flows covering critical business processes
- Comprehensive alert rules with runbooks
- Role-based access control
- Multi-team coordination features

### Enterprise Teams (50+ people)
**Choose `enterprise-team` if:**
- You need compliance and audit capabilities
- You have multi-region deployments
- Security is a top priority
- You require enterprise-grade features

**What you get:**
- All large-team features plus:
- Compliance monitoring workflows
- Advanced security with audit logging
- Multi-region monitoring setup
- Enterprise notification integrations

### DevOps Teams
**Choose `devops-team` if:**
- You focus on CI/CD and infrastructure
- You monitor build pipelines and deployments
- You need infrastructure health monitoring
- You want to optimize deployment processes

**What you get:**
- CI/CD pipeline monitoring flows
- Infrastructure health flows
- Build and deployment success SLOs
- Container and Kubernetes monitoring

### SRE Teams
**Choose `sre-team` if:**
- You're focused on reliability and SLOs
- You need error budget monitoring
- You practice blameless postmortems
- You want advanced correlation capabilities

**What you get:**
- User experience monitoring flows
- Service dependency flows
- Advanced SLOs with error budgets
- Reliability reporting and metrics

## 🔧 Setup Examples

### Example 1: Small Startup
```bash
# Quick setup for a small startup
sudo health-monitor --wizard-team
# Choose 'small-team' when prompted

# Configure your Prometheus URL
sudo health-monitor profile show
# The wizard handles connectivity testing!

# Start monitoring
sudo health-monitor
```

### Example 2: Growing Company with Multiple Environments
```bash
# Create profiles for each environment
sudo health-monitor profile create --name dev
sudo health-monitor profile create --name staging
sudo health-monitor --wizard-team
sudo health-monitor profile create --name production --preset large-team

# Configure different backends per environment
sudo health-monitor --profile dev profile show
# Set prometheus_url to dev-prometheus:9090

sudo health-monitor --profile production profile show
# Set prometheus_url to prod-prometheus:9090

# Start monitoring each environment
sudo health-monitor --profile production alert-listen --background --addr :9095
sudo health-monitor --profile staging alert-listen --background --addr :9096
```

### Example 3: DevOps Team Setup
```bash
# Initialize DevOps monitoring
sudo health-monitor --wizard-team

# Monitor your CI/CD pipeline
sudo health-monitor --profile devops-team flow list
# You'll see flows for: cicd, infrastructure

# Set up alerts for build failures
sudo health-monitor --profile devops-team alert list-rules
# Pre-configured alerts for Jenkins, Kubernetes, etc.

# Start monitoring
sudo health-monitor --profile devops-team
```

### Example 4: SRE Team with Advanced SLOs
```bash
# Initialize SRE monitoring
sudo health-monitor --wizard-team

# Check your SLOs and error budgets
sudo health-monitor --profile sre-team slo status

# Generate reliability report (or use --profile to skip selection)
sudo health-monitor --profile sre-team scorecard

# Monitor user experience flows
sudo health-monitor --profile sre-team flow list
# Flows for: user_experience, service_dependencies
```

## 📊 What Each Preset Includes

### Core Components (All Presets)
- ✅ **Optimized Configuration** - Tuned settings for your team size
- ✅ **Service Discovery** - Automatic service detection
- ✅ **Basic Alert Rules** - Essential monitoring alerts
- ✅ **Notification Setup** - Slack/PagerDuty integration
- ✅ **State Isolation** - Separate data per profile
- ✅ **Quick Start Guide** - Team-specific onboarding

### Progressive Features

| Feature | Small | Medium | Large | Enterprise |
|---------|-------|--------|-------|------------|
| Service Flows | 1-2 | 3-5 | 5+ | 5+ |
| Alert Rules | 1-2 | 3-5 | 5+ | 5+ |
| SLOs | Basic | Standard | Advanced | Advanced |
| Security | Basic | Standard | Advanced | Enterprise |
| Multi-Env | ❌ | ✅ | ✅ | ✅ |
| RBAC | ❌ | ❌ | ✅ | ✅ |
| Audit | ❌ | ❌ | ✅ | ✅ |
| Compliance | ❌ | ❌ | ❌ | ✅ |

### Specialized Features

#### DevOps Team Preset
- 🔄 **CI/CD Pipeline Monitoring** - Build success rates, deployment frequency
- 🏗️ **Infrastructure Health** - Kubernetes, containers, resource utilization
- 📊 **Performance Metrics** - Build times, deployment lead time
- 🔧 **Infrastructure Alerts** - Node failures, pod issues, resource constraints

#### SRE Team Preset
- 📈 **Advanced SLOs** - Error budgets, burn rate alerts
- 👥 **User Experience Monitoring** - End-to-end user journeys
- 🔗 **Service Dependencies** - Critical dependency monitoring
- 📋 **Reliability Reporting** - SLA compliance, reliability scores
- 🎯 **Correlation Analysis** - Advanced metric-log correlation

## 🎛️ Customization Guide

### After Initial Setup

#### 1. Configure Your Backends
```bash
# Edit your profile configuration
sudo health-monitor --profile your-team profile show

# Key settings to update:
# - prometheus_url: Your Prometheus endpoint
# - loki_url: Your Loki endpoint (optional)
# - grafana_url: Your Grafana endpoint (optional)
# - grafana_trace_ds: Tempo or Jaeger (optional)

# Adaptive Discovery Overrides (Optional)
# - prometheus_metric_map: {"error_count": "my_custom_error_metric"}
# - prometheus_label_map: {"payment": "my_custom_service_label"}
```

#### 2. Customize Service Flows
```bash
# List current flows
sudo health-monitor --profile your-team flow list

# Add custom flows (edit flows file)
sudo nano /etc/health-monitor/flows.d/your-team.yaml
```

#### 3. Tune Alert Rules
```bash
# List current alerts
sudo health-monitor --profile your-team alert list-rules

# Add custom alerts (edit alerts file)
sudo nano /etc/health-monitor/alerts.d/your-team.yaml
```

#### 4. Set Up Notifications
```bash
# Configure Slack
sudo health-monitor --profile your-team notifications enable slack
# Enter your webhook URL when prompted

# Configure PagerDuty
sudo health-monitor --profile your-team notifications enable pagerduty
# Enter your routing key when prompted
```

### Scaling Your Configuration

#### From Small to Medium
```bash
# Create a new medium-team profile using the wizard
sudo health-monitor --wizard-team

# Copy custom configurations from small profile
sudo cp /etc/health-monitor/flows.d/my-team-small.yaml \
        /etc/health-monitor/flows.d/my-team-medium.yaml

# Switch to the new profile
sudo health-monitor profile switch my-team-medium
```

#### From Medium to Large
```bash
# Use the wizard to create a large-team profile
sudo health-monitor --wizard-team

# Migrate custom configurations
# (Copy flows, alerts, and any custom settings)

# Enable advanced security features
sudo health-monitor --profile my-team-large security configure
```

## 🔍 Monitoring Best Practices by Team Size

### Small Teams - "Keep It Simple"
1. **Focus on Business Impact** - Monitor what affects your customers
2. **Start with Critical Services** - Don't try to monitor everything at once
3. **Use Simple Alerts** - Clear, actionable alerts with obvious next steps
4. **Daily Health Checks** - Make monitoring part of your daily routine
5. **Document Incidents** - Even simple notes help prevent repeat issues

### Medium Teams - "Build Processes"
1. **Environment Separation** - Use different profiles for dev/staging/prod
2. **Service Ownership** - Assign clear ownership for each service
3. **Standardize Alerting** - Consistent severity levels and escalation
4. **Team Coordination** - Use shared notifications and handoff processes
5. **Monitor Dependencies** - Understand how services affect each other

### Large Teams - "Scale and Automate"
1. **Role-Based Access** - Control who can see and modify configurations
2. **Automated Onboarding** - Use presets for new teams and services
3. **Centralized Standards** - Consistent monitoring across teams
4. **Performance Monitoring** - Monitor the monitoring system itself
5. **Compliance and Audit** - Track changes and maintain audit trails

### Enterprise Teams - "Govern and Secure"
1. **Compliance Monitoring** - Track regulatory requirements
2. **Multi-Region Setup** - Consistent monitoring across regions
3. **Advanced Security** - Encrypt secrets, audit all access
4. **Integration Ecosystem** - Connect with ticketing, CMDB, and other systems
5. **Reliability Engineering** - Formal SLOs, error budgets, postmortems

## 🚨 Troubleshooting Preset Issues

### "Preset Not Found"
```bash
# List available presets
sudo health-monitor --list-presets

# Check spelling and available options
# Common mistakes: small-team vs smallteam, devops-team vs devops
```

### "Profile Already Exists"
```bash
# Use the wizard and provide a name
sudo health-monitor --wizard-team

# Or delete existing profile first
sudo rm -rf /etc/health-monitor/profiles/existing-profile.yaml
sudo rm -rf /etc/health-monitor/state/existing-profile/
```

### "No Data After Setup"
```bash
# Check Prometheus connection
sudo health-monitor --test-prom

# Verify service labels match your metrics
curl "http://your-prometheus:9090/api/v1/label/__name__/values"

# Check profile configuration
sudo health-monitor --profile your-profile profile show
```

### "Alerts Not Firing"
```bash
# Check alert listener status
sudo health-monitor alert status

# Verify alert rules
sudo health-monitor --profile your-profile alert list-rules

# Test webhook manually
curl -X POST http://localhost:9095/webhook -d '{"status":"firing","alerts":...}'
```

## 📚 Next Steps

### Day 1 - Get Started
1. ✅ Choose and initialize your preset
2. ✅ Configure Prometheus connection
3. ✅ Start the monitoring dashboard
4. ✅ Verify services are discovered

### Week 1 - Customize
1. ✅ Tune alert thresholds for your environment
2. ✅ Add team-specific notification channels
3. ✅ Customize service flows for your architecture
4. ✅ Document your monitoring setup

### Month 1 - Scale
1. ✅ Set up additional environments (staging, production)
2. ✅ Implement team-specific workflows
3. ✅ Add custom alert rules for your services
4. ✅ Train team members on incident response

### Ongoing - Improve
1. ✅ Regular review of SLOs and alert effectiveness
2. ✅ Update configurations as services evolve
3. ✅ Share learnings with other teams
4. ✅ Contribute improvements back to presets

## 🤝 Contributing to Presets

Have a unique team setup that could help others? We'd love to hear from you!

### Share Your Experience
```bash
# Export your working configuration
sudo health-monitor --profile your-team profile show > my-team-setup.yaml

# Document your use case
echo "# My Team Setup" > my-team-guide.md
echo "## Team Size: X people" >> my-team-guide.md
echo "## Architecture: ..." >> my-team-guide.md
echo "## What Works Well: ..." >> my-team-guide.md
```

### Suggest Improvements
- New preset ideas for different team types
- Better default values for specific industries
- Additional quick start guides
- Improved troubleshooting tips

---

**Happy Monitoring! 🎉**

Remember: Good monitoring is a journey, not a destination. Start with the preset that matches your team, then evolve it as you grow and learn.
