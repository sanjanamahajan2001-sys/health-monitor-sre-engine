# EKS Integration Implementation Plan

## **Executive Summary**

This document outlines a production-ready implementation plan for integrating EKS (Amazon Elastic Kubernetes Service) into the health-monitor agent. The integration follows industry best practices by treating EKS as another telemetry source feeding the existing observability stack, rather than direct cluster scraping.

### **Core Principles**
- ✅ **Production Architecture**: EKS → Observability Stack → Health Monitor Agent
- ✅ **Backward Compatibility**: Zero breaking changes for existing users
- ✅ **User Choice**: Optional EKS integration with clear selection paths
- ✅ **Profile Isolation**: Complete state separation between environments
- ✅ **Gradual Rollout**: Feature-flagged implementation with safe migration

---

## **1. Goals & Objectives**

### **1.1 Primary Goals**
1. **Seamless EKS Integration**: Enable health-monitor to work with EKS clusters through existing observability stacks
2. **User Choice Architecture**: Allow users to choose between EKS and non-EKS environments during setup
3. **Production Readiness**: Ensure reliability, security, and performance at scale
4. **Unified Experience**: Maintain consistent CLI and TUI experience across all environments

### **1.2 Success Metrics**
- Zero breaking changes for existing users
- EKS discovery success rate > 99%
- Setup completion rate > 95% for both EKS and non-EKS paths
- Incident enrichment completeness > 95% for EKS environments
- Resource overhead < 5% for EKS features

---

## **2. User Experience Design**

### **2.1 Initial Setup Flow**

#### **Enhanced `--init` Command**
```bash
health-monitor --init
```

**Interactive Flow:**
```
? What type of infrastructure are you monitoring?
  ❏ Bare Metal / VM Services
  ❏ Kubernetes Cluster (Generic)
  ❏ Amazon EKS Cluster

? For EKS integration, how is your observability stack configured?
  ❏ Prometheus + Grafana (Recommended)
  ❏ Prometheus + Loki + Grafana
  ❏ CloudWatch + Prometheus
  ❏ Custom observability stack

? How should we configure EKS monitoring?
  ❏ Auto-discover my EKS clusters (Recommended)
  ❏ Manually specify cluster details
  ❏ I'll configure later
```

#### **Enhanced `--wizard-team` Command**
```bash
health-monitor --wizard-team
```

**Team-Specific EKS Configuration:**
```
? Select your team type:
  ❏ Platform Engineering
  ❏ Application Development
  ❏ DevOps/SRE
  ❏ Mixed Infrastructure

? EKS cluster access pattern:
  ❏ Multiple clusters (Hub-spoke model)
  ❏ Single cluster per environment
  ❏ Shared cluster across teams
```

### **2.2 Profile Structure Evolution**

#### **Current Profile Structure**
```yaml
name: default
config:
  prometheus_url: "https://prometheus.example.com"
  loki_url: "https://loki.example.com"
  # ... existing config
```

#### **Enhanced Profile Structure**
```yaml
name: eks-production
environment:
  type: "eks"  # bare_metal, vm, k8s, eks, serverless
  provider: "aws"
  cluster: "prod-eks-us-east-1"
  region: "us-east-1"
  namespaces: ["default", "payments", "orders"]
  
config:
  # Existing observability configuration
  prometheus_url: "https://prometheus.eks.example.com"
  loki_url: "https://loki.eks.example.com"
  
  # EKS-specific configuration
  eks:
    enabled: true
    auto_discover: true
    cluster_label: "cluster"
    namespace_label: "namespace"
    
    # Health thresholds
    health_thresholds:
      node_ready_percentage: 95.0
      pod_ready_percentage: 99.0
      p95_latency_ms: 500
      
    # Query optimization
    cache_ttl: "5m"
    max_concurrent_queries: 10
    
    # Security
    aws_auth_method: "irsa"  # irsa, iam_role, access_key
    assume_role_arn: ""

# Team-specific presets
team_preset: "platform-engineering"
metadata:
  aws_account_id: "123456789012"
  vpc_id: "vpc-12345"
  monitoring_stack: "kube-prometheus-stack"
```

---

## **3. Technical Architecture**

### **3.1 Environment Type Detection**

```go
// internal/environment/types.go
type EnvironmentType string

const (
    EnvironmentBareMetal EnvironmentType = "bare_metal"
    EnvironmentVM        EnvironmentType = "vm"
    EnvironmentK8s       EnvironmentType = "kubernetes"
    EnvironmentEKS       EnvironmentType = "eks"
    EnvironmentServerless EnvironmentType = "serverless"
)

type Environment struct {
    Type       EnvironmentType   `yaml:"type" json:"type"`
    Provider   string            `yaml:"provider,omitempty" json:"provider,omitempty"`
    Cluster    string            `yaml:"cluster,omitempty" json:"cluster,omitempty"`
    Region     string            `yaml:"region,omitempty" json:"region,omitempty"`
    Namespaces []string          `yaml:"namespaces,omitempty" json:"namespaces,omitempty"`
    Metadata   map[string]string `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}
```

### **3.2 EKS Discovery Service**

```go
// internal/discovery/eks.go
type EKSDiscovery struct {
    prometheus  *prometheus.Client
    loki        *loki.Client
    config      *config.EKSConfig
    logger      Logger
}

type EKSClusterInfo struct {
    Name         string                    `json:"name"`
    Region       string                    `json:"region"`
    AccountID    string                    `json:"account_id"`
    Namespaces   []string                  `json:"namespaces"`
    Nodes        EKSNodeInfo               `json:"nodes"`
    Services     []EKSServiceInfo          `json:"services"`
    Workloads    []EKSWorkloadInfo         `json:"workloads"`
    Health       EKSClusterHealth          `json:"health"`
    Observability ObservabilityStackInfo   `json:"observability"`
}

type ObservabilityStackInfo struct {
    PrometheusTargets []string `json:"prometheus_targets"`
    LokiEnabled       bool     `json:"loki_enabled"`
    GrafanaEnabled    bool     `json:"grafana_enabled"`
    StackVersion      string   `json:"stack_version"`
}
```

### **3.3 Configuration Extensions**

```go
// internal/config/config.go - Extended
type Config struct {
    // ... existing fields
    
    // Environment configuration
    Environment *Environment `yaml:"environment" json:"environment"`
    
    // EKS-specific configuration
    EKS *EKSConfig `yaml:"eks,omitempty" json:"eks,omitempty"`
    
    // Feature flags
    Features *FeatureFlags `yaml:"features" json:"features"`
}

type EKSConfig struct {
    Enabled           bool     `yaml:"enabled" json:"enabled"`
    AutoDiscover      bool     `yaml:"auto_discover" json:"auto_discover"`
    ClusterLabel      string   `yaml:"cluster_label" json:"cluster_label"`
    NamespaceLabel    string   `yaml:"namespace_label" json:"namespace_label"`
    
    // Health thresholds
    HealthThresholds HealthThresholds `yaml:"health_thresholds" json:"health_thresholds"`
    
    // Query optimization
    CacheTTL           string `yaml:"cache_ttl" json:"cache_ttl"`
    MaxConcurrentQueries int    `yaml:"max_concurrent_queries" json:"max_concurrent_queries"`
    
    // Authentication
    AWSAuthMethod      string `yaml:"aws_auth_method" json:"aws_auth_method"`
    AssumeRoleARN      string `yaml:"assume_role_arn" json:"assume_role_arn"`
    
    // Monitoring stack
    ObservabilityStack ObservabilityStack `yaml:"observability_stack" json:"observability_stack"`
}

type HealthThresholds struct {
    NodeReadyPercentage    float64 `yaml:"node_ready_percentage" json:"node_ready_percentage"`
    PodReadyPercentage     float64 `yaml:"pod_ready_percentage" json:"pod_ready_percentage"`
    P95LatencyMs          int     `yaml:"p95_latency_ms" json:"p95_latency_ms"`
    MemoryThresholdPercent float64 `yaml:"memory_threshold_percent" json:"memory_threshold_percent"`
    CPUThresholdPercent   float64 `yaml:"cpu_threshold_percent" json:"cpu_threshold_percent"`
}

type FeatureFlags struct {
    EKSDiscovery        bool `yaml:"eks_discovery" json:"eks_discovery"`
    EKSIncidents        bool `yaml:"eks_incidents" json:"eks_incidents"`
    EKSAutoConfig       bool `yaml:"eks_auto_config" json:"eks_auto_config"`
    EKSMetrics          bool `yaml:"eks_metrics" json:"eks_metrics"`
}
```

---

## **4. Implementation Phases**

### **Phase 1: Foundation (Week 1-2)**

#### **4.1 Core Infrastructure**
- [ ] Environment type detection system
- [ ] EKS configuration structures
- [ ] Profile structure extensions
- [ ] Feature flag system

#### **4.2 Discovery Service**
- [ ] EKS target detection in Prometheus
- [ ] Cluster metadata extraction
- [ ] Observability stack detection
- [ ] Auto-discovery algorithms

#### **4.3 CLI Enhancements**
- [ ] Enhanced `--init` with environment selection
- [ ] New `discover kubernetes` command
- [ ] Profile validation for EKS

### **Phase 2: Integration (Week 3-4)**

#### **4.4 Health Check Extensions**
```go
// internal/checks/eks.go
func EKSNodeHealth(report *model.Report, cluster string)
func EKSPodHealth(report *model.Report, namespace string)
func EKSWorkloadHealth(report *model.Report, cluster string)
func EKSClusterCapacity(report *model.Report, cluster string)
func EKSServiceHealth(report *model.Report, cluster string)
```

#### **4.5 Incident System Enhancement**
- [ ] EKS-specific incident patterns
- [ ] Kubernetes context enrichment
- [ ] EKS-aware RCA suggestions
- [ ] Enhanced incident metadata

#### **4.6 Wizard Integration**
- [ ] Team-specific EKS presets
- [ ] Multi-cluster configuration
- [ ] AWS authentication setup
- [ ] Observability stack detection

### **Phase 3: Production Features (Week 5-6)**

#### **4.7 Advanced Features**
- [ ] Multi-cluster support
- [ ] Namespace-based filtering
- [ ] EKS cost optimization insights
- [ ] Compliance and security scanning

#### **4.8 Performance & Reliability**
- [ ] Query optimization and caching
- [ ] Rate limiting for large clusters
- [ ] Circuit breaker patterns
- [ ] Graceful degradation

#### **4.9 Testing & Documentation**
- [ ] Comprehensive test suite
- [ ] Integration tests with real EKS clusters
- [ ] Performance benchmarks
- [ ] Production runbooks

---

## **5. User Journey Implementation**

### **5.1 New User Experience**

#### **Scenario 1: First-time Setup with EKS**
```bash
# User runs init
health-monitor --init

# Interactive EKS setup
? What type of infrastructure are you monitoring? → Amazon EKS Cluster
? How should we discover your EKS clusters? → Auto-discover
? Found 3 EKS clusters. Which to monitor? → [x] prod-eks, [x] staging-eks
? What namespaces should we monitor? → [x] All namespaces
? Detected kube-prometheus-stack. Use these settings? → Yes

# Profile created
✅ Profile 'eks-multi-cluster' created successfully
📊 Monitoring 2 clusters, 15 namespaces, 47 services
🔗 Grafana dashboards: https://grafana.example.com/d/eks-overview
```

#### **Scenario 2: Existing User Adding EKS**
```bash
# Existing user wants to add EKS
health-monitor profile create --name eks-production --clone default
health-monitor --profile eks-production --init

# EKS-specific configuration
? Configure EKS integration for profile 'eks-production'? → Yes
? EKS cluster name: → prod-eks-us-east-1
? AWS region: → us-east-1
? Prometheus URL for EKS: → https://prometheus.eks.example.com

# Profile updated
✅ EKS integration enabled for 'eks-production'
🔄 Run 'health-monitor discover eks' to validate setup
```

### **5.2 Team Wizard Enhancement**

#### **Platform Engineering Team**
```yaml
# Auto-generated profile
name: platform-engineering-eks
environment:
  type: eks
  provider: aws
  team_type: platform_engineering
  
config:
  eks:
    enabled: true
    auto_discover: true
    namespaces: ["kube-system", "monitoring", "ingress"]
    
    # Platform-specific thresholds
    health_thresholds:
      node_ready_percentage: 99.0
      etcd_health_percentage: 99.5
      
    observability_stack:
      components: ["prometheus", "grafana", "alertmanager", "loki"]
      
team_preset: platform-engineering
metadata:
  responsibility: "cluster_lifecycle"
  escalation_policy: "platform_sre"
```

#### **Application Development Team**
```yaml
# Auto-generated profile
name: app-dev-eks
environment:
  type: eks
  provider: aws
  team_type: application_development
  
config:
  eks:
    enabled: true
    auto_discover: true
    namespaces: ["payments", "orders", "auth"]
    
    # App-specific thresholds
    health_thresholds:
      pod_ready_percentage: 99.5
      p95_latency_ms: 200
      
    observability_stack:
      focus: "application_metrics"
      
team_preset: application-development
metadata:
  responsibility: "application_services"
  escalation_policy: "app_team_oncall"
```

---

## **6. Profile Management Strategy**

### **6.1 Profile Hierarchy**

```
/etc/health-monitor/
├── profiles/
│   ├── default.yaml                    # Base configuration
│   ├── eks-production.yaml             # EKS production profile
│   ├── eks-staging.yaml               # EKS staging profile
│   ├── platform-engineering.yaml      # Team-specific EKS profile
│   └── app-development.yaml           # Team-specific EKS profile
├── state/
│   ├── default/                       # Default profile state
│   ├── eks-production/                # EKS production state
│   └── eks-staging/                   # EKS staging state
├── flows.d/
│   ├── eks-production-flows.yaml      # EKS-specific flows
│   └── eks-staging-flows.yaml         # EKS-specific flows
└── alerts.d/
    ├── eks-production-alerts.yaml     # EKS-specific alerts
    └── eks-staging-alerts.yaml        # EKS-specific alerts
```

### **6.2 Profile Migration Strategy**

#### **Automatic Migration**
```go
// internal/config/migration.go
func MigrateToEKSProfile(oldProfile *Profile) (*Profile, error) {
    // Detect if user has EKS infrastructure
    if hasEKSInfrastructure(oldProfile.Config) {
        // Create EKS-enabled profile
        newProfile := createEKSProfile(oldProfile)
        
        // Preserve existing configuration
        newProfile.Config.PrometheusURL = oldProfile.Config.PrometheusURL
        newProfile.Config.LokiURL = oldProfile.Config.LokiURL
        
        // Add EKS-specific settings
        newProfile.Environment = detectEnvironment(oldProfile.Config)
        newProfile.Config.EKS = createDefaultEKSConfig()
        
        return newProfile, nil
    }
    return oldProfile, nil
}
```

#### **Manual Migration**
```bash
# User chooses to migrate
health-monitor profile migrate-to-eks --from default --to eks-default

# Interactive migration
? Migrate profile 'default' to support EKS? → Yes
? Detected EKS cluster 'prod-eks'. Add to profile? → Yes
? Preserve existing alert rules? → Yes
✅ Migration completed. New profile 'eks-default' created.
```

### **6.3 Profile Validation**

```go
// internal/config/validation.go
func ValidateEKSProfile(profile *Profile) error {
    var errors []string
    
    // Validate EKS configuration
    if profile.Config.EKS.Enabled {
        if profile.Environment.Cluster == "" {
            errors = append(errors, "EKS enabled but cluster name not specified")
        }
        
        if profile.Config.PrometheusURL == "" {
            errors = append(errors, "EKS monitoring requires Prometheus URL")
        }
        
        // Validate AWS credentials
        if err := validateAWSCredentials(profile.Config.EKS); err != nil {
            errors = append(errors, fmt.Sprintf("AWS credentials invalid: %v", err))
        }
    }
    
    if len(errors) > 0 {
        return fmt.Errorf("profile validation failed: %s", strings.Join(errors, "; "))
    }
    return nil
}
```

---

## **7. CLI Command Extensions**

### **7.1 New Commands**

#### **Discovery Commands**
```bash
# Discover EKS clusters
health-monitor discover kubernetes
health-monitor discover eks [--cluster auto|name] [--region us-east-1]

# Output example
Cluster discovered: prod-eks-us-east-1
  Region: us-east-1
  Nodes: 12
  Namespaces: 8
  Services: 24
  Workloads: 35
  
Observability stack detected:
  ✅ Prometheus: https://prometheus.eks.example.com
  ✅ Loki: https://loki.eks.example.com
  ✅ Grafana: https://grafana.eks.example.com
  
Recommended configuration:
  Cluster label: cluster
  Namespace label: namespace
  Health thresholds: Node 95%, Pod 99%
```

#### **EKS-Specific Health Checks**
```bash
# Check EKS cluster health
health-monitor check eks --cluster prod-eks
health-monitor check nodes --cluster prod-eks
health-monitor check pods --namespace payments
health-monitor check workloads --deployment orders-api

# Output example
EKS Cluster: prod-eks-us-east-1
Overall Health: ✅ HEALTHY

Node Health: ✅ 11/12 nodes ready (91.7%)
  ⚠️  node-12 NotReady (resource pressure)

Pod Health: ✅ 142/145 pods ready (97.9%)
  ❌ payments-api-7d6f8c9c-k8s9b CrashLoopBackOff

Workload Health:
  ✅ orders-api: 5/5 replicas ready
  ⚠️  payments-api: 2/3 replicas ready
  ✅ auth-service: 3/3 replicas ready
```

### **7.2 Enhanced Existing Commands**

#### **Profile Commands**
```bash
# Create EKS profile
health-monitor profile create --name eks-prod --preset eks-production

# List profiles with environment info
health-monitor profile list
  default (bare_metal)
  eks-production (eks) (active)
  eks-staging (eks)
  platform-team (eks)

# Show profile with EKS details
health-monitor profile show --name eks-production
Profile: eks-production
Environment: EKS (Amazon EKS)
Cluster: prod-eks-us-east-1
Region: us-east-1
Namespaces: default,payments,orders,auth
EKS Features: ✅ Enabled
Auto-discovery: ✅ Enabled
```

#### **Incident Commands**
```bash
# List incidents with EKS context
health-monitor incident list --environment eks
health-monitor incident list --cluster prod-eks
health-monitor incident list --namespace payments

# Create incident with EKS context
health-monitor incident create \
  --service payments-api \
  --severity P2 \
  --cluster prod-eks \
  --namespace payments \
  --deployment payments-api

# Output example
Incident INC-001 created:
  Service: payments-api
  Severity: P2
  Environment: EKS (prod-eks-us-east-1)
  Namespace: payments
  Deployment: payments-api
  Context: 2/3 replicas ready, 1 pod CrashLoopBackOff
```

---

## **8. Enhanced Dashboard (TUI)**

### **8.1 EKS Overview Dashboard**

```
┌─ Health Monitor - EKS Overview ─────────────────────────────────┐
│                                                                │
│ Environment: EKS (prod-eks-us-east-1)    Profile: eks-prod   │
│                                                                │
│ ┌─ Cluster Health ──────────────────┐ ┌─ Node Status ───────┐ │
│ │ Overall: ✅ HEALTHY               │ │ Ready: 11/12 (91.7%) │ │
│ │ Nodes: 12                         │ │ CPU: 67%            │ │
│ │ Namespaces: 8                     │ │ Memory: 71%         │ │
│ │ Services: 24                      │ │ Disk: 45%           │ │
│ └───────────────────────────────────┘ └─────────────────────┘ │
│                                                                │
│ ┌─ Top Services ─────────────────────────────────────────────┐ │
│ │ orders-api      ✅ 5/5 replicas  P95: 120ms               │ │
│ │ payments-api   ⚠️  2/3 replicas  P95: 340ms               │ │
│ │ auth-service    ✅ 3/3 replicas  P95:  80ms               │ │
│ │ user-service    ✅ 4/4 replicas  P95: 150ms               │ │
│ └────────────────────────────────────────────────────────────┘ │
│                                                                │
│ ┌─ Active Incidents ─────────────────────────────────────────┐ │
│ │ INC-001: payments-api pod restarts  (P2)  payments ns    │ │
│ │ INC-002: high memory on node-12     (P3)  kube-system    │ │
│ └────────────────────────────────────────────────────────────┘ │
│                                                                │
│ [d] Discovery  [i] Incidents  [c] Checks  [p] Profiles  [q] Quit │
└────────────────────────────────────────────────────────────────┘
```

### **8.2 Multi-Cluster View**

```
┌─ Health Monitor - Multi-Cluster View ──────────────────────────┐
│                                                                │
│ Active Profile: platform-engineering (Multi-EKS)               │
│                                                                │
│ ┌─ prod-eks-us-east-1 ──────────────────┐ ┌─ staging-eks ────┐ │
│ │ Status: ✅ HEALTHY                    │ │ Status: ⚠️ WARNING │ │
│ │ Nodes: 12/12 ready                    │ │ Nodes: 3/4 ready  │ │
│ │ Services: 24                          │ │ Services: 8      │ │
│ │ Incidents: 1 (P2)                     │ │ Incidents: 2     │ │
│ └───────────────────────────────────────┘ └─────────────────┘ │
│                                                                │
│ ┌─ dev-eks-eu-west-2 ───────────────────┐ ┌─ mgmt-eks ───────┐ │
│ │ Status: ✅ HEALTHY                    │ │ Status: ✅ HEALTHY│ │
│ │ Nodes: 6/6 ready                      │ │ Nodes: 2/2 ready  │ │
│ │ Services: 12                          │ │ Services: 5      │ │
│ │ Incidents: 0                          │ │ Incidents: 0     │ │
│ └───────────────────────────────────────┘ └─────────────────┘ │
│                                                                │
│ [Enter] Details  [Tab] Switch Cluster  [r] Refresh  [q] Quit   │
└────────────────────────────────────────────────────────────────┘
```

---

## **9. Testing Strategy**

### **9.1 Unit Testing**

```go
// internal/discovery/eks_test.go
func TestEKSTargetDetection(t *testing.T) {
    // Test Prometheus target detection for EKS
}

func TestClusterMetadataExtraction(t *testing.T) {
    // Test cluster metadata extraction from metrics
}

func TestEKSHealthChecks(t *testing.T) {
    // Test EKS-specific health checks
}

// internal/config/eks_test.go
func TestEKSProfileValidation(t *testing.T) {
    // Test EKS profile validation
}

func TestEKSMigration(t *testing.T) {
    // Test profile migration to EKS
}
```

### **9.2 Integration Testing**

```go
// test/integration/eks_test.go
func TestEKSDiscoveryEndToEnd(t *testing.T) {
    // Test complete EKS discovery workflow
}

func TestEKSIncidentCreation(t *testing.T) {
    // Test incident creation with EKS context
}

func TestEKSProfileAutoConfig(t *testing.T) {
    // Test automatic profile configuration
}
```

### **9.3 Production Testing**

```bash
# Canary deployment testing
health-monitor --profile eks-canary validate --deep
health-monitor --profile eks-canary discover eks --cluster test-eks
health-monitor --profile eks-canary check eks --cluster test-eks

# Load testing
health-monitor --profile eks-loadtest benchmark --duration 10m
health-monitor --profile eks-loadtest stress --concurrent-queries 50
```

---

## **10. Security Considerations**

### **10.1 AWS Authentication**

```go
// internal/aws/auth.go
type AWSAuthConfig struct {
    Method      string `yaml:"method"`      // irsa, iam_role, access_key
    RoleARN     string `yaml:"role_arn"`
    Region      string `yaml:"region"`
    AccessKey   string `yaml:"access_key"`
    SecretKey   string `yaml:"secret_key"`
    SessionToken string `yaml:"session_token"`
}

func ValidateAWSAuth(config *AWSAuthConfig) error {
    // Validate AWS credentials and permissions
    // Check required IAM permissions
    // Test cluster access
}
```

### **10.2 Permission Requirements**

```yaml
# Required IAM permissions for EKS integration
RequiredIAMPermissions:
  - "eks:DescribeCluster"
  - "eks:ListClusters"
  - "eks:ListNodegroups"
  - "ec2:DescribeInstances"
  - "ec2:DescribeRegions"
  - "cloudwatch:GetMetricData"
  - "iam:PassRole"  # For IRSA

# Minimum Kubernetes RBAC permissions
RequiredK8sPermissions:
  - "apiGroups: ['']", resources: ['nodes', 'pods', 'services'], verbs: ['get', 'list']
  - "apiGroups: ['apps']", resources: ['deployments', 'replicasets'], verbs: ['get', 'list']
  - "apiGroups: ['metrics.k8s.io']", resources: ['pods', 'nodes'], verbs: ['get', 'list']
```

### **10.3 Data Privacy**

```go
// internal/privacy/sanitizer.go
func SanitizeClusterData(data *EKSClusterInfo) *EKSClusterInfo {
    // Remove sensitive information from cluster data
    // Sanitize node names, IP addresses
    // Filter out sensitive labels
}

func SanitizeIncidentData(incident *Incident) *Incident {
    // Remove sensitive information from incidents
    // Sanitize pod names, container images
    // Filter out sensitive annotations
}
```

---

## **11. Performance Optimization**

### **11.1 Query Optimization**

```go
// internal/eks/cache.go
type EKSCache struct {
    clusterInfo    *EKSClusterInfo
    nodeInfo       []EKSNodeInfo
    serviceInfo    []EKSServiceInfo
    workloadInfo   []EKSWorkloadInfo
    lastUpdate     time.Time
    ttl           time.Duration
}

func (c *EKSCache) GetClusterInfo() (*EKSClusterInfo, error) {
    if time.Since(c.lastUpdate) < c.ttl {
        return c.clusterInfo, nil
    }
    return c.refreshClusterInfo()
}
```

### **11.2 Rate Limiting**

```go
// internal/eks/ratelimiter.go
type EKSRateLimiter struct {
    prometheusQPS float64
    lokiQPS       float64
    concurrent    int
}

func (r *EKSRateLimiter) ExecuteQuery(query func() error) error {
    // Implement rate limiting for EKS queries
    // Prevent overwhelming Prometheus/Loki
}
```

### **11.3 Resource Management**

```go
// internal/eks/resource_manager.go
type EKSResourceManager struct {
    maxMemoryMB    int
    maxGoroutines  int
    queryTimeout   time.Duration
}

func (rm *EKSResourceManager) CheckResources() error {
    // Monitor resource usage
    // Prevent memory leaks
    // Manage goroutine lifecycle
}
```

---

## **12. Monitoring & Observability**

### **12.1 EKS-Specific Metrics**

```go
// internal/metrics/eks.go
var EKSMetrics = struct {
    DiscoveryDuration    prometheus.HistogramVec
    QuerySuccessRate     prometheus.CounterVec
    CacheHitRate         prometheus.GaugeVec
    ClusterHealthScore   prometheus.GaugeVec
    IncidentEnrichmentRate prometheus.CounterVec
}{
    DiscoveryDuration: prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "health_monitor_eks_discovery_duration_seconds",
            Help: "Time spent discovering EKS clusters",
        },
        []string{"cluster", "region"},
    ),
    // ... other metrics
}
```

### **12.2 Health Checks for EKS Integration**

```go
// internal/health/eks.go
func CheckEKSIntegration() error {
    // Check Prometheus connectivity
    // Validate EKS metrics availability
    // Verify Loki integration
    // Test authentication
}

func CheckEKSPermissions() error {
    // Verify AWS permissions
    // Check Kubernetes RBAC
    // Validate observability stack access
}
```

---

## **13. Documentation & Training**

### **13.1 User Documentation**

```markdown
# EKS Integration Guide

## Quick Start
1. Run `health-monitor --init` and select "Amazon EKS Cluster"
2. Choose auto-discovery or manual configuration
3. Validate setup with `health-monitor discover eks`

## Configuration
- Profile configuration for EKS
- AWS authentication setup
- Observability stack integration

## Troubleshooting
- Common setup issues
- Permission problems
- Performance tuning
```

### **13.2 Operator Documentation**

```markdown
# EKS Integration Operations Guide

## Deployment
- Canary deployment strategy
- Rollback procedures
- Performance monitoring

## Security
- IAM permission requirements
- Kubernetes RBAC setup
- Data privacy considerations

## Maintenance
- Cache management
- Log rotation
- Backup procedures
```

---

## **14. Rollout Strategy**

### **14.1 Phase 1: Internal Testing (Week 1)**
- [ ] Feature flag implementation
- [ ] Core EKS discovery functionality
- [ ] Basic profile support
- [ ] Internal team testing

### **14.2 Phase 2: Beta Release (Week 2-3)**
- [ ] Enhanced wizard integration
- [ ] CLI command extensions
- [ ] Incident system enhancements
- [ ] Selected customer beta testing

### **14.3 Phase 3: General Availability (Week 4-6)**
- [ ] Full feature completion
- [ ] Comprehensive documentation
- [ ] Performance optimization
- [ ] General release

### **14.4 Phase 4: Advanced Features (Week 7-8)**
- [ ] Multi-cluster support
- [ ] Advanced analytics
- [ ] Compliance features
- [ ] Enterprise features

---

## **15. Success Criteria & KPIs**

### **15.1 Technical KPIs**
- EKS discovery success rate: > 99%
- Incident enrichment completeness: > 95%
- Query performance: < 2 seconds for cluster overview
- Resource overhead: < 5% increase
- Cache hit rate: > 80%

### **15.2 User Experience KPIs**
- Setup completion rate: > 95%
- User satisfaction score: > 4.5/5
- Support ticket reduction: > 20%
- Feature adoption rate: > 60%

### **15.3 Business KPIs**
- Zero breaking changes for existing users
- Fast ROI for EKS customers
- Reduced incident resolution time
- Improved operational efficiency

---

## **16. Risk Assessment & Mitigation**

### **16.1 Technical Risks**

| Risk | Impact | Likelihood | Mitigation |
|------|---------|------------|------------|
| Performance degradation | High | Medium | Rate limiting, caching, resource limits |
| AWS API throttling | Medium | High | Exponential backoff, request batching |
| Complex setup process | High | Medium | Guided wizard, auto-discovery |
| Breaking existing functionality | High | Low | Comprehensive testing, feature flags |

### **16.2 Business Risks**

| Risk | Impact | Likelihood | Mitigation |
|------|---------|------------|------------|
| Low adoption rate | Medium | Medium | User feedback, iterative improvement |
| Increased support burden | Medium | Medium | Documentation, training, automation |
| Security vulnerabilities | High | Low | Security review, permission validation |

---

## **17. Conclusion**

This implementation plan provides a comprehensive, production-ready approach to EKS integration that:

✅ **Maintains Backward Compatibility**: Zero breaking changes for existing users  
✅ **Provides User Choice**: Clear EKS vs non-EKS paths in setup  
✅ **Follows Production Best Practices**: Treats EKS as telemetry source, not direct target  
✅ **Ensures Safety**: Gradual rollout with feature flags and comprehensive testing  
✅ **Delivers Value**: Enhanced incident context, automated discovery, improved operational efficiency  

The modular design allows for gradual adoption while maintaining the high standards of reliability and performance expected in production environments. The integration seamlessly extends the existing health-monitor architecture without compromising its core principles.

---

## **18. Expert Recommendations & Enhancements**

> [!IMPORTANT]
> To truly make this integration production-ready and meet the user's requirements for auto-discovery via `kubeconfig`, the following enhancements are recommended:

### **18.1 Direct Kubernetes API Integration**
While treating EKS as a telemetry source is scalable, direct interaction with the Kubernetes API via `k8s.io/client-go` is essential for:
- **Reliable Metadata**: Fetching real-time namespace, pod, and node metadata without relying on Prometheus scrape intervals.
- **Event Correlation**: Streaming Kubernetes events (e.g., `BackOff`, `FailedScheduling`) directly into the incident analysis.
- **Contextual Enrichment**: Mapping Prometheus metrics to specific Pod/Node statuses (e.g., "High latency on node-12 which is currently under DiskPressure").

### **18.2 Intelligent Infrastructure Choice**
The agent should explicitly ask the user about their infrastructure type in both `--init` and the `Wizard` to provide a relevant experience:
- **Explicit Selection**: The first step of any setup should be: "What type of infrastructure are you monitoring? [EKS, Generic Kubernetes, Bare Metal/VM]".
- **Contextual Wizard Steps**: 
    - If **EKS** is selected: Prompt for `kubeconfig` context, AWS region, and CloudWatch integration.
    - If **Generic K8s** is selected: Prompt for `kubeconfig` and local Prometheus/Loki URLs.
    - If **Bare Metal** is selected: Skip K8s-specific discovery and focus on service/process monitoring.
- **Auto-Discovery Persistence**: Discovered details (Namespaces, Cluster Name, Region) MUST be saved directly into the `Profile` struct and persisted to the YAML file.

### **18.4 AWS Native Authentication & CloudWatch**
Integration with `github.com/aws/aws-sdk-go-v2` is required for production-grade AWS access:
- **Credential Provider Chain**: Support environment variables, AWS profiles, IAM Roles for Service Accounts (IRSA), and EC2 Instance Metadata automatically.
- **CloudWatch as a Telemetry Source**: For environments without Prometheus, fetch metrics from CloudWatch `AWS/ContainerInsights`.
- **Hybrid Mode**: Allow Prometheus for application metrics and CloudWatch for EKS control plane/node metrics.

### **18.5 Intelligent Alert & Metric Configuration**
To answer the efficiency requirements, the agent will bridge the gap between "Discovery" and "Monitoring":
- **Infrastructure Discovery**: Auto-fetches Namespaces, Services, and Pods to know *what* exists.
- **URL Discovery**: Scans for Prometheus/Loki/CloudWatch endpoints to know *where* the telemetry is.
- **Preset Matching**: Once services are discovered, the agent will automatically apply relevant **Alerts and SLOs** from the selected `TeamPreset`.
    - *Example*: If a service `order-processor` is discovered and the `Enterprise` preset has a generic "Microservice Latency" alert, it is automatically configured for that service.
- **Metric Mapping**: Metrics like CPU/Memory/Latency are auto-mapped from EKS Container Insights or Prometheus `kube-state-metrics`.
