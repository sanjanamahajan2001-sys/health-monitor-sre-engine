package doctor

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"health-monitor/internal/alert"
	"health-monitor/internal/config"
	"health-monitor/internal/flow"
	"health-monitor/internal/incident"
)

// RunDoctor performs comprehensive health checks
func RunDoctor() int {
	fmt.Println("🔍 Health Monitor Doctor")
	fmt.Println("========================")
	
	allPassed := true
	
	// Check data directory
	if !checkDataDir() {
		allPassed = false
	}
	
	// Check flows
	if !checkFlows() {
		allPassed = false
	}
	
	// Check alerts
	if !checkAlerts() {
		allPassed = false
	}
	
	// Check listener reachability
	if !checkListener() {
		allPassed = false
	}
	
	// Check backend connectivity
	if !checkBackends() {
		allPassed = false
	}
	
	fmt.Println()
	if allPassed {
		fmt.Println("✅ All checks passed!")
		return 0
	} else {
		fmt.Println("⚠️  Some issues found. See above for details.")
		return 1
	}
}

func checkDataDir() bool {
	fmt.Print("📁 Data directory writable... ")
	
	// Try to create incident service
	_, err := incident.NewService(nil)
	if err != nil {
		fmt.Printf("❌\n   %s\n", err.Error())
		return false
	}
	
	// Check canonical path
	canonicalPath := "/var/lib/health-monitor/incidents"
	if info, err := os.Stat(canonicalPath); err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("⚠️  (will be created on first use)\n")
		} else {
			fmt.Printf("❌\n   %s\n", err.Error())
			return false
		}
	} else if !info.IsDir() {
		fmt.Printf("❌\n   Path is not a directory: %s\n", canonicalPath)
		return false
	} else {
		fmt.Printf("✔️  %s\n", canonicalPath)
	}
	
	return true
}

func checkFlows() bool {
	fmt.Print("🌊 Flows loaded... ")
	
	_, err := flow.Load()
	if err != nil {
		fmt.Printf("❌\n   %s\n", err.Error())
		return false
	}
	
	fmt.Printf("✔️  flows loaded\n")
	return true
}

func checkAlerts() bool {
	fmt.Print("🚨 Alerts loaded... ")
	
	result, err := alert.Load()
	if err != nil {
		if alert.IsNoConfig(err) {
			fmt.Printf("⚠️  (no alerts configured)\n")
		} else {
			fmt.Printf("❌\n   %s\n", err.Error())
			return false
		}
	} else {
		fmt.Printf("✔️  %d alert rules loaded\n", len(result.Rules))
	}
	
	return true
}

func checkListener() bool {
	fmt.Print("🎧 Listener reachable... ")
	
	// Try to connect to default listener port
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://127.0.0.1:9095/healthz")
	if err != nil {
		fmt.Printf("⚠️  (listener not running on :9095)\n")
		return false
	}
	defer resp.Body.Close()
	
	if resp.StatusCode == 200 {
		fmt.Printf("✔️  listener reachable\n")
		return true
	}
	
	fmt.Printf("❌\n   HTTP %d\n", resp.StatusCode)
	return false
}

func checkBackends() bool {
	fmt.Println("🔗 Backend connectivity...")
	
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("❌ Config: %s\n", err.Error())
		return false
	}
	
	allGood := true
	
	// Check Prometheus
	if cfg.PrometheusURL != "" {
		fmt.Print("   Prometheus reachable... ")
		if checkURL(cfg.PrometheusURL) {
			fmt.Printf("✔️\n")
		} else {
			fmt.Printf("❌\n")
			allGood = false
		}
	} else {
		fmt.Printf("   ⚠️  Prometheus not configured\n")
	}
	
	// Check Grafana
	if cfg.GrafanaURL != "" {
		fmt.Print("   Grafana reachable... ")
		if checkURL(cfg.GrafanaURL) {
			fmt.Printf("✔️\n")
		} else {
			fmt.Printf("❌\n")
			allGood = false
		}
	} else {
		fmt.Printf("   ⚠️  Grafana not configured\n")
	}
	
	// Check Loki
	if cfg.LokiURL != "" {
		fmt.Print("   Loki reachable... ")
		if checkURL(cfg.LokiURL) {
			fmt.Printf("✔️\n")
		} else {
			fmt.Printf("❌\n")
			allGood = false
		}
	} else if cfg.GrafanaURL != "" && cfg.GrafanaLokiDataSource == "" {
		fmt.Printf("   ⚠️  Loki not configured\n")
	}
	
	// Check Traces
	if cfg.TraceURL != "" {
		fmt.Print("   Traces reachable... ")
		if checkURL(cfg.TraceURL) {
			fmt.Printf("✔️\n")
		} else {
			fmt.Printf("❌\n")
			allGood = false
		}
	} else if cfg.GrafanaURL != "" && cfg.GrafanaTraceDataSource == "" {
		fmt.Printf("   ⚠️  Traces not configured\n")
	}

	// Check Elasticsearch
	if cfg.ElasticURL != "" {
		fmt.Print("   Elasticsearch reachable... ")
		if checkURL(cfg.ElasticURL) {
			fmt.Printf("✔️\n")
		} else {
			fmt.Printf("❌\n")
			allGood = false
		}
	} else {
		fmt.Printf("   ⚠️  Elasticsearch not configured\n")
	}

	return allGood
}

func checkURL(url string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}
