# 🔒 Health Monitor Security Guide

## Overview

Health Monitor includes enterprise-grade security features that are **disabled by default** to maintain backward compatibility. This guide shows how to enable and configure security features safely.

## 🚀 Quick Start

### 1. Validate Current Configuration
```bash
./health-monitor security validate-config --profile alpha-us
```

### 2. Check Security Status
```bash
./health-monitor security status --profile alpha-us
```

### 3. Test Security Features
```bash
./health-monitor security test --profile alpha-us
```

## 🛡️ Security Features

### 🔐 Webhook Authentication

**Purpose**: Authenticate incoming webhook requests from monitoring systems.

**Configuration**:
```yaml
security:
  webhook:
    auth:
      enabled: true
      tokens:
        - name: "prometheus"
          token: "Bearer your-prometheus-token"
          description: "Prometheus alertmanager"
        - name: "grafana"
          token: "Bearer your-grafana-token"
          description: "Grafana alerts"
```

**Usage**:
```bash
# With authentication
curl -H "Authorization: Bearer your-prometheus-token" \
     -H "Content-Type: application/json" \
     -d '{"alerts": [{"labels": {"alertname": "Test"}}]}' \
     http://localhost:9095/webhook

# Without authentication (will fail)
curl -H "Content-Type: application/json" \
     -d '{"alerts": [{"labels": {"alertname": "Test"}}]}' \
     http://localhost:9095/webhook
```

### 🌐 IP Whitelist

**Purpose**: Restrict webhook access to specific IP addresses.

**Configuration**:
```yaml
security:
  webhook:
    auth:
      ip_whitelist:
        enabled: true
        allowed_ips:
          - "10.0.0.0/8"      # Private networks
          - "192.168.1.0/24"  # Specific office network
          - "127.0.0.1"       # Localhost
```

### ⏱️ Rate Limiting

**Purpose**: Prevent webhook floods and DoS attacks.

**Configuration**:
```yaml
security:
  webhook:
    rate_limit:
      enabled: true
      requests_per_minute: 1000
      burst_size: 100
```

**Testing**:
```bash
# Test rate limiting with rapid requests
for i in {1..150}; do
  curl -s http://localhost:9095/webhook \
    -H "Content-Type: application/json" \
    -d '{"alerts": [{"labels": {"alertname": "Test'$i'"}]}' &
done
wait
```

### ✅ Input Validation

**Purpose**: Validate webhook payloads to prevent malicious data.

**Configuration**:
```yaml
security:
  webhook:
    validation:
      enabled: true
      max_alerts_per_request: 100
      max_label_size: 1024
      max_annotation_size: 4096
      allowed_characters: "a-zA-Z0-9._-"
```

### 👥 Role-Based Access Control (RBAC)

**Purpose**: Control user access to different operations.

**Configuration**:
```yaml
security:
  rbac:
    enabled: true
    roles:
      operator:
        - "incident:list"
        - "incident:view"
        - "incident:resolve"
      admin:
        - "*"
    users:
      john: "operator"
      jane: "admin"
```

### 📋 Audit Logging

**Purpose**: Log all security-relevant events.

**Configuration**:
```yaml
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

**Log Format**:
```json
{
  "timestamp": "2025-02-19T13:14:21Z",
  "user": "webhook",
  "action": "webhook_request",
  "resource": "/webhook",
  "success": true,
  "details": "client_ip=192.168.1.100, method=POST"
}
```

### 🔑 API Keys

**Purpose**: Authenticate API requests instead of user credentials.

**Configuration**:
```yaml
security:
  api_keys:
    enabled: true
    keys:
      - name: "monitoring-system"
        key: "hm-api-abc123def456"
        permissions:
          - "incident:list"
          - "incident:view"
        description: "Monitoring system API key"
```

### 🔐 Data Encryption

**Purpose**: Encrypt sensitive configuration data.

**Generate Encryption Key**:
```bash
./health-monitor security generate-key --output-file /etc/health-monitor/encryption.key
```

**Configuration**:
```yaml
security:
  encryption:
    enabled: true
    key_file: "/etc/health-monitor/encryption.key"
    encrypted_fields:
      - "webhook_url"
      - "api_key"
      - "routing_key"
```

## 📊 Security Metrics

Monitor security metrics via the `/security-metrics` endpoint:

```bash
curl http://localhost:9095/security-metrics
```

**Response**:
```json
{
  "security_enabled": {
    "auth": true,
    "rate_limit": true,
    "validation": true
  }
}
```

Prometheus metrics are also available:
```bash
curl http://localhost:9095/metrics | grep security_
```

## 🧪 Testing Security

### Test Authentication
```bash
./health-monitor security test --profile alpha-us
```

### Test Individual Features
```bash
# Test webhook with valid token
curl -H "Authorization: Bearer valid-token" \
     -H "Content-Type: application/json" \
     -d '{"alerts": [{"labels": {"alertname": "Test"}}]}' \
     http://localhost:9095/webhook

# Test webhook with invalid token
curl -H "Authorization: Bearer invalid-token" \
     -H "Content-Type: application/json" \
     -d '{"alerts": [{"labels": {"alertname": "Test"}}]}' \
     http://localhost:9095/webhook
```

## 🔧 Configuration Management

### Environment Variables for Secrets
Use environment variables for sensitive data:

```bash
export SLACK_WEBHOOK_URL="https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK"
export PAGERDUTY_ROUTING_KEY="your-pagerduty-routing-key"
```

Reference in configuration:
```yaml
notifications:
  slack:
    webhook_url: "${SLACK_WEBHOOK_URL}"
  pagerduty:
    routing_key: "${PAGERDUTY_ROUTING_KEY}"
```

### Profile-Specific Security
Different profiles can have different security settings:

```yaml
# profiles/alpha-us-security.yaml
security:
  webhook:
    auth:
      enabled: true
      tokens:
        - name: "prod-prometheus"
          token: "Bearer prod-token"

# profiles/dev-security.yaml  
security:
  webhook:
    auth:
      enabled: false  # More permissive for development
```

## 🚨 Security Best Practices

### 1. Gradual Enablement
Enable security features one at a time:
1. Start with input validation
2. Add rate limiting
3. Enable authentication
4. Configure audit logging
5. Set up RBAC

### 2. Token Management
- Use unique tokens for each system
- Rotate tokens regularly
- Store tokens securely (environment variables, secret management)
- Use descriptive token names

### 3. IP Whitelist
- Be specific with IP ranges
- Include monitoring system IPs only
- Regularly review and update whitelist

### 4. Rate Limiting
- Set reasonable limits based on expected traffic
- Monitor rate limit hits
- Adjust limits based on usage patterns

### 5. Audit Logs
- Enable audit logging in production
- Regularly review audit logs
- Set up log rotation and monitoring
- Store logs securely

## 🔍 Troubleshooting

### Common Issues

**Authentication Failures**:
```bash
# Check if auth is enabled
./health-monitor security status --profile alpha-us

# Verify token format
curl -H "Authorization: Bearer your-token" http://localhost:9095/webhook
```

**IP Whitelist Issues**:
```bash
# Test IP from different source
curl -H "X-Forwarded-For: 192.168.1.100" http://localhost:9095/webhook

# Check whitelist configuration
./health-monitor security validate-config --profile alpha-us
```

**Rate Limiting**:
```bash
# Check rate limit metrics
curl http://localhost:9095/metrics | grep security_rate_limit

# Test rate limit behavior
./health-monitor security test --profile alpha-us
```

### Debug Mode
Enable debug logging to troubleshoot:
```bash
HEALTH_MONITOR_LOG_LEVEL=debug ./health-monitor --profile alpha-us alert-listen
```

## 📋 Security Checklist

### Before Production Deployment
- [ ] Validate all security configurations
- [ ] Test authentication with all tokens
- [ ] Verify IP whitelist entries
- [ ] Test rate limiting behavior
- [ ] Validate input rules
- [ ] Set up audit logging
- [ ] Configure RBAC if needed
- [ ] Generate and secure encryption keys
- [ ] Set up monitoring for security metrics
- [ ] Document security procedures

### Regular Maintenance
- [ ] Review and rotate authentication tokens
- [ ] Update IP whitelist as needed
- [ ] Monitor audit logs for suspicious activity
- [ ] Update rate limits based on usage
- [ ] Backup encryption keys
- [ ] Review user permissions in RBAC
- [ ] Test security features after updates

## 🆘 Emergency Procedures

### Disable Security Quickly
If security features cause issues, disable them immediately:

```yaml
security:
  webhook:
    auth:
      enabled: false
    rate_limit:
      enabled: false
    validation:
      enabled: false
```

### Emergency Rollback
```bash
# Restore previous configuration
cp /etc/health-monitor/backup/config.yaml /etc/health-monitor/config.yaml

# Restart service
systemctl restart health-monitor
```

## 📞 Support

For security-related issues:
1. Check this guide first
2. Review audit logs
3. Run security validation
4. Test with debug mode enabled
5. Contact security team if needed

---

**Remember**: All security features are **opt-in** and **disabled by default**. Enable them gradually and test thoroughly in development before production deployment.
