# 🚀 Runbook Suggestions - Complete User Guide

## 📋 **Table of Contents**
1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [Configuration](#configuration)
4. [Command Reference](#command-reference)
5. [Pattern Recognition](#pattern-recognition)
6. [Custom Patterns](#custom-patterns)
7. [Production Examples](#production-examples)
8. [Troubleshooting](#troubleshooting)

---

## 🎯 **Overview**

The Runbook Suggestions system provides intelligent, automated runbook recommendations for incidents based on error patterns detected in logs. It helps responders quickly identify relevant troubleshooting procedures and reduces mean time to resolution (MTTR).

### **Key Features:**
- **Real-time Suggestions**: Generated immediately after incident creation
- **20+ Built-in Patterns**: Database, HTTP, Resource, Security, and Performance patterns
- **Custom Patterns**: Define service-specific error patterns
- **Confidence Scoring**: 70%-95% confidence based on pattern specificity
- **CLI/TUI Integration**: View suggestions in incident details
- **Metrics Tracking**: Monitor suggestion accuracy and usage

---

## 🚀 **Quick Start**

### **1. Enable Runbook Suggestions**
```yaml
# profiles/your-profile.yaml
runbook_suggestions:
  enabled: true
  confidence_threshold: 0.7
  max_suggestions: 3
  metrics_enabled: true
```

### **2. Create an Incident**
```bash
# Push error logs
NOW=$(($(date +%s) * 1000000000))
curl -X POST http://localhost:3100/loki/api/v1/push \
  -H "Content-Type: application/json" \
  -d '{
    "streams": [
      {
        "stream": {"service": "billing_api"},
        "values": [
          ["'$NOW'", "ERROR postgres connection timeout during checkout"]
        ]
      }
    ]
  }'

# Create incident
sudo ./health-monitor --profile core-platform incident start \
  --service billing_api \
  --title "Database Connection Issues" \
  --severity P1
```

### **3. View Suggestions**
```bash
# View incident with runbook suggestions
sudo ./health-monitor --profile core-platform incident view

# View in TUI mode
sudo ./health-monitor --profile core-platform incident view --tui
```

---

## ⚙️ **Configuration**

### **Complete Configuration Options**
```yaml
runbook_suggestions:
  # Basic settings
  enabled: true                          # Enable/disable suggestions
  confidence_threshold: 0.7             # Minimum confidence (0.0-1.0)
  max_suggestions: 3                    # Maximum suggestions per incident
  cache_ttl: 3600                       # Cache TTL in seconds
  
  # Pattern settings
  custom_patterns:                      # Custom error patterns
    - name: "billing_service_error"
      pattern: "billing service unavailable"
      runbook: "rb-billing-service-recovery"
      confidence: 0.9
      description: "Billing service failure handling"
  
  # Feature flags
  metrics_enabled: true                 # Track suggestion metrics
  feedback_enabled: false               # User feedback system
  ml_model_enabled: false               # ML-based suggestions
  
  # Service filtering
  disabled_services: []                 # Services to exclude
  enabled_categories:                   # Incident categories to include
    - capacity
    - latency
    - dependency
    - infra
```

---

## 📖 **Command Reference**

### **Incident Management Commands**

#### **Start Incident with Runbook Suggestions**
```bash
# Basic incident creation
sudo ./health-monitor --profile <profile> incident start \
  --service <service-name> \
  --title "<incident-title>" \
  --severity <P1|P2|P3|P4>

# Example with all options
sudo ./health-monitor --profile core-platform incident start \
  --service billing_api \
  --title "Payment Processing Failure" \
  --severity P1 \
  --description "Customers unable to complete payments" \
  --impacted-flows "Checkout Flow,Payment Flow"
```

#### **View Incident with Suggestions**
```bash
# CLI view (default)
sudo ./health-monitor --profile core-platform incident view

# TUI view (interactive)
sudo ./health-monitor --profile core-platform incident view --tui

# Copy incident details
sudo ./health-monitor --profile core-platform incident view --copy all

# Export to JSON
sudo ./health-monitor --profile core-platform incident view --format json
```

#### **Resolve Incident**
```bash
# Basic resolution
sudo ./health-monitor --profile core-platform incident resolve \
  --incident-id INC-20260220-101436 \
  --resolution "Fixed postgres connection pool issue"

# Resolution with RCA
sudo ./health-monitor --profile core-platform incident resolve \
  --incident-id INC-20260220-101436 \
  --resolution "Increased connection pool size" \
  --rca-cause "Database connection exhaustion" \
  --rca-impact "Payment processing failures" \
  --rca-prevention "Implement connection pooling monitoring"
```

### **Runbook-Specific Commands**

#### **Get Detailed Runbook Suggestions**
```bash
# Get suggestions for specific incident
sudo ./health-monitor --profile core-platform runbook suggest INC-20260220-101436

# Get suggestions with verbose output
sudo ./health-monitor --profile core-platform runbook suggest INC-20260220-101436 --verbose

# Export suggestions to file
sudo ./health-monitor --profile core-platform runbook suggest INC-20260220-101436 --output suggestions.json
```

#### **Manage Custom Patterns**
```bash
# List all custom patterns
sudo ./health-monitor --profile core-platform runbook patterns list

# Add custom pattern
sudo ./health-monitor --profile core-platform runbook patterns add \
  --name "redis_connection_failure" \
  --pattern "redis connection refused" \
  --runbook "rb-redis-recovery" \
  --confidence 0.85

# Remove custom pattern
sudo ./health-monitor --profile core-platform runbook patterns remove redis_connection_failure
```

#### **Feedback System** (When enabled)
```bash
# Mark suggestion as helpful
sudo ./health-monitor --profile core-platform runbook feedback INC-20260220-101436 \
  --helpful true \
  --pattern "postgres_connection_timeout"

# Mark suggestion as not helpful
sudo ./health-monitor --profile core-platform runbook feedback INC-20260220-101436 \
  --helpful false \
  --reason "Pattern was too generic"
```

---

## 🧠 **Pattern Recognition**

### **Built-in Pattern Categories**

#### **Database Patterns**
| Pattern | Confidence | Runbook | Description |
|---------|------------|---------|-------------|
| `postgres_connection_timeout` | 95% | `rb-postgres-connection-timeout` | PostgreSQL connection timeout |
| `postgres_connection_failed` | 95% | `rb-postgres-connection-failed` | PostgreSQL connection failure |
| `database_deadlock` | 95% | `rb-database-deadlock-resolution` | Database deadlock detected |
| `transaction_rollback` | 85% | `rb-transaction-rollback-handling` | Transaction rollback issues |

#### **HTTP/API Patterns**
| Pattern | Confidence | Runbook | Description |
|---------|------------|---------|-------------|
| `http_502_bad_gateway` | 95% | `rb-502-bad-gateway-troubleshooting` | Bad Gateway error |
| `http_503_service_unavailable` | 95% | `rb-503-service-unavailable-troubleshooting` | Service Unavailable |
| `http_504_gateway_timeout` | 95% | `rb-504-gateway-timeout-troubleshooting` | Gateway Timeout |
| `http_429_rate_limit` | 95% | `rb-429-rate-limiting` | Rate limiting exceeded |

#### **Resource Patterns**
| Pattern | Confidence | Runbook | Description |
|---------|------------|---------|-------------|
| `memory_pressure` | 85% | `rb-memory-pressure-resolution` | Memory exhaustion |
| `disk_full` | 95% | `rb-disk-full-emergency` | Disk space exhausted |
| `performance_degradation` | 85% | `rb-performance-degradation-analysis` | Performance issues |

#### **Security Patterns**
| Pattern | Confidence | Runbook | Description |
|---------|------------|---------|-------------|
| `authentication_issue` | 85% | `rb-authentication-troubleshooting` | Authentication failures |
| `ssl_tls_issue` | 85% | `rb-ssl-tls-certificate-issues` | SSL/TLS certificate problems |

---

## 🔧 **Custom Patterns**

### **Defining Custom Patterns**

#### **Via Configuration File**
```yaml
runbook_suggestions:
  custom_patterns:
    - name: "payment_processor_down"
      pattern: "payment processor service unavailable"
      runbook: "rb-payment-processor-failover"
      confidence: 0.9
      description: "Payment processor service failure"
    
    - name: "cache_miss_spike"
      pattern: "cache miss rate above threshold"
      runbook: "rb-cache-optimization"
      confidence: 0.8
      description: "Cache performance degradation"
```

#### **Via CLI Commands**
```bash
# Add custom pattern interactively
sudo ./health-monitor --profile core-platform runbook patterns add

# Add with all parameters
sudo ./health-monitor --profile core-platform runbook patterns add \
  --name "api_rate_limit_exceeded" \
  --pattern "rate limit exceeded" \
  --runbook "rb-api-rate-limit-handling" \
  --confidence 0.85 \
  --description "API rate limiting issues"
```

### **Custom Pattern Best Practices**
1. **Be Specific**: Use exact error messages when possible
2. **Set Appropriate Confidence**: 95% for very specific, 70-85% for general
3. **Descriptive Names**: Use clear, actionable pattern names
4. **Test Patterns**: Verify patterns match intended errors
5. **Document Runbooks**: Ensure suggested runbooks exist and are helpful

---

## 🎯 **Production Examples**

### **Example 1: Database Connection Issues**
```bash
# Step 1: Simulate database errors
NOW=$(($(date +%s) * 1000000000))
curl -X POST http://localhost:3100/loki/api/v1/push \
  -H "Content-Type: application/json" \
  -d '{
    "streams": [
      {
        "stream": {"service": "user_service"},
        "values": [
          ["'$NOW'", "ERROR postgres connection timeout after 30 seconds"],
          ["'$((NOW + 1000000000))'", "ERROR connection pool exhausted"]
        ]
      }
    ]
  }'

# Step 2: Create incident
sudo ./health-monitor --profile core-platform incident start \
  --service user_service \
  --title "Database Connection Pool Exhaustion" \
  --severity P1

# Step 3: View suggestions
sudo ./health-monitor --profile core-platform incident view

# Expected output:
# Pattern detected: postgres_connection_timeout
# Confidence: 95%
# Suggested runbook: rb-postgres-connection-timeout
```

### **Example 2: HTTP Service Degradation**
```bash
# Step 1: Simulate HTTP errors
NOW=$(($(date +%s) * 1000000000))
curl -X POST http://localhost:3100/loki/api/v1/push \
  -H "Content-Type: application/json" \
  -d '{
    "streams": [
      {
        "stream": {"service": "api_gateway"},
        "values": [
          ["'$NOW'", "ERROR 502 Bad Gateway from upstream service"],
          ["'$((NOW + 2000000000))'", "ERROR upstream service timeout"]
        ]
      }
    ]
  }'

# Step 2: Create incident
sudo ./health-monitor --profile core-platform incident start \
  --service api_gateway \
  --title "Upstream Service Unavailable" \
  --severity P2 \
  --impacted-flows "API Requests,External Integrations"

# Step 3: View in TUI
sudo ./health-monitor --profile core-platform incident view --tui

# Expected output:
# Pattern detected: http_502_bad_gateway
# Confidence: 95%
# Suggested runbook: rb-502-bad-gateway-troubleshooting
```

### **Example 3: Custom Pattern Matching**
```bash
# Step 1: Add custom pattern (if not in config)
sudo ./health-monitor --profile core-platform runbook patterns add \
  --name "third_party_payment_failure" \
  --pattern "stripe payment processing failed" \
  --runbook "rb-stripe-payment-recovery" \
  --confidence 0.9

# Step 2: Simulate custom error
NOW=$(($(date +%s) * 1000000000))
curl -X POST http://localhost:3100/loki/api/v1/push \
  -H "Content-Type: application/json" \
  -d '{
    "streams": [
      {
        "stream": {"service": "billing_service"},
        "values": [
          ["'$NOW'", "ERROR stripe payment processing failed: card declined"]
        ]
      }
    ]
  }'

# Step 3: Create incident
sudo ./health-monitor --profile core-platform incident start \
  --service billing_service \
  --title "Payment Processing Failures" \
  --severity P1

# Step 4: View suggestions
sudo ./health-monitor --profile core-platform incident view

# Expected output:
# Pattern detected: third_party_payment_failure
# Confidence: 90%
# Suggested runbook: rb-stripe-payment-recovery
```

---

## 🔍 **Troubleshooting**

### **Common Issues**

#### **Suggestions Not Generated**
```bash
# Check if suggestions are enabled
grep -A 5 "runbook_suggestions" profiles/core-platform.yaml

# Check logs for errors
sudo journalctl -u health-monitor -f | grep "runbook"

# Verify configuration
sudo ./health-monitor --profile core-platform config validate
```

#### **Low Confidence Suggestions**
```bash
# Check confidence threshold
grep "confidence_threshold" profiles/core-platform.yaml

# View pattern matching details
sudo ./health-monitor --profile core-platform runbook suggest INC-20260220-101436 --verbose
```

#### **Custom Patterns Not Working**
```bash
# List custom patterns
sudo ./health-monitor --profile core-platform runbook patterns list

# Test pattern matching
sudo ./health-monitor --profile core-platform runbook patterns test \
  --pattern "your error message" \
  --service your_service
```

### **Debug Mode**
```bash
# Enable debug logging
export LOG_LEVEL=debug

# Create incident with debug output
sudo ./health-monitor --profile core-platform incident start \
  --service test_service \
  --title "Debug Test" \
  --severity P4 \
  --debug
```

### **Performance Issues**
```bash
# Check metrics
sudo ./health-monitor --profile core-platform metrics runbook

# Monitor suggestion generation time
sudo ./health-monitor --profile core-platform metrics --filter "runbook_suggestions_duration"
```

---

## 📊 **Metrics and Monitoring**

### **Available Metrics**
- `health_monitor_runbook_suggestions_total`: Total suggestions generated
- `health_monitor_runbook_suggestions_duration_seconds`: Time to generate suggestions
- `health_monitor_pattern_match_count{pattern="..."}`: Pattern match frequency
- `health_monitor_suggestion_confidence_score`: Confidence distribution

### **Viewing Metrics**
```bash
# View all runbook metrics
sudo ./health-monitor --profile core-platform metrics runbook

# Export metrics to Prometheus format
sudo ./health-monitor --profile core-platform metrics --format prometheus

# View metrics dashboard
sudo ./health-monitor --profile core-platform dashboard runbook
```

---

## 🎓 **Best Practices**

### **Configuration Best Practices**
1. **Set Appropriate Thresholds**: Use 0.7 for production, 0.5 for development
2. **Limit Suggestions**: Set max_suggestions to 3-5 to avoid overwhelming users
3. **Enable Metrics**: Always enable metrics in production for monitoring
4. **Custom Patterns**: Start with high confidence (0.9) for custom patterns

### **Operational Best Practices**
1. **Review Suggestions**: Regularly review suggestion accuracy
2. **Update Patterns**: Add new patterns as new error types emerge
3. **Monitor Performance**: Track suggestion generation time and accuracy
4. **Train Responders**: Ensure team knows how to use suggestions effectively

### **Runbook Development**
1. **Keep Runbooks Current**: Regularly update runbook procedures
2. **Standardize Format**: Use consistent runbook structure
3. **Include Examples**: Add specific commands and expected outputs
4. **Test Procedures**: Verify runbook steps actually resolve issues

---

## 🔮 **Advanced Features**

### **Machine Learning Integration** (Future)
```yaml
runbook_suggestions:
  ml_model_enabled: true
  ml_model_path: "/models/runbook_suggestions.model"
  ml_confidence_threshold: 0.8
```

### **Feedback System** (Future)
```bash
# Enable feedback collection
sudo ./health-monitor --profile core-platform config set \
  runbook_suggestions.feedback_enabled true

# View feedback analytics
sudo ./health-monitor --profile core-platform analytics feedback
```

### **Multi-Service Correlation** (Future)
```bash
# Detect related incidents across services
sudo ./health-monitor --profile core-platform correlate \
  --time-window 1h \
  --pattern "database_connection"
```

---

## 📞 **Support and Resources**

### **Getting Help**
- **Documentation**: Check this guide and API docs
- **Community**: Join the health-monitor Slack channel
- **Issues**: Report bugs on GitHub
- **Training**: Request team training sessions

### **Contributing**
- **Patterns**: Submit new pattern suggestions
- **Runbooks**: Contribute runbook templates
- **Documentation**: Improve this guide
- **Code**: Submit pull requests for enhancements

---

**🎉 You're now ready to use the Runbook Suggestions system effectively!**

For additional support or questions, refer to the troubleshooting section or contact the health-monitor team.
