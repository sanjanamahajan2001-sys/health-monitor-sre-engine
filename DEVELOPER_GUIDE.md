# Health-Monitor Developer Guide

Complete guide for developers working on the Health-Monitor project.

## Table of Contents

1. [Project Structure](#project-structure)
2. [Building](#building)
3. [Architecture](#architecture)
4. [Adding New Checks](#adding-new-checks)
5. [Auto-Update Implementation](#auto-update-implementation)
6. [Testing](#testing)
7. [Release Process](#release-process)

---

## Project Structure

```
health-monitor/
├── cmd/
│   └── health-monitor/
│       └── main.go              # Entry point
├── internal/
│   ├── analyse/                 # Data collection
│   │   ├── disk/
│   │   ├── memory/
│   │   ├── gpu/
│   │   ├── system/              # CPU, uptime, load
│   │   └── ...
│   ├── checks/                  # Check orchestration
│   ├── rules/                   # Status evaluation
│   ├── output/                  # TUI rendering
│   └── runner/                  # Command execution
├── pkg/
│   ├── model/                   # Data models
│   │   ├── result.go            # Report structures
│   │   ├── version.go          # Version info
│   │   └── version_fetcher.go  # Version fetching
│   └── update/                  # Auto-update logic
│       └── updater.go
└── go.mod                       # Dependencies
```

### Key Components

- **`cmd/health-monitor/main.go`**: Application entry point, orchestrates checks
- **`internal/analyse/`**: Collects raw system data via shell commands
- **`internal/checks/`**: Orchestrates data collection and populates report
- **`internal/rules/`**: Evaluates data and determines status (SAFE/CHECK/RISK)
- **`internal/output/`**: Renders interactive TUI using bubbletea
- **`pkg/update/`**: Handles automatic updates

---

## Building

### Prerequisites

- Go 1.23+ installed
- Linux environment (for Linux builds)
- Git (for commit hash)

### Build Commands

**Development build:**
```bash
go build ./cmd/health-monitor
```

**Production build with version:**
```bash
VERSION=v0.3
COMMIT=$(git rev-parse --short HEAD)
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
go build -o health-monitor-linux-amd64-$VERSION \
  -ldflags "-X health-monitor/pkg/model.Version=$VERSION \
             -X health-monitor/pkg/model.Commit=$COMMIT \
             -X health-monitor/pkg/model.BuildTime=$DATE" \
  ./cmd/health-monitor

chmod +x health-monitor-linux-amd64-$VERSION
```

### Build Flags

- `-X health-monitor/pkg/model.Version`: Version number (e.g., v0.3)
- `-X health-monitor/pkg/model.Commit`: Git commit hash
- `-X health-monitor/pkg/model.BuildTime`: Build timestamp

### Cross-Compilation

```bash
# Linux AMD64
GOOS=linux GOARCH=amd64 go build ...

# Linux ARM64
GOOS=linux GOARCH=arm64 go build ...

# macOS
GOOS=darwin GOARCH=amd64 go build ...
```

---

## Architecture

### Data Flow

```
main.go
  ↓
checks.Disk() → analyse/disk → rules.DiskStatus() → Report
checks.Memory() → analyse/memory → rules.MemoryStatus() → Report
checks.System() → analyse/system → (no rules) → Report
  ↓
output.Print() → interactive_tui.go → bubbletea → TUI
```

### Check Pattern

Each check follows this pattern:

1. **Collect data** (`internal/analyse/{check}/`)
   - Run shell commands
   - Parse output
   - Return structured data

2. **Evaluate status** (`internal/rules/{check}.go`)
   - Compare against thresholds
   - Return SAFE/CHECK/RISK status

3. **Populate report** (`internal/checks/{check}.go`)
   - Add to Summary (if needed)
   - Add to Metrics
   - Handle errors gracefully

### Example: Adding a New Check

**1. Create data collection (`internal/analyse/network/network.go`):**
```go
package network

import "health-monitor/internal/runner"

type NetworkData struct {
    Interfaces int
    ActiveConnections int
}

func Collect() (NetworkData, error) {
    // Collect network data
    out, err := runner.Exec("ip -o link show | wc -l")
    // ... parse and return
}
```

**2. Create rules (`internal/rules/network.go`):**
```go
package rules

func NetworkStatus(data network.NetworkData) model.Status {
    // Evaluate and return SAFE/CHECK/RISK
}
```

**3. Create check (`internal/checks/network.go`):**
```go
package checks

func Network(report *model.Report) {
    data, err := network.Collect()
    if err != nil {
        return // Fail silently
    }
    
    status := rules.NetworkStatus(data)
    report.Summary = append(report.Summary, model.SummaryItem{
        Name: "NETWORK",
        Status: status,
        Value: fmt.Sprintf("%d interfaces", data.Interfaces),
    })
    
    report.Metrics = append(report.Metrics,
        model.Metric{"Network Interfaces", fmt.Sprintf("%d", data.Interfaces)},
    )
}
```

**4. Add to main.go:**
```go
checks.Network(&report)
```

---

## Auto-Update Implementation

### How It Works

1. **Version Check** (`pkg/update/updater.go`)
   - Fetches `{UPDATE_BASE_URL}/version.txt`
   - Compares with current version
   - Returns update availability

2. **Binary Download**
   - Downloads `{UPDATE_BASE_URL}/health-monitor-linux-amd64-{VERSION}`
   - Saves to temporary file
   - Verifies binary (runs `--version`)

3. **Atomic Replacement**
   - Creates backup of current binary
   - Uses `rename()` for atomic replacement
   - Sets executable permissions

4. **Notification**
   - Shows message after successful update
   - Next run uses new binary

### Key Functions

- `CheckForUpdate()`: Checks if update is available
- `PerformUpdate()`: Downloads and installs update
- `CheckAndPerformUpdate()`: Combined check and update with timeout
- `fetchLatestVersion()`: Fetches version from remote
- `downloadBinary()`: Downloads binary file
- `replaceBinary()`: Atomically replaces current binary

### Configuration

- `DefaultUpdateBaseURL`: Default R2 URL
- `UPDATE_BASE_URL`: Environment variable override
- `UpdateCheckTimeout`: 5 seconds
- `UpdateDownloadTimeout`: 30 seconds

### Error Handling

- Network failures: Silent fallback, continue execution
- Invalid binaries: Rejected, current binary unchanged
- Replacement failures: Backup restored, error logged
- Timeouts: Update skipped, continue normally

---

## Testing

### Manual Testing

**Test update mechanism:**
```bash
# 1. Build v0.3
VERSION=v0.3 go build ...

# 2. Upload to R2
# 3. Update version.txt to v0.3
# 4. Run with v0.2 installed
health-monitor
# Should update to v0.3
```

**Test TUI:**
```bash
# Run and test scrolling
health-monitor
# Use arrow keys, j/k, home/end
# Press 'q' to exit
```

**Test error handling:**
```bash
# Set invalid URL
export UPDATE_BASE_URL="https://invalid-url.com"
health-monitor
# Should continue normally despite update failure
```

### Mini-APM Test Matrix (Prometheus/OSS)

**Auto-detect metrics:**
- Only `PROMETHEUS_URL` set, OpenTelemetry `http_client_duration_seconds` present
- Expect: dependency metric auto-detected, edges populated

**Fallback metrics:**
- No OTel metric, but `http_client_request_duration_seconds` present
- Expect: fallback metric selected, edges populated

**Label detection:**
- Destination label present (`peer` or `destination_service`)
- Expect: source/destination labels inferred correctly

**Missing labels (Data Gaps):**
- No destination label on metric
- Expect: APM Data Gaps with suggested fix

**Cardinality guardrail:**
- Destination label has many values; set `APM_CARDINALITY_LIMIT=5`
- Expect: APM Data Gaps and per-edge queries skipped

**Baseline effects:**
- Low traffic or short window
- Expect: confidence downgraded; baseline delta may be N/A

**gRPC client metrics:**
- `rpc_client_duration_seconds` present with `rpc_service` and `rpc_method`
- Expect: gRPC metric auto-detected and labels mapped correctly

**Label scoring:**
- Multiple candidate labels present (e.g., `peer` and `destination_service_name`)
- Expect: highest-score label chosen and logged in APM reasoning

**Two-hop inference:**
- Metrics show `checkout -> payments` and `payments -> db` elevated
- Expect: suspected upstream shows two-hop chain and higher confidence

**Cardinality fallback:**
- High route cardinality
- Expect: route label dropped first; aggregated by destination only if still too high

**Istio metrics:**
- `istio_request_duration_seconds` present with source/destination labels
- Expect: Istio metric selected and labels mapped

**Graph ranking:**
- Multiple upstreams present with varying deltas
- Expect: top 3 ranked causes shown with scores

**Confidence min RPS:**
- `APM_MIN_RPS=1.0` with low-traffic edges
- Expect: confidence capped at Low with “RPS below threshold” evidence

**Reasoning outputs:**
- Verify “Why we think this is the cause”, “What to check next”, “Possible fixes” sections populated

**Service filter:**
- `API_SERVICE=checkout`
- Expect: suspected upstream prefers edges from checkout service

**Route filter:**
- `API_ROUTE=/checkout`
- Expect: edges filtered by route label when available

### Unit Testing

```bash
# Run tests (if available)
go test ./...
```

### Integration Testing

1. Build multiple versions
2. Upload to R2
3. Test update flow
4. Verify metrics appear correctly
5. Test TUI interactions

---

## Release Process

### 1. Update Version

Edit `pkg/model/version.go`:
```go
Version = "v0.4"
```

### 2. Build Binary

```bash
VERSION=v0.4
COMMIT=$(git rev-parse --short HEAD)
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
go build -o health-monitor-linux-amd64-$VERSION \
  -ldflags "-X health-monitor/pkg/model.Version=$VERSION \
             -X health-monitor/pkg/model.Commit=$COMMIT \
             -X health-monitor/pkg/model.BuildTime=$DATE" \
  ./cmd/health-monitor

chmod +x health-monitor-linux-amd64-$VERSION
```

### 3. Upload to R2

1. Upload binary: `health-monitor-linux-amd64-v0.4`
2. Update `version.txt` to `v0.4`
3. Upload `version.txt`
4. Verify both files are accessible

### 4. Test Release

1. Test on server with previous version
2. Verify auto-update works
3. Verify new features work
4. Check for any regressions

### 5. Documentation

- Update CHANGELOG (if maintained)
- Update USER_GUIDE.md if needed
- Update this guide if architecture changes

---

## Code Style

### Naming Conventions

- **Packages**: lowercase, single word
- **Functions**: PascalCase for exported, camelCase for private
- **Variables**: camelCase
- **Constants**: UPPER_SNAKE_CASE

### Error Handling

- Fail silently for non-critical checks
- Log errors to stderr for debugging
- Never crash the application
- Always provide fallbacks

### Comments

- Document exported functions
- Explain complex logic
- Add TODO comments for future work

---

## Dependencies

### Main Dependencies

- `github.com/charmbracelet/bubbletea`: Interactive TUI framework
- `github.com/charmbracelet/lipgloss`: Terminal styling
- `github.com/charmbracelet/glamour`: Markdown rendering

### Adding Dependencies

```bash
go get github.com/package/name
go mod tidy
```

---

## Troubleshooting Development Issues

### Build Errors

**"package not found"**
```bash
go mod tidy
go mod download
```

**"undefined symbol"**
- Check imports
- Verify package names
- Run `go build ./...` to check all packages

### Runtime Errors

**TUI not displaying**
- Check terminal supports ANSI
- Verify bubbletea is working
- Check for errors in stderr

**Update not working**
- Verify URLs are correct
- Check network connectivity
- Test with curl manually

---

## Contributing

1. Follow code style guidelines
2. Add tests for new features
3. Update documentation
4. Test on multiple systems
5. Ensure backward compatibility

---

## Resources

- [Bubbletea Documentation](https://github.com/charmbracelet/bubbletea)
- [Lipgloss Documentation](https://github.com/charmbracelet/lipgloss)
- [Go Documentation](https://golang.org/doc/)
