package alert

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/flow"
	"health-monitor/internal/incident"
	_ "health-monitor/internal/metrics"
)

func HandleCLI(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}
	switch args[0] {
	case "listen":
		return handleListen(args[1:])
	case "reload":
		return handleReload(args[1:])
	case "list-rules":
		return handleListRules(args[1:])
	case "validate-config":
		return handleValidate(args[1:])
	case "test":
		return handleTest(args[1:])
	case "status":
		return handleStatus(args[1:])
	case "stop":
		return handleStop(args[1:])
	default:
		printUsage()
		return 1
	}
}

func handleListen(args []string) int {
	fs := flag.NewFlagSet("alert listen", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	addr := fs.String("addr", "127.0.0.1:9095", "bind address")
	tlsCert := fs.String("tls-cert", "", "path to TLS certificate")
	tlsKey := fs.String("tls-key", "", "path to TLS key")
	background := fs.Bool("background", false, "run listener in background")
	pidFile := fs.String("pid-file", "", "write PID to file")
	once := fs.Bool("once", false, "handle a single request then exit")
	dataDir := fs.String("data-dir", "", "data directory (incidents stored under data-dir/incidents)")
	authToken := fs.String("auth-token", "", "bearer token for webhook auth")
	authTokenFile := fs.String("auth-token-file", "", "file containing bearer token")
	authUser := fs.String("auth-user", "", "basic auth user for webhook")
	authPass := fs.String("auth-pass", "", "basic auth password for webhook")
	authBasicFile := fs.String("auth-basic-file", "", "file containing basic auth user:pass")
	logFormat := fs.String("log-format", "json", "log format: json or text")
	logFile := fs.String("log-file", "", "log file path")
	maxRPS := fs.Int("max-rps", 100, "max webhook requests per second (0 disables)")
	maxWorkers := fs.Int("max-workers", 4, "max concurrent alert workers")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	
	// Initialize profile context if not already done
	if err := config.InitializeFromCLI(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize profile: %v\n", err)
		return 1
	}
	
	// Profile-specific data directory handling
	if strings.TrimSpace(*dataDir) != "" {
		// Override profile state directory
		pm := config.GetProfileManager()
		profileStateDir := pm.GetStatePath()
		_ = os.Setenv("HEALTH_MONITOR_DATA_DIR", profileStateDir)
	}
	SetLogFormat(*logFormat)
	if strings.TrimSpace(*pidFile) == "" && *background {
		*pidFile = defaultPIDFile()
	}
	if *background && os.Getenv("HEALTH_MONITOR_ALERT_CHILD") != "1" {
		return startBackground(*addr, *pidFile, *once, *tlsCert, *tlsKey, *authToken, *authTokenFile, *authUser, *authPass, *authBasicFile, *dataDir, *logFile)
	}
	
	// Start flow watcher for hot-reload
	flow.StartWatcher(context.Background(), 5*time.Second, func() {
		fmt.Println("Flow configuration changes detected. Reloading...")
	})
	if *background && os.Getenv("HEALTH_MONITOR_ALERT_CHILD") == "1" {
		os.Setenv("HEALTH_MONITOR_ALERT_CHILD", "")
	}
	if *once {
		checkFile := strings.TrimSpace(*pidFile)
		if checkFile == "" {
			checkFile = defaultPIDFile()
		}
		if pid, err := ReadPIDFile(checkFile); err == nil && IsProcessRunning(pid) {
			logWarn("listener already running", map[string]string{"pid": fmt.Sprintf("%d", pid)})
			fmt.Println("Listener already running.")
			return 1
		}
	}
	if (strings.TrimSpace(*tlsCert) == "") != (strings.TrimSpace(*tlsKey) == "") {
		logError("tls cert and key must be provided together", nil)
		return 1
	}
	authCfg, err := LoadAuthConfig(*authToken, *authTokenFile, *authUser, *authPass, *authBasicFile)
	if err != nil {
		logError("failed to load auth config", map[string]string{"error": err.Error()})
		return 1
	}
	result, err := LoadProfileAware()
	if err != nil {
		if IsNoConfig(err) {
			logWarn("alerts disabled (no config found)", nil)
			result = LoadResult{Rules: nil}
		} else {
			printLoadError(err)
			return 1
		}
	}
	for _, warning := range result.Warnings {
		logWarn(warning, nil)
	}
	service, err := incident.NewService(nil)
	if err != nil {
		logError("failed to initialize incident service", map[string]string{"error": err.Error()})
		return 1
	}
	
	// Check config for dedup interval
	cfg, err := config.Load()
	dedupInterval := 2 * time.Minute
	if err == nil && cfg.AlertDedupIntervalMinutes > 0 {
		dedupInterval = time.Duration(cfg.AlertDedupIntervalMinutes) * time.Minute
	}

	processor := NewProcessor(service, result.Rules, dedupInterval)
	processor.ResetCache()
	processor.Stats.Reset()
	
	// Set status metrics
	processor.Stats.SetAlertsLoaded(len(result.Rules) > 0)
	
	// Check flows and set metrics
	if _, err := flow.Load(); err != nil {
		processor.Stats.SetFlowsLoaded(false)
	} else {
		processor.Stats.SetFlowsLoaded(true)
	}
	
	// Check config validity and set metrics
	processor.Stats.SetConfigValid(err == nil && cfg.GrafanaURL != "")
	
	if *maxWorkers > 0 {
		processor.MaxWorkers = *maxWorkers
	}
	if *maxRPS > 0 {
		processor.Rate = NewRateLimiter(*maxRPS, *maxRPS)
	}
	go func() {
		reload := make(chan os.Signal, 1)
		signal.Notify(reload, syscall.SIGHUP)
		for range reload {
			newResult, err := LoadProfileAware()
			if err != nil {
				logWarn("reload failed", map[string]string{"error": err.Error()})
				continue
			}
			processor.SetRules(newResult.Rules)
			processor.ResetCache()
			logWarn("alert rules reloaded", map[string]string{"count": fmt.Sprintf("%d", len(newResult.Rules))})
		}
	}()
	var mux *http.ServeMux
	var server *Server
	var handler http.Handler
	if *once {
		var onceGuard sync.Once
		mux = http.NewServeMux()
		server = NewServer(*addr, mux)
		mux.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
			processor.HandleWebhook(w, r)
			onceGuard.Do(func() {
				go server.Close()
			})
		})
		registerMetrics(mux, processor)
	} else {
		// Use secure handler with security configuration
		cfg, err := config.LoadForProfile(config.GetActiveProfileName())
		if err != nil {
			logError("failed to load config", map[string]string{"error": err.Error()})
			return 1
		}
		handler, err = NewSecureMux(processor, cfg.Security, func() bool { return len(result.Rules) > 0 })
		if err != nil {
			logError("failed to create secure handler", map[string]string{"error": err.Error()})
			return 1
		}
		server = NewServer(*addr, handler)
	}
	if handler == nil {
		handler = authMiddleware(mux, authCfg)
	}
	if handler != nil && server.httpServer != nil {
		server.httpServer.Handler = handler
	}
	if strings.TrimSpace(*logFile) != "" {
		f, err := os.OpenFile(*logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			defer f.Close()
			os.Stderr = f
			os.Stdout = f
			// Re-initialize logger with new file if needed, but alert package handles its own logging
		}
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		logError("alert listener failed", map[string]string{"error": err.Error()})
		return 1
	}
	if strings.TrimSpace(*pidFile) != "" {
		if err := WritePIDFile(*pidFile); err != nil {
			logWarn("failed to write pid file", map[string]string{"error": err.Error()})
		}
	}
	fmt.Printf("Alert listener started on %s\n", *addr)
	if strings.TrimSpace(*tlsCert) != "" {
		if err := server.ServeTLS(ln, *tlsCert, *tlsKey); err != nil {
			logError("alert listener failed", map[string]string{"error": err.Error()})
			return 1
		}
		return 0
	}
	if err := server.Serve(ln); err != nil {
		logError("alert listener failed", map[string]string{"error": err.Error()})
		return 1
	}
	return 0
}

func handleListRules(args []string) int {
	fs := flag.NewFlagSet("alert list-rules", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	result, err := LoadProfileAware()
	if err != nil {
		printLoadError(err)
		return 1
	}
	for _, warning := range result.Warnings {
		logWarn(warning, nil)
	}
	if len(result.Rules) == 0 {
		fmt.Println("No alert rules configured.")
		return 0
	}
	for _, rule := range result.Rules {
		fmt.Printf("match=%s severity=%s mode=%s title=%s\n",
			formatMatch(rule.Match),
			rule.Severity,
			rule.Mode,
			strings.TrimSpace(rule.Title),
		)
	}
	return 0
}

func handleValidate(args []string) int {
	fs := flag.NewFlagSet("alert validate-config", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	result, err := LoadProfileAware()
	if err != nil {
		printLoadError(err)
		return 1
	}
	for _, warning := range result.Warnings {
		logWarn(warning, nil)
	}
	fmt.Printf("Alert config OK (%d rule(s))\n", len(result.Rules))
	return 0
}

func handleTest(args []string) int {
	fs := flag.NewFlagSet("alert test", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	payloadPath := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if payloadPath == "" {
		logError("payload path is required", nil)
		return 1
	}
	result, err := LoadProfileAware()
	if err != nil {
		printLoadError(err)
		return 1
	}
	for _, warning := range result.Warnings {
		logWarn(warning, nil)
	}
	payload, err := readPayloadFile(payloadPath)
	if err != nil {
		logError("failed to read payload", map[string]string{"error": err.Error()})
		return 1
	}
	// Check config for dedup interval
	cfg, _ := config.Load()
	dedupInterval := 2 * time.Minute
	if cfg.AlertDedupIntervalMinutes > 0 {
		dedupInterval = time.Duration(cfg.AlertDedupIntervalMinutes) * time.Minute
	}

	service, _ := incident.NewService(nil); processor := NewProcessor(service, result.Rules, dedupInterval)
	decisions, warnings := processor.Process(payload, true)
	for _, warning := range warnings {
		logWarn(warning, nil)
	}
	if len(decisions) == 0 {
		fmt.Println("No actions would be taken.")
		return 0
	}
	for _, decision := range decisions {
		fmt.Println(decision.Message)
	}
	return 0
}

func handleStatus(args []string) int {
	fs := flag.NewFlagSet("alert status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pidFile := fs.String("pid-file", defaultPIDFile(), "pid file path")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	info, err := ReadPIDFile(*pidFile)
	if err != nil {
		logWarn("listener not running", map[string]string{"pid_file": *pidFile})
		fmt.Println("Listener not running.")
		return 1
	}
	if !IsProcessRunning(info) {
		_ = os.Remove(*pidFile)
		logWarn("listener not running", map[string]string{"pid": fmt.Sprintf("%d", info)})
		fmt.Println("Listener not running.")
		return 1
	}
	fmt.Printf("Listener running (pid %d)\n", info)
	return 0
}

func handleStop(args []string) int {
	fs := flag.NewFlagSet("alert stop", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pidFile := fs.String("pid-file", defaultPIDFile(), "pid file path")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if scope, ok := systemdScope(); ok && systemctlAvailable() {
		if err := systemctlStop(scope); err != nil {
			logWarn("systemd stop failed, falling back to pid file", map[string]string{"error": err.Error()})
		} else {
			if strings.TrimSpace(*pidFile) != "" {
				_ = os.Remove(*pidFile)
			}
			fmt.Println("Listener stopped (systemd).")
			return 0
		}
	}
	info, err := ReadPIDFile(*pidFile)
	if err != nil {
		logWarn("listener not running", map[string]string{"pid_file": *pidFile})
		fmt.Println("Listener not running.")
		return 1
	}
	if !IsProcessRunning(info) {
		_ = os.Remove(*pidFile)
		fmt.Println("Listener not running.")
		return 1
	}
	process, err := os.FindProcess(info)
	if err != nil {
		logError("failed to find process", map[string]string{"error": err.Error()})
		return 1
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		logError("failed to stop listener", map[string]string{"error": err.Error()})
		return 1
	}
	for attempts := 0; attempts < 40; attempts++ {
		if !IsProcessRunning(info) {
			_ = os.Remove(*pidFile)
			fmt.Printf("Listener stopped (pid %d)\n", info)
			return 0
		}
		time.Sleep(50 * time.Millisecond)
	}
	logWarn("listener still running after stop", map[string]string{"pid": fmt.Sprintf("%d", info)})
	return 0
}

func handleReload(args []string) int {
	fs := flag.NewFlagSet("alert reload", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pidFile := fs.String("pid-file", defaultPIDFile(), "pid file path")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	pid, err := ReadPIDFile(*pidFile)
	if err != nil {
		logWarn("listener not running", map[string]string{"pid_file": *pidFile})
		fmt.Println("Listener not running.")
		return 1
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		logError("failed to find process", map[string]string{"error": err.Error()})
		return 1
	}
	if err := process.Signal(syscall.SIGHUP); err != nil {
		logError("reload failed", map[string]string{"error": err.Error()})
		return 1
	}
	fmt.Printf("Reload signal sent (pid %d)\n", pid)
	return 0
}

func printUsage() {
	fmt.Print(`Alert Commands
==============

USAGE
  health-monitor alert listen --addr :9095 [--background] [--pid-file <path>] [--once]
                              [--tls-cert <path> --tls-key <path>]
                              [--auth-token <token> | --auth-token-file <path>]
                              [--auth-user <user> --auth-pass <pass> | --auth-basic-file <path>]
                              [--data-dir <path>] [--log-format json|text]
                              [--max-rps <n>] [--max-workers <n>]
  health-monitor alert reload [--pid-file <path>]
  health-monitor alert list-rules
  health-monitor alert validate-config
  health-monitor alert test <payload.json>
  health-monitor alert status [--pid-file <path>]
  health-monitor alert stop [--pid-file <path>]

CONFIG PATHS
  /etc/health-monitor/alerts.yaml
  /etc/health-monitor/alerts.d/*.yaml
  ~/.health-monitor/alerts.yaml
  ~/.health-monitor/alerts.d/*.yaml
`)
}

func printLoadError(err error) {
	switch {
	case IsNoConfig(err):
		logWarn("alerts disabled (no config found)", nil)
	case IsInvalidConfig(err):
		logWarn("invalid alert config — skipping", nil)
	default:
		logError("failed to load alerts", map[string]string{"error": err.Error()})
	}
}

func startBackground(addr string, pidFile string, once bool, tlsCert string, tlsKey string, authToken string, authTokenFile string, authUser string, authPass string, authBasicFile string, dataDir string, logFile string) int {
	exe, err := os.Executable()
	if err != nil {
		logError("failed to locate executable", map[string]string{"error": err.Error()})
		return 1
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logError("address already in use", map[string]string{"error": err.Error(), "addr": addr})
		return 1
	}
	_ = ln.Close()
	if scope, ok := systemdScope(); ok && systemctlAvailable() {
		if err := systemctlStart(scope); err != nil {
			logWarn("systemd start failed, falling back to background", map[string]string{"error": err.Error()})
		} else {
			fmt.Println("Listener started via systemd.")
			return 0
		}
	}
	var args []string
	if profile := config.GetActiveProfileName(); profile != "" {
		args = append(args, "--profile", profile)
	}
	args = append(args, "alert", "listen", "--addr", addr)
	if strings.TrimSpace(pidFile) != "" {
		args = append(args, "--pid-file", pidFile)
	}
	if once {
		args = append(args, "--once")
	}
	if strings.TrimSpace(tlsCert) != "" {
		args = append(args, "--tls-cert", tlsCert, "--tls-key", tlsKey)
	}
	if strings.TrimSpace(authToken) != "" {
		args = append(args, "--auth-token", authToken)
	}
	if strings.TrimSpace(authTokenFile) != "" {
		args = append(args, "--auth-token-file", authTokenFile)
	}
	if strings.TrimSpace(authUser) != "" {
		args = append(args, "--auth-user", authUser)
	}
	if strings.TrimSpace(authPass) != "" {
		args = append(args, "--auth-pass", authPass)
	}
	if strings.TrimSpace(authBasicFile) != "" {
		args = append(args, "--auth-basic-file", authBasicFile)
	}
	if strings.TrimSpace(dataDir) != "" {
		args = append(args, "--data-dir", dataDir)
	}
	if strings.TrimSpace(logFile) != "" {
		args = append(args, "--log-file", logFile)
	}
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Env = append(os.Environ(), "HEALTH_MONITOR_ALERT_CHILD=1")
	if err := cmd.Start(); err != nil {
		logError("failed to start background listener", map[string]string{"error": err.Error()})
		return 1
	}
	pidPath := strings.TrimSpace(pidFile)
	claimedPID := cmd.Process.Pid
	if pidPath != "" {
		for attempts := 0; attempts < 40; attempts++ {
			if pid, err := ReadPIDFile(pidPath); err == nil && IsProcessRunning(pid) {
				fmt.Printf("Listener running in background (PID %d)\n", pid)
				return 0
			}
			time.Sleep(50 * time.Millisecond)
		}
		if !IsProcessRunning(claimedPID) {
			logError("listener failed to start in background", map[string]string{
				"pid":  fmt.Sprintf("%d", claimedPID),
				"hint": "run without --background to see errors",
			})
			return 1
		}
		if err := WritePIDFileWithPID(pidPath, claimedPID); err != nil {
			logWarn("failed to write pid file fallback", map[string]string{"error": err.Error()})
		}
	}
	// Final verification: ensure the process is still running before claiming success.
	time.Sleep(150 * time.Millisecond)
	if !IsProcessRunning(claimedPID) {
		logError("listener failed to start in background", map[string]string{
			"pid":  fmt.Sprintf("%d", claimedPID),
			"hint": "run without --background to see errors",
		})
		return 1
	}
	fmt.Printf("Listener running in background (PID %d)\n", claimedPID)
	return 0
}

func systemdScope() (string, bool) {
	if userUnitPath() != "" {
		return "user", true
	}
	if systemUnitPath() != "" {
		return "system", true
	}
	return "", false
}

func userUnitPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	path := filepath.Join(home, ".config", "systemd", "user", "health-monitor-alert.service")
	if fileExists(path) {
		return path
	}
	return ""
}

func systemUnitPath() string {
	path := "/etc/systemd/system/health-monitor-alert.service"
	if fileExists(path) {
		return path
	}
	return ""
}

func systemctlAvailable() bool {
	_, err := exec.LookPath("systemctl")
	return err == nil
}

func systemctlStart(scope string) error {
	args := []string{}
	if scope == "user" {
		args = append(args, "--user")
	}
	args = append(args, "start", "health-monitor-alert.service")
	cmd := exec.Command("systemctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func systemctlStop(scope string) error {
	args := []string{}
	if scope == "user" {
		args = append(args, "--user")
	}
	args = append(args, "stop", "health-monitor-alert.service")
	cmd := exec.Command("systemctl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
