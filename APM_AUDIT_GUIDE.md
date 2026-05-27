# Mini-APM Audit Guide

This guide documents Mini‑APM behavior and provides a repeatable audit checklist
with exact commands to validate functionality across different metric families.

## What Mini‑APM Does

- Auto‑detects dependency latency metrics (HTTP client, gRPC, Istio)
- Auto‑detects source/destination/route labels
- Builds a dependency graph (edges with P95, delta, RPS)
- Ranks likely root causes and reports confidence + evidence
- Emits reasoning, next checks, and possible fixes
- Guards against low traffic and high cardinality

## Key Outputs (TUI + Export)

- **Suspected Upstream Cause**
- **Correlation Confidence** (low/medium/high)
- **APM Root Cause Ranking**
- **Why We Think This Is The Cause**
- **What To Check Next**
- **Possible Fixes**
- **APM Data Gaps**
- **APM Auto‑Detect Reasoning**
- **Top Dependency Edges (P95)**

## Configuration Reference (Env / Config)

Core:
- `APM_DEPENDENCY_METRIC` (optional override)
- `APM_SOURCE_LABEL`, `APM_DEST_LABEL`, `APM_ROUTE_LABEL`
- `APM_WINDOW` (default: 10m)
- `APM_TOP_EDGES` (default: 5)
- `APM_CARDINALITY_LIMIT` (default: 500)
- `APM_MIN_RPS` (default: 0.1)

Alignment with API Latency:
- `API_SERVICE_LABEL` affects Loki hints and service filtering
- If you want APM to align, set `APM_SOURCE_LABEL` / `APM_DEST_LABEL`

## Audit Checklist (CLI)

### 1) HTTP client metrics (OpenTelemetry style)

```bash
export APM_DEPENDENCY_METRIC=http_client_request_duration_seconds
sudo ./health-monitor
```

Expected:
- Auto‑detect reasoning shows `http_client_request_duration_seconds`
- Top edges like `api_service -> payment_gateway`
- Root cause ranking populated

### 2) gRPC client metrics (if present)

```bash
export APM_DEPENDENCY_METRIC=rpc_client_duration_seconds
sudo ./health-monitor
```

Expected:
- Auto‑detect reasoning includes `(grpc)` in metric note
- Labels such as `rpc_service` or `rpc_method` show up

### 3) Istio metrics

```bash
export APM_DEPENDENCY_METRIC=istio_request_duration_seconds
sudo ./health-monitor
```

Expected:
- Labels like `source_workload`, `destination_service`
- Root cause often points to the slowest destination

### 4) Low‑traffic guardrail

```bash
export APM_MIN_RPS=10
sudo ./health-monitor
```

Expected:
- No root‑cause inference
- APM Data Gaps: “insufficient traffic for root‑cause inference”

### 5) Invalid label override fallback

```bash
export APM_SOURCE_LABEL=does_not_exist
export APM_DEST_LABEL=does_not_exist
sudo ./health-monitor
```

Expected:
- Auto‑detect reasoning shows label override missing + fallback

## Optional Local Demo (Mock Metrics)

Use the mock exporters from the verification steps:
- `istio_request_duration_seconds` mock exporter
- `http_client_request_duration_seconds` mock exporter

Run Prometheus, add scrape targets, then run:

```bash
sudo ./health-monitor
```

## Audit Pass/Fail Criteria

Pass if all are true:
- Root cause appears when traffic is sufficient
- “No inference” appears when traffic is too low
- Auto‑detect picks reasonable labels
- Ranking order matches top edges in output
- Data gaps explain missing labels/metrics

Fail if any are true:
- Root cause appears under low traffic without warning
- Labels are missing but no data gap is reported
- Ranking does not match edges shown

