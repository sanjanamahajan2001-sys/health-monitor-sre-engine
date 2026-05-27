#!/bin/bash

echo "🔍 Debugging Loki Query Issues"
echo "================================"

# Test basic Loki connection
echo "1. Testing Loki connection..."
curl -s http://localhost:3100/ready || echo "❌ Loki not ready"

echo ""
echo "2. Testing recent logs for billing_api..."

# Test basic query for billing_api logs
NOW=$(date +%s)
START=$((NOW - 300)) # 5 minutes ago

echo "Querying Loki from $START to $NOW..."

# Basic query without error filtering
curl -s -G "http://localhost:3100/loki/api/v1/query_range" \
  --data-urlencode "query={service=\"billing_api\"}" \
  --data-urlencode "start=${START}000000000" \
  --data-urlencode "end=${NOW}000000000" \
  | jq '.data.result | length' || echo "❌ Basic query failed"

echo ""
echo "3. Testing error pattern query..."

# Query with error pattern (what the system uses)
curl -s -G "http://localhost:3100/loki/api/v1/query_range" \
  --data-urlencode 'query={service="billing_api"} |=~ "ERROR|Exception|timeout|5[0-9][0-9]"' \
  --data-urlencode "start=${START}000000000" \
  --data-urlencode "end=${NOW}000000000" \
  | jq '.data.result | length' || echo "❌ Error pattern query failed"

echo ""
echo "4. Testing specific ERROR query..."

# Query for specific ERROR messages
curl -s -G "http://localhost:3100/loki/api/v1/query_range" \
  --data-urlencode 'query={service="billing_api"} |=~ "ERROR"' \
  --data-urlencode "start=${START}000000000" \
  --data-urlencode "end=${NOW}000000000" \
  | jq '.data.result | length' || echo "❌ ERROR query failed"

echo ""
echo "5. Pushing fresh test logs..."

# Push fresh test logs
TEST_NOW=$(($(date +%s) * 1000000000))
curl -X POST http://localhost:3100/loki/api/v1/push \
  -H "Content-Type: application/json" \
  -d '{
    "streams": [
      {
        "stream": {"service": "billing_api"},
        "values": [
          ["'$TEST_NOW'", "ERROR postgres connection timeout during checkout"],
          ["'$((TEST_NOW + 1000000000))'", "ERROR transaction rollback due to deadlock"]
        ]
      }
    ]
  }' && echo "✅ Test logs pushed"

echo ""
echo "6. Testing again after push..."

sleep 2

# Query again after pushing
curl -s -G "http://localhost:3100/loki/api/v1/query_range" \
  --data-urlencode 'query={service="billing_api"} |=~ "ERROR"' \
  --data-urlencode "start=${START}000000000" \
  --data-urlencode "end=${NOW}000000000" \
  | jq '.data.result | length' || echo "❌ Still failing"

echo ""
echo "7. Show recent logs format..."
curl -s -G "http://localhost:3100/loki/api/v1/query_range" \
  --data-urlencode "query={service=\"billing_api\"}" \
  --data-urlencode "start=${START}000000000" \
  --data-urlencode "end=${NOW}000000000" \
  | jq '.data.result[0].values' | head -10 || echo "❌ Could not fetch logs"
