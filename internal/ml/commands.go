package ml

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"health-monitor/internal/config"
	"health-monitor/internal/output"
	"health-monitor/internal/storage"

	"github.com/charmbracelet/lipgloss"
)

var (
	// CLI Styles
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	idStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("4")) // Blue
	tsStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // Grey
	patternStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("5")) // Purple
	incStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("205")) // Pinkish-orange
	
	// Confidence Styles
	highConf   = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true) // Green
	medConf    = lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true) // Yellow
	lowConf    = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true) // Red

	// Section Styles
	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			PaddingTop(1).
			PaddingBottom(1)
	
	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 2)
	
	keyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Width(15)
	valStyle = lipgloss.NewStyle().Bold(true)
)

func HandlePreventCLI(args []string) int {
	fs := flag.NewFlagSet("prevent", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	// Add interval as a potential global flag for convenience, though it's mainly for start
	interval := fs.Duration("interval", 0, "scanning interval")
	profile := fs.String("profile", "", "profile name")
	
	// We only want to parse flags that come BEFORE the subcommand
	// But the standard flag package doesn't support this easily if we want subcommand-specific flags too.
	// For now, let's keep it simple and just check args[0].
	
	if len(args) == 0 {
		printPreventUsage()
		return 1
	}

	subcommand := args[0]
	subArgs := args[1:]

	// If the user did: health-monitor prevent --interval 10s start
	// We need to re-order or handle it.
	if strings.HasPrefix(subcommand, "-") {
		if err := fs.Parse(args); err != nil {
			return 1
		}
		if fs.NArg() > 0 {
			subcommand = fs.Arg(0)
			subArgs = fs.Args()[1:]
		}
	}

	switch subcommand {
	case "start":
		// If interval was set globally, prepend it to subArgs if not already there
		if *interval > 0 {
			found := false
			for _, a := range subArgs {
				if strings.Contains(a, "interval") {
					found = true
					break
				}
			}
			if !found {
				subArgs = append([]string{"--interval", interval.String()}, subArgs...)
			}
		}
		if *profile != "" {
			subArgs = append([]string{"--profile", *profile}, subArgs...)
		}
		return handleStart(subArgs)
	case "stop":
		return handleStop(args[1:])
	case "status":
		return handleStatus(args[1:])
	case "list":
		return handleList(args[1:])
	case "describe":
		return handleDescribe(args[1:])
	case "purge":
		return handlePurge(args[1:])
	default:
		printPreventUsage()
		return 1
	}
}

func handleStart(args []string) int {
	fs := flag.NewFlagSet("prevent start", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	background := fs.Bool("background", false, "run in background")
	pidFile := fs.String("pid-file", "", "write PID to file")
	interval := fs.Duration("interval", 1*time.Minute, "scanning interval")
	dataDir := fs.String("data-dir", "", "data directory override")
	profile := fs.String("profile", "", "profile name")
	backfill := fs.Duration("backfill", 0, "bootstrap by checking past logs (e.g. 15m)")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	profileName := *profile
	if profileName == "" {
		profileName = config.GetActiveProfileName()
	}

	if *pidFile == "" {
		*pidFile = defaultPIDFile(profileName)
	}

	// 1. Check if already running
	if pid, err := readPID(*pidFile); err == nil && isProcessRunning(pid) {
		output.Warnf("Predictor daemon already running for profile %q (PID %d)", profileName, pid)
		return 1
	}

	// 2. Handle data directory override
	if *dataDir != "" {
		_ = os.Setenv("HEALTH_MONITOR_DATA_DIR", *dataDir)
	}

	if *background && os.Getenv("HEALTH_MONITOR_PREVENT_CHILD") != "1" {
		return startBackground(profileName, *pidFile, *interval, *dataDir)
	}

	// Daemon logic
	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("health-monitor-prevent-%s.log", profileName))
	logF, _ := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if logF != nil {
		defer logF.Close()
		fmt.Fprintf(logF, "[%s] Daemon starting (PID %d)\n", time.Now().Format(time.RFC3339), os.Getpid())
	}

	mlAgent := NewMLAgent("v1")
	mlRecorder := NewLifecycleRecorder(profileName)
	predictor := NewPredictorDaemon(profileName, mlAgent, mlRecorder)
	
	// 🛰️ Active Incident Awareness
	predictor.SetIncidentChecker(&FileSystemIncidentChecker{profile: profileName})
	
	// Setup notification handler (dummy for now, will be integrated in main.go if needed)
	predictor.SetNotifyHandler(func(p Prediction) {
		output.Infof("PREDICTION NOTIFICATION: %s - %s", p.Pattern, p.Explanation)
	})

	if strings.TrimSpace(*pidFile) != "" {
		// Ensure directory exists
		_ = os.MkdirAll(filepath.Dir(*pidFile), 0755)
		
		if err := storage.AtomicWriteFile(*pidFile, []byte(fmt.Sprintf("%d", os.Getpid())), 0644); err != nil {
			output.Warnf("Failed to write PID file %s: %v", *pidFile, err)
			if logF != nil {
				fmt.Fprintf(logF, "[%s] ERROR: Failed to write PID file: %v\n", time.Now().Format(time.RFC3339), err)
			}
		} else {
			if logF != nil {
				fmt.Fprintf(logF, "[%s] PID file written: %s\n", time.Now().Format(time.RFC3339), *pidFile)
			}
		}
	}

	// 🛰️ Internal backfill logic
	if *backfill > 0 {
		predictor.SetBackfillWindow(*backfill)
		if logF != nil {
			fmt.Fprintf(logF, "[%s] Backfilling logs from last %v...\n", time.Now().Format(time.RFC3339), *backfill)
		}
	}

	predictor.Start(*interval)

	// Keep running
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	
	if logF != nil {
		fmt.Fprintf(logF, "[%s] Ticking every %v. Waiting for signals...\n", time.Now().Format(time.RFC3339), *interval)
	}

	sig := <-stop
	if logF != nil {
		fmt.Fprintf(logF, "[%s] Received signal %v, stopping...\n", time.Now().Format(time.RFC3339), sig)
	}
	predictor.Stop()
	return 0
}

func handleStop(args []string) int {
	fs := flag.NewFlagSet("prevent stop", flag.ContinueOnError)
	profile := fs.String("profile", "", "profile name")
	pidFile := fs.String("pid-file", "", "pid file path")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	profileName := *profile
	if profileName == "" {
		profileName = config.GetActiveProfileName()
	}

	if *pidFile == "" {
		*pidFile = defaultPIDFile(profileName)
	}

	pid, err := readPID(*pidFile)
	if err != nil {
		output.Errorf("Daemon not running (could not read PID file: %v)", err)
		return 1
	}

	if !isProcessRunning(pid) {
		output.Warnf("Process %d is not running. Cleaning up PID file.", pid)
		_ = os.Remove(*pidFile)
		return 0
	}

	process, _ := os.FindProcess(pid)
	if err := process.Signal(syscall.SIGTERM); err != nil {
		output.Errorf("Failed to stop daemon: %v", err)
		return 1
	}

	_ = os.Remove(*pidFile)
	output.Infof("Predictor daemon stopped (PID %d)", pid)
	return 0
}

func handleStatus(args []string) int {
	fs := flag.NewFlagSet("prevent status", flag.ContinueOnError)
	profile := fs.String("profile", "", "profile name")
	pidFile := fs.String("pid-file", "", "pid file path")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	profileName := *profile
	if profileName == "" {
		profileName = config.GetActiveProfileName()
	}

	actualPidFile := *pidFile
	if actualPidFile == "" {
		actualPidFile = defaultPIDFile(profileName)
	}

	pid, err := readPID(actualPidFile)
	if err != nil {
		fmt.Printf("Predictor daemon is NOT running for profile %q (could not read %s: %v).\n", profileName, actualPidFile, err)
		return 0
	}
	
	if !isProcessRunning(pid) {
		fmt.Printf("Predictor daemon is NOT running (found stale PID %d in %s).\n", pid, actualPidFile)
		return 0
	}

	fmt.Printf("Predictor daemon is running for profile %q (PID %d)\n", profileName, pid)
	return 0
}

func handleList(args []string) int {
	fs := flag.NewFlagSet("prevent list", flag.ContinueOnError)
	profile := fs.String("profile", "", "profile name")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	profileName := *profile
	if profileName == "" {
		profileName = config.GetActiveProfileName()
	}

	pm := config.GetProfileManager()
	stateDir := pm.GetStatePathForProfile(profileName)
	predDir := filepath.Join(stateDir, "predictions")

	entries, err := os.ReadDir(predDir)
	if err != nil || len(entries) == 0 {
		fmt.Println("No predictions found yet.")
		return 0
	}

	var predictions []Prediction
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			data, err := os.ReadFile(filepath.Join(predDir, entry.Name()))
			if err == nil {
				var p Prediction
				if err := json.Unmarshal(data, &p); err == nil {
					predictions = append(predictions, p)
				}
			}
		}
	}

	// Sort by timestamp descending
	for i := 0; i < len(predictions); i++ {
		for j := i + 1; j < len(predictions); j++ {
			if predictions[j].Timestamp.After(predictions[i].Timestamp) {
				predictions[i], predictions[j] = predictions[j], predictions[i]
			}
		}
	}

	// Sort by timestamp descending
	for i := 0; i < len(predictions); i++ {
		for j := i + 1; j < len(predictions); j++ {
			if predictions[j].Timestamp.After(predictions[i].Timestamp) {
				predictions[i], predictions[j] = predictions[j], predictions[i]
			}
		}
	}

	fmt.Println(headerStyle.Render("PREDICTIVE PREVENTION: RECENT DISCOVERIES"))
	fmt.Println()

	// Adjusted Widths: ID(30), TS(22), CONF(8), PATTERN(25), SOURCE(20)
	hID := headerStyle.Width(30).Align(lipgloss.Left).Render("ID")
	hTS := headerStyle.Width(22).Align(lipgloss.Left).Render("TIMESTAMP")
	hCF := headerStyle.Width(8).Align(lipgloss.Left).Render("CONF.")
	hPT := headerStyle.Width(25).Align(lipgloss.Left).Render("PATTERN")
	hSC := headerStyle.Width(20).Align(lipgloss.Left).Render("SOURCE_INC")
	hEX := headerStyle.Render("EXPLANATION")

	fmt.Printf("%s %s %s %s %s %s\n", hID, hTS, hCF, hPT, hSC, hEX)
	fmt.Println(strings.Repeat("-", 150))

	for _, p := range predictions {
		sourceIncID := "-"
		if strings.Contains(p.Explanation, "past incident") {
			parts := strings.Split(p.Explanation, " ")
			for _, part := range parts {
				clean := strings.TrimSuffix(part, ".")
				if strings.HasPrefix(clean, "INC-") {
					sourceIncID = clean
					break
				}
			}
		}

		// Data rows - Match widths exactly and use Truncate to prevent wrapping
		cid := p.ID
		if len(cid) > 30 { cid = cid[:27] + "..." }
		sid := idStyle.Width(30).Align(lipgloss.Left).Render(cid)
		
		sts := tsStyle.Width(22).Align(lipgloss.Left).Render(p.Timestamp.Format("2006-01-02 15:04:05"))
		
		cpat := p.Pattern
		if len(cpat) > 25 { cpat = cpat[:22] + "..." }
		spat := patternStyle.Width(25).Align(lipgloss.Left).Render(cpat)
		
		confStr := fmt.Sprintf("%.2f", p.Confidence)
		var sconf string
		if p.Confidence >= 0.9 {
			sconf = highConf.Width(8).Align(lipgloss.Left).Render(confStr)
		} else if p.Confidence >= 0.7 {
			sconf = medConf.Width(8).Align(lipgloss.Left).Render(confStr)
		} else {
			sconf = lowConf.Width(8).Align(lipgloss.Left).Render(confStr)
		}

		var sinc string
		if sourceIncID != "-" {
			sinc = incStyle.Width(20).Align(lipgloss.Left).Render(sourceIncID)
		} else {
			sinc = tsStyle.Width(20).Align(lipgloss.Left).Render("-")
		}

		fmt.Printf("%s %s %s %s %s %s\n", 
			sid,
			sts,
			sconf,
			spat,
			sinc,
			p.Explanation)
	}

	fmt.Printf("\n💡 Tip: Use 'health-monitor prevent describe <ID>' for deep resolution intelligence.\n")
	return 0
}

func handleDescribe(args []string) int {
	if len(args) < 1 {
		fmt.Println("Usage: health-monitor prevent describe <prediction-id> [--profile <name>]")
		return 1
	}

	predID := args[0]
	fs := flag.NewFlagSet("prevent describe", flag.ContinueOnError)
	profile := fs.String("profile", "", "profile name")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}

	profileName := *profile
	if profileName == "" {
		profileName = config.GetActiveProfileName()
	}

	pm := config.GetProfileManager()
	stateDir := pm.GetStatePathForProfile(profileName)
	path := filepath.Join(stateDir, "predictions", predID+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		output.Errorf("Prediction %q not found for profile %q.", predID, profileName)
		return 1
	}

	var p Prediction
	if err := json.Unmarshal(data, &p); err != nil {
		output.Errorf("Failed to parse prediction: %v", err)
		return 1
	}

	var sconf string
	confStr := fmt.Sprintf("%.2f", p.Confidence)
	if p.Confidence >= 0.9 {
		sconf = highConf.Render(confStr)
	} else if p.Confidence >= 0.7 {
		sconf = medConf.Render(confStr)
	} else {
		sconf = lowConf.Render(confStr)
	}

	fmt.Println(headerStyle.Render("🔍 PREDICTION DEEP DIVE"))
	
	// Use a wrapping width to prevent border breakage on long text
	terminalWidth := 100 
	wrapStyle := lipgloss.NewStyle().Width(terminalWidth - 20)
	
	timeSince := time.Since(p.Timestamp).Round(time.Second).String() + " ago"
	
	detailsContent := strings.Builder{}
	detailsContent.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("ID:"), valStyle.Render(p.ID)))
	detailsContent.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Pattern:"), patternStyle.Render(p.Pattern)))
	detailsContent.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Category:"), valStyle.Render(p.Category)))
	detailsContent.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Confidence:"), sconf))
	detailsContent.WriteString(fmt.Sprintf("%s %s (%s)\n", keyStyle.Render("Created:"), tsStyle.Render(p.Timestamp.Format(time.RFC3339)), timeSince))
	detailsContent.WriteString(fmt.Sprintf("%s %s", keyStyle.Render("Component:"), valStyle.Render(p.Component)))
	
	fmt.Println(boxStyle.Render(detailsContent.String()))

	fmt.Println(sectionStyle.Render("🧠 ANALYSIS"))
	// Wrap explanation too
	fmt.Printf("  %s\n", wrapStyle.Render(p.Explanation))
	fmt.Printf("  Model: %s\n", tsStyle.Render(p.ModelID))

	if p.RootCause != "" || p.Prevention != "" || p.FixSummary != "" || p.ResolutionHint != "" {
		fmt.Println(sectionStyle.Foreground(lipgloss.Color("10")).Render("⚡ RESOLUTION INTELLIGENCE"))
		
		// Use a wrapping width to prevent border breakage on long text
		terminalWidth := 100 
		wrapStyle := lipgloss.NewStyle().Width(terminalWidth - 10)
		
		rca := strings.Builder{}
		if p.RootCause != "" {
			rca.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Root Cause:"), wrapStyle.Render(p.RootCause)))
		}
		if p.Prevention != "" {
			rca.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Prevention:"), wrapStyle.Render(p.Prevention)))
		}
		if p.FixSummary != "" {
			rca.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Fix:"), wrapStyle.Render(p.FixSummary)))
		}
		if p.ResolutionHint != "" {
			rca.WriteString(fmt.Sprintf("%s %s", keyStyle.Render("Advice:"), wrapStyle.Render(p.ResolutionHint)))
		}
		
		fmt.Println(boxStyle.BorderForeground(lipgloss.Color("10")).Render(strings.TrimSpace(rca.String())))
	}

	// 🚀 NEW: Show Lessons Learned if available
	if p.LessonsLearned != nil && (p.LessonsLearned.WhatWentWell != "" || p.LessonsLearned.WhatCouldBeBetter != "" || p.LessonsLearned.WhereWeGotLucky != "") {
		fmt.Println(sectionStyle.Foreground(lipgloss.Color("12")).Render("🎓 LESSONS FROM PAST"))
		
		lessons := strings.Builder{}
		if p.LessonsLearned.WhatWentWell != "" {
			lessons.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("What Went Well:"), wrapStyle.Render(p.LessonsLearned.WhatWentWell)))
		}
		if p.LessonsLearned.WhatCouldBeBetter != "" {
			lessons.WriteString(fmt.Sprintf("%s %s\n", keyStyle.Render("Needs Improvement:"), wrapStyle.Render(p.LessonsLearned.WhatCouldBeBetter)))
		}
		if p.LessonsLearned.WhereWeGotLucky != "" {
			lessons.WriteString(fmt.Sprintf("%s %s", keyStyle.Render("Where We Got Lucky:"), wrapStyle.Render(p.LessonsLearned.WhereWeGotLucky)))
		}
		fmt.Println(boxStyle.BorderForeground(lipgloss.Color("12")).Render(strings.TrimSpace(lessons.String())))
	}

	// 🚀 NEW: Show Action Items if available
	if len(p.ActionItems) > 0 {
		fmt.Println(sectionStyle.Foreground(lipgloss.Color("13")).Render("📋 RECOMMENDED ACTIONS"))
		for i, action := range p.ActionItems {
			fmt.Printf("  %d. %s\n", i+1, wrapStyle.Render(action))
		}
	}
	
	// 🚀 NEW: Show Full Logs if available
	if p.Snapshot.FullLogs != nil && len(p.Snapshot.FullLogs.TopErrors) > 0 {
		fmt.Println(sectionStyle.Render("📝 CORRELATED ERRORS"))
		for _, err := range p.Snapshot.FullLogs.TopErrors {
			fmt.Printf("  • %s [%d occurrences]\n", lowConf.Render(err.Message), err.Count)
			if err.Sample != "" {
				fmt.Printf("    %s %s\n", tsStyle.Render("Sample:"), err.Sample)
			}
		}
	}

	// 🚀 NEW: Show Full Traces if available
	if p.Snapshot.FullTraces != nil {
		fmt.Println(sectionStyle.Render("⏱️ PERFORMANCE TRACES"))
		fmt.Printf("  P95 Latency   : %s\n", highConf.Render(fmt.Sprintf("%.2fms", p.Snapshot.FullTraces.P95LatencyMs)))
		fmt.Printf("  Slowest Route : %s\n", valStyle.Render(p.Snapshot.FullTraces.SlowestRoute))
		if len(p.Snapshot.FullTraces.TopSpans) > 0 {
			fmt.Println("  Top Spans:")
			for i, span := range p.Snapshot.FullTraces.TopSpans {
				if i >= 3 { break }
				fmt.Printf("    - %s (%s)\n", span.Name, tsStyle.Render(fmt.Sprintf("%.2fms", span.DurationMs)))
			}
		}
	}

	if len(p.Snapshot.Metrics) > 0 {
		fmt.Println(sectionStyle.Render("📊 CAPTURED METRICS"))
		for k, v := range p.Snapshot.Metrics {
			fmt.Printf("  %-25s: %s\n", k, valStyle.Render(fmt.Sprintf("%.4f", v)))
		}
	}

	if len(p.Snapshot.Metadata) > 0 {
		fmt.Println(sectionStyle.Render("🏷️ METADATA CONTEXT"))
		for k, v := range p.Snapshot.Metadata {
			if v == "" { continue }
			fmt.Printf("  %-20s: %s\n", k, tsStyle.Render(v))
		}
	}

	fmt.Println()
	return 0
}

func handlePurge(args []string) int {
	fs := flag.NewFlagSet("prevent purge", flag.ContinueOnError)
	profile := fs.String("profile", "", "profile name")
	days := fs.Int("days", 7, "delete predictions older than X days")
	force := fs.Bool("force", false, "skip confirmation")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	profileName := *profile
	if profileName == "" {
		profileName = config.GetActiveProfileName()
	}

	pm := config.GetProfileManager()
	stateDir := pm.GetStatePathForProfile(profileName)
	predDir := filepath.Join(stateDir, "predictions")

	entries, err := os.ReadDir(predDir)
	if err != nil {
		output.Errorf("Failed to read predictions: %v", err)
		return 1
	}

	cutoff := time.Now().AddDate(0, 0, -*days)
	toDelete := []string{}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			info, err := entry.Info()
			if err == nil && info.ModTime().Before(cutoff) {
				toDelete = append(toDelete, filepath.Join(predDir, entry.Name()))
			}
		}
	}

	if len(toDelete) == 0 {
		fmt.Printf("No predictions older than %d days found for profile %q.\n", *days, profileName)
		return 0
	}

	if !*force {
		fmt.Printf("This will delete %d predictions older than %d days. Continue? [y/N]: ", len(toDelete), *days)
		var confirm string
		fmt.Scanln(&confirm)
		if strings.ToLower(confirm) != "y" {
			fmt.Println("Cancelled.")
			return 0
		}
	}

	count := 0
	for _, p := range toDelete {
		if err := os.Remove(p); err == nil {
			count++
		}
	}

	output.Infof("Successfully purged %d predictions.", count)
	return 0
}

func startBackground(profile, pidFile string, interval time.Duration, dataDir string) int {
	exe, err := os.Executable()
	if err != nil {
		output.Errorf("Failed to locate executable: %v", err)
		return 1
	}

	args := []string{"prevent", "start", "--interval", interval.String(), "--pid-file", pidFile}
	if profile != "" {
		args = append([]string{"--profile", profile}, args...)
	}
	if dataDir != "" {
		args = append(args, "--data-dir", dataDir)
	}

	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Env = append(os.Environ(), "HEALTH_MONITOR_PREVENT_CHILD=1")
	
	// Open log file for redirection
	logPath := filepath.Join(os.TempDir(), fmt.Sprintf("health-monitor-prevent-%s.log", profile))
	logF, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		cmd.Stdout = logF
		cmd.Stderr = logF
	}

	if err := cmd.Start(); err != nil {
		output.Errorf("Failed to start background daemon: %v", err)
		return 1
	}

	fmt.Printf("Predictor daemon started in background (PID %d)\n", cmd.Process.Pid)
	return 0
}

func defaultPIDFile(profile string) string {
	if profile == "" || profile == "default" {
		return filepath.Join(os.TempDir(), "health-monitor-prevent.pid")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("health-monitor-prevent-%s.pid", profile))
}

func readPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var pid int
	_, err = fmt.Sscanf(string(data), "%d", &pid)
	return pid, err
}

func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil
}

func printPreventUsage() {
	fmt.Print(`Predictive Prevention Commands
==============================

USAGE
  health-monitor prevent start [--profile <name>] [--background] [--pid-file <path>] [--interval <duration>] [--data-dir <path>] [--backfill <duration>]
  health-monitor prevent stop [--profile <name>] [--pid-file <path>]
  health-monitor prevent status [--profile <name>] [--pid-file <path>]
  health-monitor prevent list [--profile <name>]
  health-monitor prevent describe <prediction-id> [--profile <name>]
  health-monitor prevent purge [--profile <name>] [--days <7>] [--force]

EXAMPLES
  sudo health-monitor prevent start --profile production --background
  sudo health-monitor prevent status --profile production
  health-monitor prevent list --profile production
  health-monitor prevent describe PRED-123456789
  health-monitor prevent purge --days 30
`)
}
