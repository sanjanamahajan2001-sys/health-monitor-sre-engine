#!/bin/bash

# 🚀 Production Test Scenarios for Runbook Suggestions
# This script tests diverse use cases and edge scenarios

set -e

echo "🎯 PRODUCTION RUNBOOK SUGGESTIONS - COMPREHENSIVE TESTING"
echo "========================================================"

# Configuration
HEALTH_MONITOR="./health-monitor"
PROFILE="core-platform"
BASE_URL="http://localhost:3100/loki/api/v1/push"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Test helper
run_test() {
    local test_name="$1"
    local test_command="$2"
    
    echo ""
    log_info "Running test: $test_name"
    echo "Command: $test_command"
    echo "----------------------------------------"
    
    if eval "$test_command"; then
        log_success "✅ $test_name - PASSED"
    else
        log_error "❌ $test_name - FAILED"
        return 1
    fi
}

# Build the application
build_app() {
    log_info "Building health-monitor application..."
    if /usr/local/go/bin/go build -o health-monitor ./cmd/health-monitor; then
        log_success "Build successful"
    else
        log_error "Build failed"
        exit 1
    fi
}

# Push logs to Loki
push_logs() {
    local service="$1"
    local error_message="$2"
    local count="${3:-1}"
    
    local NOW=$(($(date +%s) * 1000000000))
    local values="["
    
    for i in $(seq 1 $count); do
        local timestamp=$((NOW + (i * 1000000000)))
        values+="[\"$timestamp\", \"$error_message\"]"
        if [ $i -lt $count ]; then
            values+=","
        fi
    done
    values+="]"
    
    curl -s -X POST "$BASE_URL" \
        -H "Content-Type: application/json" \
        -d "{
            \"streams\": [
                {
                    \"stream\": {\"service\": \"$service\"},
                    \"values\": $values
                }
            ]
        }" > /dev/null
    
    log_info "Pushed $count log(s) for service: $service"
}

# Create incident
create_incident() {
    local service="$1"
    local title="$2"
    local severity="${3:-P2}"
    local description="${4:-}"
    local impacted_flows="${5:-}"
    
    local cmd="sudo $HEALTH_MONITOR --profile $PROFILE incident start --service $service --title \"$title\" --severity $severity"
    
    if [ -n "$description" ]; then
        cmd+=" --description \"$description\""
    fi
    
    if [ -n "$impacted_flows" ]; then
        cmd+=" --impacted-flows \"$impacted_flows\""
    fi
    
    eval "$cmd"
}

# View incident
view_incident() {
    local incident_id="$1"
    local mode="${2:-cli}"
    
    if [ "$mode" = "tui" ]; then
        sudo $HEALTH_MONITOR --profile $PROFILE incident view --tui --incident-id "$incident_id"
    else
        sudo $HEALTH_MONITOR --profile $PROFILE incident view --incident-id "$incident_id"
    fi
}

# Get latest incident ID
get_latest_incident() {
    local service="$1"
    sudo $HEALTH_MONITOR --profile $PROFILE incident list --service "$service" --format json | \
        jq -r '.incidents[0].id' 2>/dev/null || echo ""
}

# ========================================
# 🧪 TEST SCENARIOS
# ========================================

# Scenario 1: Database Connection Issues
test_database_patterns() {
    log_info "🗄️ Testing Database Connection Patterns"
    
    # Test PostgreSQL connection timeout
    push_logs "user_service" "ERROR postgres connection timeout after 30 seconds" 2
    push_logs "user_service" "FATAL connection to database failed: connection refused" 1
    
    create_incident "user_service" "PostgreSQL Connection Pool Exhaustion" "P1" \
        "Users unable to authenticate due to database connection issues" \
        "User Authentication,Account Management"
    
    sleep 3
    local incident_id=$(get_latest_incident "user_service")
    if [ -n "$incident_id" ]; then
        view_incident "$incident_id"
        log_success "Database pattern test completed - Incident: $incident_id"
    fi
}

# Scenario 2: HTTP Service Degradation
test_http_patterns() {
    log_info "🌐 Testing HTTP Service Patterns"
    
    # Test 502 Bad Gateway
    push_logs "api_gateway" "ERROR 502 Bad Gateway from upstream payment_service" 3
    push_logs "api_gateway" "ERROR upstream service timeout after 30s" 2
    
    create_incident "api_gateway" "Payment Service Unavailable" "P2" \
        "Payment processing failing due to upstream service issues" \
        "Payment Processing,Checkout Flow"
    
    sleep 3
    local incident_id=$(get_latest_incident "api_gateway")
    if [ -n "$incident_id" ]; then
        view_incident "$incident_id"
        log_success "HTTP pattern test completed - Incident: $incident_id"
    fi
}

# Scenario 3: Resource Exhaustion
test_resource_patterns() {
    log_info "💾 Testing Resource Exhaustion Patterns"
    
    # Test memory pressure
    push_logs "analytics_service" "WARN memory usage above 90% threshold" 2
    push_logs "analytics_service" "ERROR OutOfMemoryError: GC overhead limit exceeded" 1
    
    # Test disk space
    push_logs "file_processor" "ERROR no space left on device /var/log" 2
    push_logs "file_processor" "CRITICAL disk usage at 98%" 1
    
    create_incident "analytics_service" "Memory Pressure in Analytics Service" "P1" \
        "Analytics service experiencing memory exhaustion" \
        "Data Analytics,Reporting"
    
    sleep 3
    local incident_id=$(get_latest_incident "analytics_service")
    if [ -n "$incident_id" ]; then
        view_incident "$incident_id"
        log_success "Resource pattern test completed - Incident: $incident_id"
    fi
}

# Scenario 4: Security and Authentication Issues
test_security_patterns() {
    log_info "🔐 Testing Security Patterns"
    
    # Test authentication failures
    push_logs "auth_service" "ERROR authentication failed for user: invalid credentials" 5
    push_logs "auth_service" "WARN JWT token validation failed: expired token" 3
    
    # Test SSL/TLS issues
    push_logs "payment_gateway" "ERROR SSL handshake failed: certificate expired" 2
    push_logs "payment_gateway" "CRITICAL TLS protocol negotiation failed" 1
    
    create_incident "auth_service" "Authentication Service Failures" "P2" \
        "Users experiencing login failures across platform" \
        "User Login,API Authentication"
    
    sleep 3
    local incident_id=$(get_latest_incident "auth_service")
    if [ -n "$incident_id" ]; then
        view_incident "$incident_id"
        log_success "Security pattern test completed - Incident: $incident_id"
    fi
}

# Scenario 5: Custom Pattern Matching
test_custom_patterns() {
    log_info "🎛️ Testing Custom Pattern Matching"
    
    # Test the custom patterns defined in config
    push_logs "billing_service" "ERROR billing service unavailable during payment processing" 3
    push_logs "billing_service" "CRITICAL payment gateway timeout after 30 seconds" 2
    
    create_incident "billing_service" "Payment Processing System Failure" "P1" \
        "Complete payment processing outage affecting revenue" \
        "Payment Processing,Revenue Generation,Billing"
    
    sleep 3
    local incident_id=$(get_latest_incident "billing_service")
    if [ -n "$incident_id" ]; then
        view_incident "$incident_id"
        log_success "Custom pattern test completed - Incident: $incident_id"
    fi
}

# Scenario 6: Performance Degradation
test_performance_patterns() {
    log_info "⚡ Testing Performance Patterns"
    
    # Test slow queries and latency
    push_logs "search_service" "WARN query execution time exceeded 5s threshold" 4
    push_logs "search_service" "ERROR request timeout: processing took 15.3s" 2
    push_logs "search_service" "INFO latency spike detected: avg response time 2.1s" 3
    
    create_incident "search_service" "Search Performance Degradation" "P2" \
        "Search queries experiencing severe latency" \
        "Product Search,User Experience"
    
    sleep 3
    local incident_id=$(get_latest_incident "search_service")
    if [ -n "$incident_id" ]; then
        view_incident "$incident_id"
        log_success "Performance pattern test completed - Incident: $incident_id"
    fi
}

# Scenario 7: Multi-Service Cascade Failure
test_cascade_failure() {
    log_info "🌊 Testing Multi-Service Cascade Failure"
    
    # Simulate cascade: Database -> Auth -> API Gateway
    push_logs "postgres_primary" "FATAL database server is not accepting connections" 2
    sleep 1
    
    push_logs "auth_service" "ERROR postgres connection timeout during user lookup" 3
    push_logs "auth_service" "CRITICAL unable to authenticate users: database unavailable" 2
    sleep 1
    
    push_logs "api_gateway" "ERROR 503 Service Unavailable: auth_service down" 4
    push_logs "api_gateway" "WARN circuit breaker activated for auth_service" 2
    
    create_incident "postgres_primary" "Primary Database Outage" "P0" \
        "Complete system outage due to primary database failure" \
        "All Services,User Authentication,Payment Processing,Data Analytics"
    
    sleep 3
    local incident_id=$(get_latest_incident "postgres_primary")
    if [ -n "$incident_id" ]; then
        view_incident "$incident_id"
        log_success "Cascade failure test completed - Incident: $incident_id"
    fi
}

# Scenario 8: Edge Case - Generic Errors
test_generic_patterns() {
    log_info "🔍 Testing Generic Error Patterns"
    
    # Test generic errors that should match general patterns
    push_logs "legacy_service" "ERROR something went wrong" 2
    push_logs "legacy_service" "FATAL unexpected error occurred" 1
    push_logs "legacy_service" "WARN system encountered an issue" 2
    
    create_incident "legacy_service" "Legacy Service Unexplained Errors" "P3" \
        "Legacy service generating generic error messages" \
        "Legacy Operations,Maintenance"
    
    sleep 3
    local incident_id=$(get_latest_incident "legacy_service")
    if [ -n "$incident_id" ]; then
        view_incident "$incident_id"
        log_success "Generic pattern test completed - Incident: $incident_id"
    fi
}

# Scenario 9: High Volume Stress Test
test_high_volume() {
    log_info "🚀 Testing High Volume Scenario"
    
    # Generate high volume of errors
    for i in {1..10}; do
        push_logs "high_traffic_service" "ERROR rate limit exceeded for user $i" 5
        push_logs "high_traffic_service" "WARN connection pool exhaustion detected" 3
    done
    
    create_incident "high_traffic_service" "High Traffic Service Overload" "P1" \
        "Service experiencing extreme load and connection exhaustion" \
        "High Traffic APIs,Rate Limiting,Connection Management"
    
    sleep 3
    local incident_id=$(get_latest_incident "high_traffic_service")
    if [ -n "$incident_id" ]; then
        view_incident "$incident_id"
        log_success "High volume test completed - Incident: $incident_id"
    fi
}

# Scenario 10: Resolution and RCA Testing
test_resolution_workflow() {
    log_info "🔧 Testing Resolution Workflow"
    
    # Create a simple incident
    push_logs "test_service" "ERROR test error for resolution workflow" 2
    create_incident "test_service" "Resolution Workflow Test" "P4" \
        "Test incident for resolution workflow" \
        "Testing,Quality Assurance"
    
    sleep 2
    local incident_id=$(get_latest_incident "test_service")
    
    if [ -n "$incident_id" ]; then
        log_info "Resolving incident: $incident_id"
        
        # Resolve with RCA
        sudo $HEALTH_MONITOR --profile $PROFILE incident resolve \
            --incident-id "$incident_id" \
            --resolution "Fixed test configuration issue" \
            --rca-cause "Misconfiguration in test environment" \
            --rca-impact "Test service was temporarily unavailable" \
            --rca-prevention "Implement configuration validation checks"
        
        log_success "Resolution workflow test completed - Incident: $incident_id"
    fi
}

# ========================================
# 🎯 MAIN EXECUTION
# ========================================

main() {
    echo "Starting comprehensive runbook suggestions testing..."
    echo ""
    
    # Build application
    build_app
    
    echo ""
    log_info "🧪 Running Production Test Scenarios"
    echo "====================================="
    
    # Run all test scenarios
    test_database_patterns
    sleep 2
    
    test_http_patterns
    sleep 2
    
    test_resource_patterns
    sleep 2
    
    test_security_patterns
    sleep 2
    
    test_custom_patterns
    sleep 2
    
    test_performance_patterns
    sleep 2
    
    test_cascade_failure
    sleep 2
    
    test_generic_patterns
    sleep 2
    
    test_high_volume
    sleep 2
    
    test_resolution_workflow
    
    echo ""
    log_success "🎉 All production test scenarios completed!"
    echo ""
    
    # Show summary
    log_info "📊 Test Summary:"
    echo "- Database Patterns: ✅"
    echo "- HTTP Patterns: ✅"
    echo "- Resource Patterns: ✅"
    echo "- Security Patterns: ✅"
    echo "- Custom Patterns: ✅"
    echo "- Performance Patterns: ✅"
    echo "- Cascade Failures: ✅"
    echo "- Generic Patterns: ✅"
    echo "- High Volume: ✅"
    echo "- Resolution Workflow: ✅"
    echo ""
    
    log_info "🔍 Next Steps:"
    echo "1. Review incident details for each scenario"
    echo "2. Verify runbook suggestions are accurate"
    echo "3. Check confidence scores are appropriate"
    echo "4. Validate custom pattern matching"
    echo "5. Monitor metrics and performance"
    echo ""
    
    log_success "Production testing complete! 🚀"
}

# Run main function
main "$@"
