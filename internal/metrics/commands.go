package metrics

import (
	"encoding/json"
	"fmt"
	"time"
)

func HandleMetrics(args []string) int {
	if len(args) > 0 && args[0] == "reset" {
		GetQueryMetrics().Reset()
		fmt.Println("✓ Query metrics reset")
		return 0
	}

	stats := GetQueryMetrics().GetStats()
	
	// Pretty print metrics
	fmt.Printf("📊 Query Performance Metrics\n\n")
	
	fmt.Printf("🔍 Loki Queries:\n")
	fmt.Printf("  Total: %d\n", stats.LokiQueryCount)
	fmt.Printf("  Errors: %d\n", stats.LokiErrorCount)
	fmt.Printf("  Avg Duration: %v\n", time.Duration(int64(stats.LokiQueryDuration)/max(stats.LokiQueryCount, 1)))
	fmt.Printf("  Success Rate: %.1f%%\n", float64(stats.LokiQueryCount-stats.LokiErrorCount)/float64(max(stats.LokiQueryCount, 1))*100)
	
	fmt.Printf("\n📈 Prometheus Queries:\n")
	fmt.Printf("  Total: %d\n", stats.PrometheusQueryCount)
	fmt.Printf("  Errors: %d\n", stats.PrometheusErrorCount)
	fmt.Printf("  Avg Duration: %v\n", time.Duration(int64(stats.PrometheusQueryDuration)/max(stats.PrometheusQueryCount, 1)))
	fmt.Printf("  Success Rate: %.1f%%\n", float64(stats.PrometheusQueryCount-stats.PrometheusErrorCount)/float64(max(stats.PrometheusQueryCount, 1))*100)
	
	fmt.Printf("\n📋 Overall:\n")
	fmt.Printf("  Total Queries: %d\n", stats.TotalQueries)
	fmt.Printf("  Total Errors: %d\n", stats.TotalErrors)
	fmt.Printf("  Average Latency: %v\n", stats.AverageLatency)
	fmt.Printf("  Last Query: %v\n", stats.LastQueryTime.Format("2006-01-02 15:04:05"))
	
	if len(args) > 0 && args[0] == "json" {
		fmt.Printf("\n📄 JSON Output:\n")
		if data, err := json.MarshalIndent(stats, "", "  "); err == nil {
			fmt.Println(string(data))
		}
	}
	
	return 0
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
