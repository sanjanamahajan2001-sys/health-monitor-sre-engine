# Alert Command Debug - Systemd Issue Identified

## 🚨 Problem Analysis

The `alert listen --background` command is failing because:

1. **Systemd Scope Creation**: Code tries to create systemd scope first
2. **Silent Failure**: If systemd fails, it should fallback to background process
3. **Permission Issue**: Systemd scope creation might be failing silently
4. **No Fallback**: Background process fallback might not be working

## 🔍 Debug Steps

### Step 1: Test Without Background
```bash
# This should show the actual error
./health-monitor alert listen --addr :9095 --data-dir /var/lib/health-monitor
```

### Step 2: Check Systemd Issues
```bash
# Check if systemd is available
systemctl --version

# Check if user can create scopes
systemctl --user status
```

### Step 3: Force Background Process
```bash
# Disable systemd by setting environment variable
HEALTH_MONITOR_NO_SYSTEMD=1 ./health-monitor alert listen --background --addr :9095 --data-dir /var/lib/health-monitor
```

## 🛠️ Quick Fix

The issue is likely in the systemd scope creation logic. Try:

```bash
# Option 1: Run without systemd
HEALTH_MONITOR_NO_SYSTEMD=1 ./health-monitor alert listen --background --addr :9095 --data-dir /var/lib/health-monitor

# Option 2: Use local directory
mkdir -p ./data/incidents
./health-monitor alert listen --background --addr :9095 --data-dir ./data

# Option 3: Run in foreground to see error
./health-monitor alert listen --addr :9095 --data-dir /var/lib/health-monitor
```

## 🎯 Root Cause

The `startBackground` function (line 417) tries systemd first, then falls back. If systemd scope creation fails silently, the fallback might not execute properly.

## 🚀 Recommended Action

Try Option 1 first to disable systemd and force background process:

```bash
HEALTH_MONITOR_NO_SYSTEMD=1 ./health-monitor alert listen --background --addr :9095 --data-dir /var/lib/health-monitor
```

This should show: `Listener running in background (PID xxxx)`
