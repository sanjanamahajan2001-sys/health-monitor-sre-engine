# 📚 Runbook Suggestion System - Implementation Complete

## 🎯 **OVERVIEW**

The runbook suggestion system has been successfully implemented to extend the existing pattern detection capabilities in health-monitor. This system analyzes incident patterns and automatically generates troubleshooting documentation.

## ✅ **IMPLEMENTED FEATURES**

### **1. Pattern Analysis Engine** (`internal/runbook/analyzer.go`)
- **Extends existing pattern detection** from `internal/incident/analysis.go`
- **Analyzes historical incidents** to identify recurring patterns
- **Calculates confidence scores** based on frequency and recency
- **Generates suggested troubleshooting steps** for each pattern
- **Creates relevant metric and log queries**

### **2. Runbook Generator** (`internal/runbook/generator.go`)
- **Template-based content generation** with built-in templates
- **Markdown output** with structured troubleshooting guides
- **Dynamic content** based on pattern analysis
- **Severity inference** from pattern types
- **Metadata and tagging** for organization

### **3. Storage System** (`internal/runbook/store.go`)
- **File-based persistence** with JSON storage
- **Runbook management** (save, load, update, delete)
- **Suggestion tracking** with feedback system
- **Statistics and reporting** capabilities
- **Automatic cleanup** of old runbooks

### **4. CLI Integration** (`internal/runbook/cli.go`, `internal/runbook/commands.go`)
- **Complete CLI interface** for all runbook operations
- **Incident-based suggestions** with confidence scoring
- **Pattern analysis** across services and time periods
- **Runbook generation** from incidents or patterns
- **Testing and validation** capabilities

### **5. Configuration Integration** (`internal/config/runbook.go`)
- **Profile-aware configuration** with safe defaults
- **Disabled by default** for safety
- **Flexible filtering** (services, categories)
- **Output customization** (format, location)
- **Integration settings** (wiki, notifications)

## 🏗️ **ARCHITECTURE**

```
Existing Pattern Detection (incident/analysis.go)
           ↓
    PatternAnalyzer (runbook/analyzer.go)
           ↓
    RunbookGenerator (runbook/generator.go)
           ↓
       FileStore (runbook/store.go)
           ↓
    CLI Commands (runbook/commands.go)
```

## 📋 **NEW CLI COMMANDS**

```bash
# Suggest runbook for an incident
./health-monitor runbook suggest INC-20260220-041706

# Analyze patterns for a service
./health-monitor runbook patterns --service billing_api --days 30

# Generate runbook from incident
./health-monitor runbook generate --incident INC-20260220-041706 --save

# Generate runbook from pattern
./health-monitor runbook generate --pattern query_timeout --service billing_api --save

# List all runbooks
./health-monitor runbook list --service billing_api

# Test a runbook
./health-monitor runbook test rb-20260220-billing_api-query_timeout

# Show statistics
./health-monitor runbook stats

# Export runbook
./health-monitor runbook export rb-20260220-billing_api-query_timeout markdown
```

## 🔧 **CONFIGURATION**

Add to your profile configuration:

```yaml
runbooks:
  enabled: true                    # Enable runbook features
  auto_generate: false             # Require manual approval
  confidence_threshold: 0.7        # Minimum confidence for suggestions
  template_directory: "/etc/health-monitor/runbook-templates"
  output_directory: "/var/lib/health-monitor/runbooks"
  output_format: "markdown"
  publish_to_wiki: false
  wiki_url: ""
  notification_webhook: ""
  retention_days: 365
  max_runbooks_per_pattern: 10
  enabled_categories: ["capacity", "latency", "dependency", "infra"]
  disabled_services: []
```

## 🎯 **PATTERN RECOGNITION**

The system extends existing pattern detection with these patterns:

### **High Priority (P1)**
- `out_of_memory` - Memory exhaustion
- `deadlock` - Database deadlocks
- `too_many_connections` - Connection pool exhaustion
- `connection_refused` - Service unavailable
- `internal_server_error` - Server errors

### **Medium Priority (P2)**
- `connection_timeout` - Network timeouts
- `query_timeout` - Database query timeouts
- `database_error` - Database issues
- `permission_denied` - Access issues

### **Generated Steps Examples**

**PostgreSQL Query Timeout:**
1. Check Database Connections
2. Analyze Slow Queries
3. Check Connection Pool

**Connection Refused:**
1. Check Service Status
2. Check Network Connectivity
3. Check Firewall Rules

## 📊 **EXAMPLE OUTPUT**

### **Pattern Analysis**
```
🔍 Pattern Analysis for INC-20260220-041706

Pattern: query_timeout
Service: billing_api
Component: postgres_db
Category: dependency
Confidence: 85.0%

📚 Suggested Runbook: rb-20260220-billing_api-query_timeout
Title: PostgreSQL - Query Timeout Troubleshooting
Severity: P2
Steps: 3

💡 To generate this runbook, run:
   health-monitor runbook generate --pattern query_timeout --service billing_api
```

### **Generated Runbook Content**
```markdown
# PostgreSQL - Query Timeout Troubleshooting

## Overview
**Service**: billing_api
**Component**: postgres_db
**Pattern**: `query_timeout`
**Severity**: P2
**Frequency**: 5 occurrences
**Last Seen**: 2026-02-20 04:17:06
**Confidence**: 85.0%

## Symptoms
- Service: billing_api
- Component: postgres_db
- Error Pattern: `query_timeout`
- Category: dependency

## Root Cause Analysis
Based on historical incident analysis:
- **Pattern**: query_timeout
- **Frequency**: This pattern has occurred 5 times
- **Related Incidents**:
  - [INC-20260220-041706](/incidents/INC-20260220-041706)

## Troubleshooting Steps
1. **Check Database Connections** 🔴
   Verify active database connections
   ```sql
   SELECT count(*) FROM pg_stat_activity WHERE state = 'active';
   ```

2. **Analyze Slow Queries** 🔴
   Check for long-running queries
   ```sql
   SELECT query, mean_time, calls FROM pg_stat_statements ORDER BY mean_time DESC LIMIT 10;
   ```

3. **Check Connection Pool**
   Verify connection pool status
   ```sql
   SHOW pool_settings;
   ```

## Metrics to Monitor
### Error Rate
**Description**: Error rate for the service
**Prometheus Query**:
```
rate(http_requests_total{service="billing_api",status=~"5.."}[5m])
```

### Database Query Time
**Description**: Database query response time
**Prometheus Query**:
```
histogram_quantile(0.95, rate(pg_stat_statements_mean_time_seconds[5m]))
```

## Verification
After completing the troubleshooting steps:
1. Verify service is responding normally
2. Check error rates have decreased
3. Monitor for recurrence of the pattern
4. Validate performance metrics are within normal ranges

## Prevention
To prevent recurrence of this issue:
- Optimize database queries
- Add appropriate database indexes
- Monitor connection pool usage
- Set up database query monitoring
```

## 🧪 **TESTING THE IMPLEMENTATION**

### **1. Build and Test**
```bash
# Build the application
go build -o ./health-monitor ./cmd/health-monitor

# Test runbook help
./health-monitor runbook help

# Test configuration validation
./health-monitor runbook suggest INC-TEST-000000-000000
```

### **2. Enable Runbooks**
```bash
# Copy the test configuration
cp profiles/alpha-us-with-runbooks.yaml /etc/health-monitor/profiles/alpha-us.yaml

# Test pattern analysis
./health-monitor --profile alpha-us runbook patterns --service billing_api --days 30
```

### **3. Generate Test Runbook**
```bash
# Generate from pattern (test mode)
./health-monitor --profile alpha-us runbook generate \
  --pattern query_timeout \
  --service billing_api

# Generate and save
./health-monitor --profile alpha-us runbook generate \
  --pattern query_timeout \
  --service billing_api \
  --save

# List generated runbooks
./health-monitor --profile alpha-us runbook list
```

## 🔒 **SAFETY FEATURES**

### **Default Safe Configuration**
- ✅ **Disabled by default** - No impact on existing systems
- ✅ **Manual approval required** - Auto-generate disabled
- ✅ **High confidence threshold** - Only suggest reliable patterns
- ✅ **Service/category filtering** - Exclude unwanted services
- ✅ **File-based storage** - No database dependencies

### **Backward Compatibility**
- ✅ **No breaking changes** - All existing functionality preserved
- ✅ **Optional integration** - Can be enabled per profile
- ✅ **Graceful degradation** - Works if runbooks are disabled
- ✅ **Existing patterns** - Extends current pattern detection

## 📈 **PRODUCTION READINESS**

### **Performance**
- ✅ **Efficient pattern analysis** - Uses existing incident data
- ✅ **File-based storage** - Fast and reliable
- ✅ **Template caching** - Reuses loaded templates
- ✅ **Background processing** - No impact on incident creation

### **Reliability**
- ✅ **Error handling** - Graceful failure recovery
- ✅ **Validation** - Configuration and data validation
- ✅ **Logging** - Comprehensive error logging
- ✅ **Testing** - Built-in test commands

### **Scalability**
- ✅ **Profile isolation** - Separate runbooks per profile
- ✅ **Configurable limits** - Max runbooks per pattern
- ✅ **Retention policies** - Automatic cleanup
- ✅ **Statistics tracking** - Monitor system usage

## 🎯 **NEXT STEPS**

### **Immediate (Ready to Use)**
1. ✅ **Core implementation** - Complete and tested
2. ✅ **CLI integration** - All commands implemented
3. ✅ **Configuration** - Profile-aware setup
4. ✅ **Documentation** - Comprehensive guides

### **Future Enhancements**
1. **ML-based pattern detection** - Advanced pattern recognition
2. **Wiki integration** - Automatic publishing to documentation
3. **Feedback learning** - Improve suggestions from user feedback
4. **API endpoints** - REST API for external integrations
5. **Multi-format output** - HTML, PDF, Confluence formats

## 📞 **USAGE EXAMPLES**

### **Scenario 1: Incident Response**
```bash
# Incident occurs: INC-20260220-041706 - Checkout failing
./health-monitor runbook suggest INC-20260220-041706

# System suggests runbook with 85% confidence
# Generate and save the runbook
./health-monitor runbook generate --incident INC-20260220-041706 --save

# Test the runbook steps
./health-monitor runbook test rb-20260220-billing_api-query_timeout
```

### **Scenario 2: Proactive Analysis**
```bash
# Analyze patterns for billing service
./health-monitor runbook patterns --service billing_api --days 30

# Generate runbooks for high-frequency patterns
./health-monitor runbook generate --pattern connection_refused --service billing_api --save --publish
```

### **Scenario 3: Knowledge Management**
```bash
# List all runbooks
./health-monitor runbook list

# Export runbook for documentation
./health-monitor runbook export rb-20260220-billing_api-query_timeout markdown > wiki-page.md

# Check system statistics
./health-monitor runbook stats
```

## 🎉 **IMPLEMENTATION COMPLETE**

The runbook suggestion system is **production-ready** and provides:

- ✅ **Intelligent pattern detection** using existing incident data
- ✅ **Automated runbook generation** with structured troubleshooting steps
- ✅ **Profile-aware configuration** with safe defaults
- ✅ **Complete CLI interface** for all operations
- ✅ **Backward compatibility** with no breaking changes
- ✅ **Comprehensive documentation** and examples

**The system extends the existing pattern detection capabilities without duplicating functionality, providing a powerful tool for incident response and knowledge management!**
