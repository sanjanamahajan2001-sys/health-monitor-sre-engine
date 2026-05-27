## API Latency Setup (Prometheus)

This agent can pull P90/P95/P99 latency, error rate, and RPS from Prometheus.
For production, use system-wide config and a token file.

## Recommended Production Setup

1. Create system config:

`/etc/health-monitor/config.json`

Quick init (recommended):
```
sudo health-monitor --init
```

Example:
```
{
  "prometheus_url": "https://prom.example.com",
  "api_service": "payment",
  "api_route": "/charge",
  "service_label": "service",
  "route_label": "route",
  "latency_metric": "http_request_duration_seconds",
  "request_count_metric": "http_requests_total",
  "error_label": "status",
  "error_regex": "5..",
  "latency_threshold_seconds": 10,
  "window": "5m",
  "extra_label_selectors": "env=\"prod\""
}
```

2. Create a token file (recommended):

`/etc/health-monitor/prometheus.token`

If running as a non-root user, you can also place it at:

`~/.health-monitor/prometheus.token`

Contents: the read-only Prometheus token (single line).

Permissions:
```
sudo chown root:root /etc/health-monitor/prometheus.token
sudo chmod 600 /etc/health-monitor/prometheus.token
```

3. Run the agent:
```
sudo health-monitor
```

## Auth-Protected Prometheus (How To Test)

1. Configure auth:

- Bearer token file (recommended):
  - `/etc/health-monitor/prometheus.token` (root) or `~/.health-monitor/prometheus.token` (user)
- Basic auth file (user:pass):
  - `/etc/health-monitor/prometheus.basic` (root) or `~/.health-monitor/prometheus.basic` (user)
- Or env vars:
  - `PROMETHEUS_TOKEN`
  - `PROMETHEUS_USER` / `PROMETHEUS_PASS`

2. Test connectivity:
```
health-monitor --test-prom
```

If auth fails, the command returns a non-zero exit code.

## Loki Logs (Phase 2)

1. Configure Loki URL:

- Set in config: `"loki_url": "https://loki.example.com"`
- Or env: `LOKI_URL`

2. Auth (optional):

- Token file: `/etc/health-monitor/loki.token` (or `~/.health-monitor/loki.token`)
- Basic auth file: `/etc/health-monitor/loki.basic` (or `~/.health-monitor/loki.basic`)
- Env: `LOKI_TOKEN`, `LOKI_USER`, `LOKI_PASS`

3. Labels (auto-detected by default):

- Optional overrides: `LOKI_SERVICE_LABEL`, `LOKI_ROUTE_LABEL`
- Error filter: `LOKI_ERROR_REGEX` (default: `(?i)(error|exception|5\\d\\d)`)
- Window: `LOKI_WINDOW` (default: `10m`)

4. TUI:

- Press `o` for Loki config, `l` for logs view

## Grafana Link-Outs (Optional)

If you use Grafana Explore, you can configure link-outs so the agent shows
clickable URLs for Prometheus and Loki queries.

Set:

- `GRAFANA_URL`
- `GRAFANA_PROM_DS` (Prometheus datasource name)
- `GRAFANA_LOKI_DS` (Loki datasource name)

In TUI:

- Press `v` to configure Grafana

## Disable Update Checks (Optional)

If your environment blocks outbound access, you can disable update checks:

```
HEALTH_MONITOR_DISABLE_UPDATES=1 health-monitor
```

## URL-Only Auto Mode (Minimal Setup)

If you only want to provide the Prometheus URL and let the agent detect
metrics and labels automatically:

```
export PROMETHEUS_URL="http://localhost:9090"
sudo health-monitor
```

This enables auto-discovery by default when no config file exists.

## Environment Variables (Alternative)

You can provide settings via environment variables instead of the config file:

- `PROMETHEUS_URL`
- `PROMETHEUS_TOKEN` (not recommended for production)
- `PROMETHEUS_TOKEN_FILE`
- `PROMETHEUS_USER`, `PROMETHEUS_PASS`
- `API_SERVICE`, `API_ROUTE`
- `API_SERVICE_LABEL`, `API_ROUTE_LABEL`
- `API_LATENCY_METRIC`, `API_REQUESTS_METRIC`
- `API_AUTO_DISCOVER` (set to `true` to auto-detect metrics/labels)
- `API_ERROR_LABEL`, `API_ERROR_REGEX`
- `API_LATENCY_THRESHOLD`, `PROMETHEUS_WINDOW`
- `PROMETHEUS_EXTRA_LABELS`

## Notes

- The in-app editor (`e`) only edits non-secret fields.
- Secrets should be provided via token file or env vars.
- If `/etc/health-monitor/config.json` is missing, the agent falls back to:
  `~/.health-monitor/config.json`.
