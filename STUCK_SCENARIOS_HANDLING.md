# Handling Stuck Scenarios - Update Mechanism

This document explains how the health-monitor handles stuck processes, timeouts, and failure scenarios during updates.

## Timeout Protection Mechanisms

### Built-in Timeouts

The update mechanism has **multiple layers of timeout protection**:

1. **Version Check Timeout**: 5 seconds
   - Maximum time to fetch `version.txt`
   - If exceeded, update check fails gracefully

2. **Binary Download Timeout**: 30 seconds
   - Maximum time to download the binary file
   - If exceeded, download is cancelled

3. **Overall Timeout**: 40 seconds total
   - Combined timeout for entire update process
   - Ensures update never blocks execution indefinitely

4. **Context Cancellation**: All HTTP requests use context with timeout
   - Automatically cancels requests when timeout expires
   - Prevents hanging connections

## Disable Update Checks (Production Hardening)

If your environment has restricted outbound access or you want to suppress
update warnings, you can disable update checks entirely:

```
HEALTH_MONITOR_DISABLE_UPDATES=1 health-monitor
```

When disabled, the agent skips update checks and proceeds directly to health
checks.

## What Happens If Update Gets Stuck?

### Scenario 1: Network Hangs (Connection Stuck)

**What happens:**
- HTTP request hangs waiting for response
- After 5 seconds (version check) or 30 seconds (download), timeout triggers
- Context is cancelled, connection is closed
- Update process returns error
- **Health checks continue normally**

**Code protection:**
```go
ctx, cancel := context.WithTimeout(context.Background(), UpdateCheckTimeout)
defer cancel()  // Ensures context is cancelled even if function returns early

client := &http.Client{
    Timeout: UpdateCheckTimeout,  // Double protection
}
```

### Scenario 2: Slow Download (Large File)

**What happens:**
- Download starts but is very slow
- After 30 seconds, timeout triggers
- Download is cancelled
- Temporary file is cleaned up
- Update fails, but **health checks continue**

**Protection:**
- `UpdateDownloadTimeout = 30 * time.Second`
- Context cancellation stops the download
- Temp file is removed on error

### Scenario 3: Update Process Hangs Entirely

**What happens:**
- Update runs in a goroutine (separate thread)
- Main process has an overall timeout of 40 seconds
- If update doesn't complete in 40 seconds, timeout triggers
- Main process continues with health checks
- **Application never gets stuck**

**Code protection:**
```go
select {
case result := <-resultChan:
    return result, nil
case err := <-errChan:
    return nil, err
case <-time.After(UpdateCheckTimeout + UpdateDownloadTimeout + 5*time.Second):
    // Overall timeout - update check failed silently
    return &UpdateResult{...}, nil
}
```

### Scenario 4: Binary Verification Hangs

**What happens:**
- Binary is downloaded successfully
- Verification runs `./binary --version`
- If `--version` hangs (unlikely but possible)
- **Current implementation**: No timeout on verification
- **Risk**: Could potentially hang (very rare)

**Mitigation needed:**
- Should add timeout to binary verification
- Can be improved in future version

### Scenario 5: File System Operations Hang

**What happens:**
- File operations (copy, rename, chmod) could theoretically hang
- **Risk**: Very low, but possible on network filesystems
- **Current protection**: None (relies on OS)

**Mitigation:**
- File operations are fast (usually < 1 second)
- If they hang, it's an OS/filesystem issue
- Health checks would also be affected in this case

## Safety Guarantees

### ✅ What CANNOT Break

1. **Health checks always run**
   - Update runs in goroutine with timeout
   - Main process continues regardless of update status
   - Health checks are completely independent

2. **Application never hangs indefinitely**
   - Maximum 40 seconds for update attempt
   - After timeout, execution continues
   - No blocking operations without timeouts

3. **Current binary always works**
   - Update failures don't affect current binary
   - Binary replacement is atomic (all-or-nothing)
   - Backup is created before replacement

### ⚠️ What COULD Happen (Rare Scenarios)

1. **Binary verification hangs** (very rare)
   - If `--version` command hangs, update could hang
   - **Fix needed**: Add timeout to verification

2. **File system operations hang** (extremely rare)
   - Network filesystem issues
   - Disk I/O problems
   - Would affect entire system, not just update

3. **Goroutine leak** (theoretical)
   - If goroutine doesn't return, it stays in memory
   - **Current protection**: Timeout ensures goroutine completes or is abandoned

## Current Implementation Details

### Timeout Values

```go
UpdateCheckTimeout = 5 * time.Second      // Version check
UpdateDownloadTimeout = 30 * time.Second  // Binary download
OverallTimeout = 40 seconds                // Total maximum wait
```

### Error Handling Flow

```
Update Check Starts
    ↓
[5s timeout] → Version Check
    ↓ (if timeout)
Error logged → Continue to health checks

    ↓ (if success)
[30s timeout] → Binary Download
    ↓ (if timeout)
Error logged → Continue to health checks

    ↓ (if success)
Binary Verification
    ↓ (if fails)
Error logged → Continue to health checks

    ↓ (if success)
Atomic Replacement
    ↓ (if fails)
Backup restored → Error logged → Continue to health checks

    ↓ (if success)
Update Complete → Show message
    ↓
Continue to health checks
```

## Improvements Needed

### 1. Add Timeout to Binary Verification

**Current:**
```go
cmd := exec.Command(path, "--version")
if err := cmd.Run(); err != nil {
    return fmt.Errorf("binary verification failed: %w", err)
}
```

**Should be:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

cmd := exec.CommandContext(ctx, path, "--version")
if err := cmd.Run(); err != nil {
    return fmt.Errorf("binary verification failed: %w", err)
}
```

### 2. Add Timeout to File Operations (Optional)

For network filesystems, could add timeouts to file operations, but this is usually unnecessary as they're very fast.

## Testing Stuck Scenarios

### Test 1: Network Timeout

```bash
# Block network temporarily
sudo iptables -A OUTPUT -d storage-domain.com -j DROP

# Run health-monitor
health-monitor
# Should timeout after 5-40 seconds and continue
```

### Test 2: Slow Download

```bash
# Use slow network or throttle
# Update should timeout after 30 seconds
```

### Test 3: Invalid Binary

```bash
# Upload corrupted binary
# Verification should fail
# Update should be rejected
```

## Summary

**Current Protection:**
- ✅ Network timeouts (5s version, 30s download)
- ✅ Overall timeout (40s total)
- ✅ Context cancellation
- ✅ Goroutine isolation
- ✅ Error handling and logging
- ✅ Health checks always continue

**Potential Improvements:**
- ⚠️ Add timeout to binary verification (recommended)
- ⚠️ Add timeout to file operations (optional, low priority)

**Bottom Line:**
The update mechanism is designed to **never block or hang the application**. Even in worst-case scenarios, health checks continue normally after a maximum of 40 seconds. The agent is resilient and will continue functioning even if updates fail completely.
