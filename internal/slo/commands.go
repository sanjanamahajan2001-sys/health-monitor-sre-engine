package slo

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	
	"context"
	"io"
	"log"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	
	"health-monitor/internal/alert"
	"health-monitor/internal/config"
	"health-monitor/internal/flow"
	"health-monitor/internal/incident"
	"health-monitor/internal/metrics"
)

// HandleSLOCommand routes SLO subcommands
func HandleSLOCommand(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "SLO subcommand required (list)")
		return 1
	}
	
	switch args[0] {
	case "list":
		return handleSLOList(args[1:])
	case "monitor":
		if len(args) > 1 {
			switch args[1] {
			case "stop":
				return handleSLOMonitorStop(args[2:])
			case "status":
				return handleSLOMonitorStatus(args[2:])
			}
		}
		return handleSLOMonitor(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown SLO subcommand: %s\n", args[0])
		return 1
	}
}

func handleSLOList(args []string) int {
	fs := flag.NewFlagSet("slo list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	
	if err := fs.Parse(args); err != nil {
		return 1
	}
	
	// Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		return 1
	}
	
	// Load flows
	flowResult, err := flow.LoadOnce()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load flows: %v\n", err)
		return 1
	}
	
	// Create metrics provider
	provider, err := metrics.NewMetricProvider(&cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create metrics provider: %v\n", err)
		return 1
	}
	
	// Create SLO service
	service, err := NewService(provider)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create SLO service: %v\n", err)
		return 1
	}
	
	// Check SLOs
	results := service.CheckSLOs(flowResult.Flows)
	
	if len(results) == 0 {
		fmt.Println("No SLOs configured.")
		return 0
	}
	
	// Print table
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tFLOW\tOBJ\tWINDOW\tCURRENT\tBURN(5m/1h)\tBUDGET(30d)\tTREND\tCOMPLIANCE\tRISK")
	
	for _, r := range results {
		// Compliance status
		compIcon := "[?]"
		var compText string
		
		switch r.Compliance {
		case "OK", "COMPLIANT": // Support both old and new
			compIcon = "[OK]"
			compText = "OK"
		case "BREACHING":
			compIcon = "[XX]"
			compText = "BREACHING"
		case "NO_TRAFFIC":
			compIcon = "[-]"
			compText = "NO_TRAFFIC"
		case "NO_DATA":
			compIcon = "[!]"
			compText = "NO_DATA"
		case "ERROR":
			compIcon = "[ERR]"
			compText = "ERROR"
		case "INSUFFICIENT_DATA": // Legacy support
			compIcon = "[-]"
			compText = "INSUFFICIENT_DATA"
		default:
			compIcon = "[?]"
			compText = "UNKNOWN"
		}
		
		// Risk status
		riskText := r.Risk
		if r.Risk == "CRITICAL" {
			riskText = "CRITICAL"
		} else if r.Risk == "HIGH" {
			riskText = "HIGH"
		}
		
		current := r.FormatCurrent()
		burn := r.FormatBurnRate()
		budget := r.FormatBudget()
		
		// Override current value for special status types
		if r.Compliance == "NO_TRAFFIC" {
			current = "N/A"
		} else if r.Compliance == "ERROR" {
			current = "ERR"
		} else if r.Compliance == "NO_DATA" {
			current = "N/A"
		}
		
		// Risk assessment (only for valid data)
		if r.DataQuality != "good" {
			riskText = "-"
		}
		
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s %s\t%s\n",
			r.SLOID,
			r.FlowID,
			r.FormatObjective(),
			r.Window,
			current,
			burn,
			budget,
			r.Trend,
			compIcon,
			compText,
			riskText,
		)
	}
	
	w.Flush()
	return 0
}

func handleSLOMonitor(args []string) int {
	fs := flag.NewFlagSet("slo monitor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	interval := fs.Duration("interval", 1*time.Minute, "monitoring interval")
	background := fs.Bool("background", false, "run monitor in background")
	pidFile := fs.String("pid-file", "", "path to PID file")
	logFile := fs.String("log-file", "", "path to log file (for background mode)")
	
	if err := fs.Parse(args); err != nil {
		return 1
	}

	// Background execution logic
	if *background && os.Getenv("HEALTH_MONITOR_SLO_CHILD") != "1" {
		return startSLOBackground(*interval, *pidFile, *logFile)
	}

	// 1. Logging setup must be ABSOLUTELY FIRST for the child
	var logWriter io.Writer = os.Stderr
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "CRITICAL: Failed to open log file: %v\n", err)
			return 1
		}
		if os.Getenv("HEALTH_MONITOR_SLO_CHILD") == "1" {
			logWriter = f
		} else {
			logWriter = io.MultiWriter(os.Stderr, f)
		}
		log.SetOutput(logWriter)
	}

	// 2. PID file handling - do this early but after log setup
	if *pidFile != "" {
		if err := alert.WritePIDFileWithPID(*pidFile, os.Getpid()); err != nil {
			log.Printf("CRITICAL ERROR: Failed to write pid file %s: %v", *pidFile, err)
			return 1
		}
		log.Printf("INFO: PID file written to %s (PID: %d)", *pidFile, os.Getpid())
		// We remove the PID file on exit
		defer func() {
			log.Printf("INFO: Cleaning up PID file %s", *pidFile)
			os.Remove(*pidFile)
		}()
	}

	cfg, err := config.Load()
	if err != nil {
		log.Printf("CRITICAL ERROR: Failed to load config: %v", err)
		return 1
	}

	flowResult, err := flow.LoadOnce()
	if err != nil {
		log.Printf("CRITICAL ERROR: Failed to load flows: %v", err)
		return 1
	}

	provider, err := metrics.NewMetricProvider(&cfg)
	if err != nil {
		log.Printf("CRITICAL ERROR: Failed to create metrics provider: %v", err)
		return 1
	}

	service, err := NewService(provider)
	if err != nil {
		log.Printf("CRITICAL ERROR: Failed to create SLO service: %v", err)
		return 1
	}

	incService, err := incident.NewService(nil)
	if err != nil {
		log.Printf("CRITICAL ERROR: Failed to create incident service: %v", err)
		return 1
	}

	// Load alert rules to instantiate processor
	alertResult, err := alert.LoadProfileAware()
	if err != nil {
		log.Printf("CRITICAL ERROR: Failed to load alert rules: %v", err)
		return 1
	}

	dedupInterval := time.Duration(cfg.AlertDedupIntervalMinutes) * time.Minute
	if dedupInterval <= 0 {
		dedupInterval = 2 * time.Minute
	}
	processor := alert.NewProcessor(incService, alertResult.Rules, dedupInterval)
	
	ctx := context.Background()
	log.Printf("INFO: Starting SLO background monitor (interval: %v, pid: %d)", *interval, os.Getpid())
	
	// Start monitoring loop
	service.Monitor(ctx, *interval, flowResult.Flows, func(res SLOResult) {
		// Only process results that are actually BREACHING
		if res.Compliance == "BREACHING" || res.Risk == "HIGH" || res.Risk == "CRITICAL" {
			log.Printf("SLO Breach Detected: %s for %s (%s). Risk: %s", 
				res.SLOID, res.FlowID, res.Compliance, res.Risk)
			
			// Process using the bridge
			ProcessSLOResults(processor, []SLOResult{res})
		} else if res.Error != nil {
			log.Printf("WARN: SLO Check failed for %s: %v", res.SLOID, res.Error)
		}
	})

	return 0
}

func startSLOBackground(interval time.Duration, pidFile, logFile string) int {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get executable path: %v\n", err)
		return 1
	}

	var args []string
	if profile := config.GetActiveProfileName(); profile != "" {
		args = append(args, "--profile", profile)
	}
	args = append(args, "slo", "monitor", "--interval", interval.String())
	if pidFile != "" {
		args = append(args, "--pid-file", pidFile)
	}
	if logFile != "" {
		args = append(args, "--log-file", logFile)
	}

	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	
	// Environment inheritance handles profile propagation via HEALTH_MONITOR_PROFILE
	cmd.Env = append(os.Environ(), "HEALTH_MONITOR_SLO_CHILD=1")
	
	// If a log file is provided, redirect child's stdout/stderr to it to capture startup errors
	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			cmd.Stdout = f
			cmd.Stderr = f
			defer f.Close()
		}
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start background monitor: %v\n", err)
		return 1
	}

	// Wait and verify the child actually wrote its PID file
	success := false
	for i := 0; i < 5; i++ {
		time.Sleep(200 * time.Millisecond)
		if !alert.IsProcessRunning(cmd.Process.Pid) {
			break
		}
		if pidFile != "" {
			if _, err := alert.ReadPIDFile(pidFile); err == nil {
				success = true
				break
			}
		} else {
			// No pid file to check, just hope for the best after full wait
			if i == 4 { success = true }
		}
	}

	if !success {
		fmt.Fprintf(os.Stderr, "Monitor failed to start or write PID file. Check logs: %s\n", logFile)
		return 1
	}

	fmt.Printf("SLO Monitor running in background (PID %d)\n", cmd.Process.Pid)
	if logFile != "" {
		fmt.Printf("Logs redirected to: %s\n", logFile)
	}
	return 0
}

func handleSLOMonitorStatus(args []string) int {
	fs := flag.NewFlagSet("slo monitor status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pidFile := fs.String("pid-file", "", "path to PID file")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	path := *pidFile
	if path == "" {
		pm := config.GetProfileManager()
		activeProfile := pm.GetActiveProfile()
		path = filepath.Join(os.TempDir(), fmt.Sprintf("health-monitor-slo-%s.pid", activeProfile))
	}

	pid, err := alert.ReadPIDFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SLO Monitor not running (PID file %s not found)\n", path)
		return 1
	}

	if !alert.IsProcessRunning(pid) {
		fmt.Fprintf(os.Stderr, "SLO Monitor (PID %d) is not running. Stale PID file removed.\n", pid)
		_ = os.Remove(path)
		return 1
	}

	fmt.Printf("SLO Monitor is running (PID %d)\n", pid)
	return 0
}

func handleSLOMonitorStop(args []string) int {
	fs := flag.NewFlagSet("slo monitor stop", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	pidFile := fs.String("pid-file", "", "path to PID file")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	path := *pidFile
	if path == "" {
		// Calculate default PID file for SLO monitor (different from alert listener)
		pm := config.GetProfileManager()
		activeProfile := pm.GetActiveProfile()
		path = filepath.Join(os.TempDir(), fmt.Sprintf("health-monitor-slo-%s.pid", activeProfile))
	}

	pid, err := alert.ReadPIDFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: monitor not running or PID file %s not found\n", path)
		return 1
	}

	if !alert.IsProcessRunning(pid) {
		fmt.Fprintf(os.Stderr, "Error: process %d is not running\n", pid)
		_ = os.Remove(path)
		return 1
	}

	// Send SIGTERM
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to kill process %d: %v\n", pid, err)
		return 1
	}

	fmt.Printf("Sent stop signal to SLO monitor (PID %d)\n", pid)
	// Give it a moment to cleanup
	for i := 0; i < 10; i++ {
		if !alert.IsProcessRunning(pid) {
			fmt.Println("Monitor stopped successfully.")
			return 0
		}
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Println("Monitor is taking a while to stop. It should exit soon.")
	return 0
}
