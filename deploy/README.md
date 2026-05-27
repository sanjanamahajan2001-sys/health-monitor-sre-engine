# Production Deployment Guide

## Systemd Service (Production)

### Installation

1. **Build and install binary:**
```bash
go build -o health-monitor ./cmd/health-monitor
sudo mkdir -p /opt/health-monitor
sudo cp health-monitor /opt/health-monitor/
sudo chmod +x /opt/health-monitor/health-monitor
```

2. **Create directories:**
```bash
sudo mkdir -p /var/lib/health-monitor /var/log/health-monitor /etc/health-monitor
sudo chown -R health-monitor:health-monitor /var/lib/health-monitor /var/log/health-monitor
sudo chmod 755 /var/lib/health-monitor /var/log/health-monitor
```

3. **Install systemd service:**
```bash
sudo cp deploy/systemd/health-monitor.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable health-monitor
sudo systemctl start health-monitor
```

4. **Check status:**
```bash
sudo systemctl status health-monitor
sudo journalctl -u health-monitor -f
```

## Docker Compose (Production)

### Quick Start
```bash
# Build and run
docker-compose up -d

# View logs
docker-compose logs -f health-monitor

# Stop
docker-compose down
```

### Configuration
```bash
# Create config directory
mkdir -p config

# Copy example config
cp /etc/health-monitor/health-monitor.json config/health-monitor.json

# Edit configuration
vim config/health-monitor.json
```

### Health Checks
- **Readiness**: `http://localhost:8080/healthz`
- **Metrics**: `http://localhost:9090/metrics`

## Production Configuration

### Required Settings
```json
{
  "prometheus_url": "https://prometheus.example.com",
  "grafana_url": "https://grafana.example.com",
  "grafana_prom_ds": "Prometheus",
  "grafana_loki_ds": "Loki",
  "grafana_trace_ds": "Tempo"
}
```

### Data Directory
- **Canonical**: `/var/lib/health-monitor/incidents`
- **Writable**: Service will fail if not writable
- **Migration**: Auto-migrates from `~/.health-monitor`

### Security
- **Non-root**: Runs as `health-monitor` user
- **Minimal privileges**: Drops all capabilities
- **File permissions**: Proper ownership and modes

## Monitoring

### Logs
```bash
# Systemd
sudo journalctl -u health-monitor -f

# Docker
docker-compose logs -f health-monitor
```

### Metrics
```bash
curl http://localhost:9090/metrics
```

### Health
```bash
curl http://localhost:8080/healthz
```

## Production Best Practices

### ✅ Do
- Use canonical data directory
- Run as non-root user
- Enable systemd service
- Configure log rotation
- Monitor health endpoints
- Set resource limits

### ❌ Don't
- Run as root
- Use home directories
- Skip health checks
- Ignore log rotation
- Run without resource limits
