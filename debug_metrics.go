package main

import (
	"fmt"
	"os"
	"github.com/prometheus/client_golang/prometheus"
	_ "health-monitor/internal/metrics"
)

func main() {
	fmt.Println("Testing metrics registration...")
	families, _ := prometheus.DefaultGatherer.Gather()
	fmt.Printf("Found %d metric families\n", len(families))
	
	found := false
	for _, f := range families {
		name := f.GetName()
		if name == "health_monitor_incidents_active" {
			fmt.Printf("✅ FOUND: %s\n", name)
			found = true
		}
		if len(name) >= 13 && name[:13] == "health_monitor" {
			fmt.Printf("✅ FOUND: %s\n", name)
			found = true
		}
	}
	
	if !found {
		fmt.Println("❌ NO health_monitor metrics found!")
		os.Exit(1)
	}
}
