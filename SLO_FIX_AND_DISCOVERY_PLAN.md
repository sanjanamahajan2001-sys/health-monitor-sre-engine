# SLO System Fix & Service Discovery Implementation Plan

## **Problem Analysis**

After analyzing your code and the SLO output, I've identified two critical issues:

### **Issue 1: Missing Data Handling Problem**
Your SLO system incorrectly treats missing data as breaches, causing every new service to appear as broken.

**Current Logic (Problematic):**
```go
// Lines 88-95 in slo/service.go
if total == 0 {
    result.Status = StatusUnknown
    result.Compliance = "UNKNOWN"
    result.Risk = "-"
    result.Error = fmt.Errorf("no traffic (total=0)")
    results = append(results, result)
    continue
}
```

**But the display logic shows "ERROR" instead of proper states:**
```go
// Lines 100-107 in slo/commands.go
compIcon := "❓"
if r.Compliance == "COMPLIANT" {
    compIcon = "✅"
} else if r.Compliance == "BREACHING" {
    compIcon = "❌"
} else if r.Compliance == "INSUFFICIENT_DATA" {
    compIcon = "⚪"
}
```

### **Issue 2: Hardcoded Service Problem**
Your flows are created from presets with hardcoded services like `api_service`, `payment_service`, but your test EKS cluster only has infrastructure services (prometheus, grafana, loki, etc.), not application services.

**Root Cause:** The wizard creates preset flows without checking if services actually exist in the observability stack.

---

## **Solution Architecture**

### **Phase 1: Fix SLO Status System (Immediate)**

#### **1.1 Enhanced SLO Status Types**
```go
// internal/slo/types.go
type SLOStatus string
type ComplianceStatus string

const (
    // Legacy (keep for compatibility)
    StatusHealthy   SLOStatus = "HEALTHY"
    StatusWarning   SLOStatus = "WARNING" 
    StatusBreaching SLOStatus = "BREACHING"
    StatusUnknown   SLOStatus = "UNKNOWN"
    
    // New compliance states
    ComplianceOK      ComplianceStatus = "OK"
    ComplianceNoTraffic ComplianceStatus = "NO_TRAFFIC"
    ComplianceNoData   ComplianceStatus = "NO_DATA"
    ComplianceError    ComplianceStatus = "ERROR"
    ComplianceBreaching ComplianceStatus = "BREACHING"
)

type SLOResult struct {
    // ... existing fields
    Compliance ComplianceStatus `json:"compliance"`
    NoTrafficReason string       `json:"no_traffic_reason,omitempty"`
    DataQuality    string       `json:"data_quality,omitempty"` // "good", "insufficient", "missing"
}
```

#### **1.2 Fixed Status Determination Logic**
```go
// internal/slo/service.go - Enhanced CheckSLOs method
func (s *Service) CheckSLOs(flows []flow.Flow) []SLOResult {
    for _, f := range flows {
        for _, slo := range f.SLOs {
            result := SLOResult{
                FlowID:    f.ID,
                SLOID:     slo.ID,
                Service:   slo.Service,
                Objective: slo.Objective,
                Window:    slo.Window,
                Type:      slo.Type,
                Compliance: ComplianceOK, // Default to OK
            }
            
            // Handle based on SLO type
            if slo.Type == "ratio" && slo.ErrorQuery != "" && slo.TotalQuery != "" {
                // 1. Fetch Total
                totalQuery := fmt.Sprintf("sum(rate(%s[%s]))", slo.TotalQuery, slo.Window)
                total, err := s.metricsProvider.QueryInstant(totalQuery)
                
                if err != nil {
                    result.Compliance = ComplianceError
                    result.Error = fmt.Errorf("query failed: %w", err)
                    result.DataQuality = "missing"
                    results = append(results, result)
                    continue
                }
                
                // Production SRE logic: No traffic is NOT a breach
                if total == 0 {
                    result.Compliance = ComplianceNoTraffic
                    result.NoTrafficReason = "no_requests_in_window"
                    result.DataQuality = "insufficient"
                    result.Current = 0
                    result.BurnRate = 0
                    result.BudgetRemaining = 100 // Full budget remaining
                    result.Trend = "➡️"
                    results = append(results, result)
                    continue
                }
                
                // Check if we have sufficient data (minimum threshold)
                minRequestsThreshold := 10.0 // Configurable
                if total < minRequestsThreshold {
                    result.Compliance = ComplianceNoData
                    result.NoTrafficReason = fmt.Sprintf("insufficient_data: %.0f requests (< %.0f)", total, minRequestsThreshold)
                    result.DataQuality = "insufficient"
                    result.Current = 0
                    results = append(results, result)
                    continue
                }
                
                // 2. Fetch Error (only if we have sufficient traffic)
                errorQuery := fmt.Sprintf("sum(rate(%s[%s]))", slo.ErrorQuery, slo.Window)
                errVal, err := s.metricsProvider.QueryInstant(errorQuery)
                
                if err != nil {
                    result.Compliance = ComplianceError
                    result.Error = fmt.Errorf("error query failed: %w", err)
                    result.DataQuality = "missing"
                    results = append(results, result)
                    continue
                }
                
                // Calculate success rate
                successRate := 100 * (1 - (errVal / total))
                if successRate < 0 { successRate = 0 }
                if successRate > 100 { successRate = 100 }
                
                result.Current = successRate
                result.DataQuality = "good"
                
                // 3. Determine compliance based on objective
                if successRate >= slo.Objective {
                    result.Compliance = ComplianceOK
                    result.Status = StatusHealthy
                } else {
                    result.Compliance = ComplianceBreaching
                    result.Status = StatusBreaching
                }
                
                // ... rest of burn rate calculations
            }
        }
    }
}
```

#### **1.3 Enhanced Display Logic**
```go
// internal/slo/commands.go - Fixed display
func handleSLOList(args []string) int {
    for _, r := range results {
        // Compliance status with proper states
        var compIcon string
        var compText string
        
        switch r.Compliance {
        case ComplianceOK:
            compIcon = "✅"
            compText = "OK"
        case ComplianceNoTraffic:
            compIcon = "⚪"
            compText = "NO_TRAFFIC"
        case ComplianceNoData:
            compIcon = "⚠️"
            compText = "INSUFFICIENT_DATA"
        case ComplianceError:
            compIcon = "❓"
            compText = "ERROR"
        case ComplianceBreaching:
            compIcon = "❌"
            compText = "BREACHING"
        default:
            compIcon = "❓"
            compText = "UNKNOWN"
        }
        
        // Current value formatting
        current := r.FormatCurrent()
        if r.Compliance == ComplianceNoTraffic {
            current = "N/A"
        } else if r.Compliance == ComplianceError {
            current = "ERR"
        }
        
        // Risk assessment (only for valid data)
        riskText := "-"
        if r.DataQuality == "good" {
            if r.Risk == "CRITICAL" {
                riskText = "🔥 CRITICAL"
            } else if r.Risk == "HIGH" {
                riskText = "⚠️ HIGH"
            } else if r.Risk == "LOW" {
                riskText = "✅ LOW"
            }
        }
        
        fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s %s\t%s\n",
            r.SLOID, r.FlowID, r.FormatObjective(), r.Window,
            current, r.FormatBurnRate(), r.FormatBudget(),
            r.Trend, compIcon, compText, riskText)
    }
}
```

### **Phase 2: Service Discovery System**

#### **2.1 Discovery Service Architecture**
```go
// internal/discovery/service.go
type DiscoveryService struct {
    prometheus  *prometheus.Client
    loki        *loki.Client
    tempo       *tempo.Client
    config      *config.Config
}

type DiscoveredServices struct {
    Services     []DiscoveredService `json:"services"`
    Routes       []string           `json:"routes"`
    Dependencies []ServiceDependency `json:"dependencies"`
    Timestamp    time.Time          `json:"timestamp"`
}

type DiscoveredService struct {
    Name        string            `json:"name"`
    Type        string            `json:"type"` // "application", "infrastructure"
    Metrics     []string          `json:"metrics"`
    HasTraffic  bool              `json:"has_traffic"`
    Labels      map[string]string `json:"labels"`
    FirstSeen   time.Time         `json:"first_seen"`
}

type ServiceDependency struct {
    From     string    `json:"from"`
    To       string    `json:"to"`
    Strength float64   `json:"strength"` // 0.0 to 1.0
    Method   string    `json:"method"`   // "traces", "metrics", "logs"
    LastSeen time.Time `json:"last_seen"`
}
```

#### **2.2 Service Discovery Implementation**
```go
// internal/discovery/prometheus.go
func (d *DiscoveryService) DiscoverServicesFromPrometheus() (*DiscoveredServices, error) {
    services := make(map[string]DiscoveredService)
    
    // 1. Discover services from metrics
    serviceLabel := d.config.ServiceLabel
    if serviceLabel == "" {
        serviceLabel = "service"
    }
    
    // Query all services that have metrics
    servicesQuery := fmt.Sprintf(`label_values(http_requests_total, %s)`, serviceLabel)
    serviceNames, err := d.prometheus.LabelValues(servicesQuery)
    if err != nil {
        // Fallback: try other common metrics
        fallbackMetrics := []string{
            `http_server_requests_total`,
            `flask_http_request_total`,
            `django_http_requests_total`,
            `nginx_http_requests_total`,
        }
        
        for _, metric := range fallbackMetrics {
            query := fmt.Sprintf(`label_values(%s, %s)`, metric, serviceLabel)
            serviceNames, err = d.prometheus.LabelValues(query)
            if err == nil && len(serviceNames) > 0 {
                break
            }
        }
    }
    
    // 2. For each service, check if it has traffic
    for _, serviceName := range serviceNames {
        trafficQuery := fmt.Sprintf(`sum(rate(http_requests_total{%s="%s"}[5m]))`, serviceLabel, serviceName)
        traffic, err := d.prometheus.QueryInstant(trafficQuery)
        
        service := DiscoveredService{
            Name:       serviceName,
            Type:       "application",
            HasTraffic: err == nil && traffic > 0,
            Labels:     make(map[string]string),
            FirstSeen:  time.Now(),
        }
        
        // Discover available metrics for this service
        metricsQuery := fmt.Sprintf(`{__name__=~".*%s.*",%s="%s"}`, serviceLabel, serviceLabel, serviceName)
        series, err := d.prometheus.Series(metricsQuery, time.Hour)
        if err == nil {
            for _, s := range series {
                if metricName, ok := s["__name__"]; ok {
                    service.Metrics = append(service.Metrics, metricName)
                }
            }
        }
        
        services[serviceName] = service
    }
    
    // 3. Discover infrastructure services (Kubernetes components)
    infraServices := d.discoverInfraServices()
    for name, service := range infraServices {
        services[name] = service
    }
    
    // 4. Discover routes/endpoints
    routes, err := d.discoverRoutes()
    if err != nil {
        log.Printf("WARN: Failed to discover routes: %v", err)
    }
    
    // 5. Discover dependencies from traces
    dependencies, err := d.discoverDependencies()
    if err != nil {
        log.Printf("WARN: Failed to discover dependencies: %v", err)
    }
    
    result := &DiscoveredServices{
        Services:     make([]DiscoveredService, 0, len(services)),
        Routes:       routes,
        Dependencies: dependencies,
        Timestamp:    time.Now(),
    }
    
    for _, service := range services {
        result.Services = append(result.Services, service)
    }
    
    return result, nil
}

func (d *DiscoveryService) discoverInfraServices() map[string]DiscoveredService {
    services := make(map[string]DiscoveredService)
    
    // Common Kubernetes infrastructure services
    infraPatterns := map[string]string{
        "prometheus":     `kube_prometheus_prometheus`,
        "alertmanager":  `alertmanager`,
        "grafana":       `grafana`,
        "loki":          `loki`,
        "tempo":         `tempo`,
        "kube-state-metrics": `kube_state_metrics`,
        "node-exporter": `node_exporter`,
        "dns":           `kube_dns`,
    }
    
    for name, pattern := range infraPatterns {
        // Check if metrics exist for this infra service
        query := fmt.Sprintf(`{__name__=~"%s.*"}`, pattern)
        series, err := d.prometheus.Series(query, time.Hour)
        
        if err == nil && len(series) > 0 {
            services[name] = DiscoveredService{
                Name:       name,
                Type:       "infrastructure",
                HasTraffic: true, // Infra services always have traffic
                Metrics:    extractMetricNames(series),
                FirstSeen:  time.Now(),
            }
        }
    }
    
    return services
}
```

#### **2.3 Flow Generation from Discovery**
```go
// internal/flow/generator.go
type FlowGenerator struct {
    discovery *DiscoveryService
    presets   *config.PresetManager
}

func (g *FlowGenerator) GenerateFlows() ([]flow.Flow, error) {
    // 1. Discover services
    discovered, err := g.discovery.DiscoverServicesFromPrometheus()
    if err != nil {
        return nil, fmt.Errorf("service discovery failed: %w", err)
    }
    
    // 2. Group services into flows based on dependencies
    flows := g.buildFlowsFromDependencies(discovered)
    
    // 3. Apply preset templates
    if err := g.applyPresetTemplates(flows); err != nil {
        return nil, fmt.Errorf("failed to apply presets: %w", err)
    }
    
    return flows, nil
}

func (g *FlowGenerator) buildFlowsFromDependencies(discovered *DiscoveredServices) []flow.Flow {
    // Create service dependency graph
    graph := g.buildDependencyGraph(discovered.Dependencies)
    
    // Identify flow boundaries (connected components or user-defined flows)
    flows := make([]flow.Flow, 0)
    
    // Application services get their own flows
    for _, service := range discovered.Services {
        if service.Type == "application" && service.HasTraffic {
            flow := flow.Flow{
                ID:       service.Name,
                Services: g.getDownstreamServices(service.Name, graph),
                SLOs:     make([]flow.SLO, 0),
            }
            flows = append(flows, flow)
        }
    }
    
    // Infrastructure services get grouped
    infraFlow := flow.Flow{
        ID:       "infrastructure",
        Services: g.getInfrastructureServices(discovered.Services),
        SLOs:     make([]flow.SLO, 0),
    }
    flows = append(flows, infraFlow)
    
    return flows
}

func (g *FlowGenerator) applyPresetTemplates(flows []flow.Flow) error {
    // Get active preset
    preset, err := g.presets.GetActivePreset()
    if err != nil {
        return fmt.Errorf("no active preset: %w", err)
    }
    
    for i := range flows {
        flow := &flows[i]
        
        // Apply SLO templates to each service in the flow
        for _, service := range flow.Services {
            for _, sloTemplate := range preset.SLOTemplates {
                slo := g.expandSLOTemplate(sloTemplate, service)
                flow.SLOs = append(flow.SLOs, slo)
            }
        }
    }
    
    return nil
}

func (g *FlowGenerator) expandSLOTemplate(template config.SLOTemplate, serviceName string) flow.SLO {
    return flow.SLO{
        ID:          fmt.Sprintf("%s_%s", serviceName, template.Name),
        Service:     serviceName,
        Type:        template.Type,
        Objective:   template.Objective,
        Window:      template.Window,
        PromQL:      strings.ReplaceAll(template.PromQL, "$SERVICE", serviceName),
        TotalQuery:  strings.ReplaceAll(template.TotalQuery, "$SERVICE", serviceName),
        ErrorQuery:  strings.ReplaceAll(template.ErrorQuery, "$SERVICE", serviceName),
    }
}
```

### **Phase 3: Enhanced Wizard Integration**

#### **3.1 Discovery-First Wizard**
```go
// internal/config/wizard_enhanced.go
func RunEnhancedWizard() (string, Config, error) {
    fmt.Println("🔍 Enhanced Health Monitor Setup")
    fmt.Println("================================")
    
    // Step 1: Basic configuration
    cfg := Default()
    
    // Step 2: Detect observability stack
    fmt.Println("\n📊 Detecting observability stack...")
    stack, err := detectObservabilityStack()
    if err != nil {
        fmt.Printf("⚠️  Could not auto-detect stack: %v\n", err)
        stack = configureManually()
    } else {
        fmt.Printf("✅ Detected: %s\n", stack.Description)
    }
    
    // Apply stack configuration
    cfg.PrometheusURL = stack.PrometheusURL
    cfg.LokiURL = stack.LokiURL
    cfg.TraceURL = stack.TraceURL
    cfg.ServiceLabel = stack.ServiceLabel
    
    // Step 3: Discover services
    fmt.Println("\n🔍 Discovering services...")
    discovery := &discovery.DiscoveryService{
        prometheus: createPrometheusClient(cfg),
        config:     &cfg,
    }
    
    discovered, err := discovery.DiscoverServicesFromPrometheus()
    if err != nil {
        fmt.Printf("⚠️  Service discovery failed: %v\n", err)
        discovered = &discovery.DiscoveredServices{Services: []discovery.DiscoveredService{}}
    }
    
    fmt.Printf("✅ Found %d services:\n", len(discovered.Services))
    for _, service := range discovered.Services {
        status := "🟢"
        if !service.HasTraffic {
            status = "⚪"
        }
        fmt.Printf("   %s %s (%s)\n", status, service.Name, service.Type)
    }
    
    // Step 4: Generate flows automatically
    fmt.Println("\n🔗 Generating service flows...")
    generator := &flow.FlowGenerator{
        discovery: discovery,
        presets:   GetPresetManager(),
    }
    
    generatedFlows, err := generator.GenerateFlows()
    if err != nil {
        return "", cfg, fmt.Errorf("flow generation failed: %w", err)
    }
    
    fmt.Printf("✅ Generated %d flows:\n", len(generatedFlows))
    for _, flow := range generatedFlows {
        fmt.Printf("   📁 %s (%d services)\n", flow.ID, len(flow.Services))
    }
    
    // Step 5: Save configuration
    profileName := "auto-discovered"
    if err := SaveNewProfile(profileName, cfg); err != nil {
        return "", cfg, fmt.Errorf("failed to save profile: %w", err)
    }
    
    // Step 6: Save flows
    if err := flow.SaveFlows(profileName, generatedFlows); err != nil {
        return "", cfg, fmt.Errorf("failed to save flows: %w", err)
    }
    
    fmt.Printf("\n🎉 Setup complete!\n")
    fmt.Printf("📁 Profile: %s\n", profileName)
    fmt.Printf("🔗 Flows: %d\n", len(generatedFlows))
    fmt.Printf("📊 SLOs: %d\n", countSLOs(generatedFlows))
    
    return profileName, cfg, nil
}
```

#### **3.2 Enhanced Preset System**
```go
// internal/config/presets_enhanced.go
type PresetManager struct {
    presets map[string]EnhancedPreset
}

type EnhancedPreset struct {
    Name         string        `yaml:"name"`
    Description  string        `yaml:"description"`
    Type         string        `yaml:"type"` // "application", "infrastructure", "mixed"
    
    // SLO Templates (not fixed SLOs)
    SLOTemplates []SLOTemplate `yaml:"slo_templates"`
    
    // Alert Templates
    AlertTemplates []AlertTemplate `yaml:"alert_templates"`
    
    // Service patterns
    ServicePatterns []ServicePattern `yaml:"service_patterns"`
}

type SLOTemplate struct {
    Name        string  `yaml:"name"`
    Type        string  `yaml:"type"` // "ratio", "latency"
    Objective   float64 `yaml:"objective"`
    Window      string  `yaml:"window"`
    
    // Template variables
    PromQL      string `yaml:"promql"`
    TotalQuery  string `yaml:"total_query"`
    ErrorQuery  string `yaml:"error_query"`
    
    // Conditions
    Conditions  []string `yaml:"conditions"` // ["has_traffic", "is_application"]
}

// Example preset
var EnterprisePreset = EnhancedPreset{
    Name:        "enterprise",
    Description: "Production-grade monitoring for enterprise applications",
    Type:        "application",
    
    SLOTemplates: []SLOTemplate{
        {
            Name:      "availability",
            Type:      "ratio",
            Objective: 99.9,
            Window:    "5m",
            TotalQuery: `http_requests_total{service="$SERVICE"}`,
            ErrorQuery: `http_requests_total{service="$SERVICE",status!~"2.."}`,
            Conditions: []string{"has_traffic", "is_application"},
        },
        {
            Name:      "latency_p95",
            Type:      "latency", 
            Objective: 500, // ms
            Window:    "5m",
            PromQL:    `histogram_quantile(0.95, rate(http_request_duration_seconds_bucket{service="$SERVICE"}[5m])) * 1000`,
            Conditions: []string{"has_traffic", "is_application"},
        },
        {
            Name:      "infrastructure_uptime",
            Type:      "ratio",
            Objective: 99.5,
            Window:    "5m", 
            TotalQuery: `up{job="$SERVICE"}`,
            ErrorQuery: `up{job="$SERVICE"} == 0`,
            Conditions: []string{"is_infrastructure"},
        },
    },
}
```

---

## **Implementation Plan**

### **Week 1: Fix SLO Status System**
- [ ] **Day 1-2**: Implement enhanced SLO status types and logic
- [ ] **Day 3**: Fix display logic in commands.go
- [ ] **Day 4**: Add comprehensive unit tests for edge cases
- [ ] **Day 5**: Test with real data and validate status handling

### **Week 2: Service Discovery Foundation**
- [ ] **Day 1-2**: Implement discovery service architecture
- [ ] **Day 3**: Add Prometheus service discovery
- [ ] **Day 4**: Add infrastructure service detection
- [ ] **Day 5**: Implement route and dependency discovery

### **Week 3: Flow Generation System**
- [ ] **Day 1-2**: Build flow generator from discovered services
- [ ] **Day 3**: Implement preset template system
- [ ] **Day 4**: Add SLO template expansion
- [ ] **Day 5**: Test flow generation with discovered services

### **Week 4: Enhanced Wizard Integration**
- [ ] **Day 1-2**: Update wizard to be discovery-first
- [ ] **Day 3**: Add observability stack detection
- [ ] **Day 4**: Integrate automatic flow generation
- [ ] **Day 5**: Test complete setup flow

### **Week 5: Testing & Validation**
- [ ] **Day 1-2**: Deploy OpenTelemetry demo microservices
- [ ] **Day 3**: Test service discovery with real services
- [ ] **Day 4**: Validate SLO calculations with real metrics
- [ ] **Day 5**: Performance testing and optimization

---

## **Testing Strategy**

### **Test Environment Setup**
```bash
# 1. Deploy test microservices
kubectl create ns otel-demo
kubectl apply -n otel-demo -f https://raw.githubusercontent.com/open-telemetry/opentelemetry-demo/main/kubernetes/opentelemetry-demo.yaml

# 2. Port forward services
kubectl port-forward -n otel-demo svc/frontend 8080:8080
kubectl port-forward -n monitoring svc/prometheus-kube-prom-prometheus 9090:9090

# 3. Generate test traffic
while true; do curl -s http://localhost:8080 > /dev/null; done
```

### **SLO Status Testing**
```bash
# Test 1: No traffic scenario
health-monitor slo list
# Expected: NO_TRAFFIC status for services without traffic

# Test 2: Error scenario
kubectl scale deploy paymentservice --replicas=0 -n otel-demo
health-monitor slo list  
# Expected: BREACHING status for payment failures

# Test 3: Recovery scenario
kubectl scale deploy paymentservice --replicas=1 -n otel-demo
health-monitor slo list
# Expected: OK status after recovery
```

### **Service Discovery Testing**
```bash
# Test discovery
health-monitor discover services
# Expected: List of discovered services with traffic status

# Test flow generation  
health-monitor generate flows --from-discovery
# Expected: Flows generated from discovered services

# Test complete setup
health-monitor --init --discovery-first
# Expected: Complete setup with discovered services
```

---

## **Success Criteria**

### **SLO System Fixes**
✅ **No Traffic ≠ Breach**: Services with no traffic show "NO_TRAFFIC" not "BREACHING"  
✅ **Clear Status Distinction**: ERROR, NO_DATA, NO_TRAFFIC, BREACHING states are distinct  
✅ **Proper Risk Assessment**: Risk only calculated for services with sufficient data  
✅ **Backward Compatibility**: Existing functionality remains unchanged  

### **Service Discovery System**
✅ **Automatic Detection**: Discovers real services from Prometheus metrics  
✅ **Infrastructure Awareness**: Identifies Kubernetes infrastructure services  
✅ **Flow Generation**: Creates logical flows from service dependencies  
✅ **Template Application**: Applies preset SLOs to discovered services  

### **User Experience**
✅ **Discovery-First Setup**: Wizard discovers services before creating flows  
✅ **Real Service Monitoring**: Only monitors services that actually exist  
✅ **Intelligent Defaults**: Applies appropriate SLOs based on service type  
✅ **Visual Feedback**: Shows discovered services and generated flows  

---

## **Migration Path**

### **For Existing Users**
1. **Backward Compatibility**: Existing profiles continue to work unchanged
2. **Optional Migration**: Users can run `health-monitor migrate-to-discovery` to upgrade
3. **Gradual Adoption**: New features available via feature flags

### **For New Users**
1. **Discovery-First**: Default setup uses service discovery
2. **Better Experience**: No more broken SLOs for non-existent services
3. **Production Ready**: Works with real microservices from day one

This plan addresses both the immediate SLO status issue and the long-term service discovery problem, ensuring your health-monitor agent works correctly with both existing infrastructure and real application services.
