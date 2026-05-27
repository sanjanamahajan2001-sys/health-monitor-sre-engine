package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type LokiResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][]interface{}   `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run loki_diag.go <service_name> [lookback_duration]")
		fmt.Println("Example: go run loki_diag.go frontend 15m")
		os.Exit(1)
	}

	service := os.Args[1]
	lookback := "15m"
	if len(os.Args) > 2 {
		lookback = os.Args[2]
	}
	duration, _ := time.ParseDuration(lookback)

	lokiURL := os.Getenv("LOKI_URL")
	if lokiURL == "" {
		lokiURL = "http://localhost:3100"
	}
	lokiUser := os.Getenv("LOKI_USER")
	lokiPass := os.Getenv("LOKI_PASS")

	end := time.Now().UTC()
	start := end.Add(-duration)

	fmt.Printf("--- Loki Diagnostics for %s ---\n", service)
	fmt.Printf("Time Range: %s to %s (UTC)\n", start.Format("15:04:05"), end.Format("15:04:05"))
	fmt.Printf("Loki URL: %s\n\n", lokiURL)

	// Try common labels
	labels := []string{"container", "job", "app", "service"}
	foundAny := false
	
	// Error regex used by health-monitor
	errorRegex := `(?i)(error|exception|fail|timeout|refused|deadlock|warn|warning|critical|severe|fatal|panic|4\d\d|5\d\d)`
	// LogQL compatibility: replace \d with [0-9] and escape backslashes in quoted strings
	safeErrorRegex := strings.ReplaceAll(errorRegex, "\\d", "[0-9]")
	safeErrorRegex = strings.ReplaceAll(safeErrorRegex, "\\", "\\\\")

	for _, lbl := range labels {
		query := fmt.Sprintf("{%s=~\"%s|%s\"}", lbl, service, strings.ReplaceAll(service, "-", "_"))
		fmt.Printf("Checking Label [%s] Query: %s\n", lbl, query)
		
		results, err := queryLoki(lokiURL, query, start, end, lokiUser, lokiPass)
		if err != nil {
			fmt.Printf("  Error: %v\n", err)
			continue
		}

		if len(results) == 0 {
			fmt.Printf("  Result: 0 lines found.\n")
			continue
		}

		foundAny = true
		fmt.Printf("  Result: %d lines found!\n", len(results))
		fmt.Printf("  --- Log Samples (Showing last 3) ---\n")
		startIdx := 0
		if len(results) > 3 {
			startIdx = len(results) - 3
		}
		for _, line := range results[startIdx:] {
			fmt.Printf("    • %s\n", line)
		}
		
		errorQuery := fmt.Sprintf("%s |~ \"%s\"", query, safeErrorRegex)
		fmt.Printf("  Applying Error Filter Query: %s\n", errorQuery)
		errResults, err := queryLoki(lokiURL, errorQuery, start, end, lokiUser, lokiPass)
		if err != nil {
			fmt.Printf("  Filter Error: %v\n", err)
		} else {
			fmt.Printf("  Filter Result: %d lines matched the error/warn pattern.\n", len(errResults))
			if len(errResults) > 0 {
				fmt.Printf("    Sample match: %s\n", errResults[0])
			}
		}
		fmt.Println()
	}

	if !foundAny {
		fmt.Println("CRITICAL: No logs found for any common labels. Possible causes:")
		fmt.Println("1. Logs haven't reached Loki yet (Ingestion lag). Try again in 30s.")
		fmt.Println("2. Correct label is not in [container, job, app, service]. Check your Loki Explore UI.")
		fmt.Println("3. Service name mismatch. Ensure labels in Loki use the exact service name or underscored variant.")
	}
}

func queryLoki(baseURL, query string, start, end time.Time, user, pass string) ([]string, error) {
	params := url.Values{}
	params.Add("query", query)
	params.Add("start", fmt.Sprintf("%d", start.UnixNano()))
	params.Add("end", fmt.Sprintf("%d", end.UnixNano()))
	params.Add("limit", "100")

	u := fmt.Sprintf("%s/loki/api/v1/query_range?%s", strings.TrimSuffix(baseURL, "/"), params.Encode())
	req, _ := http.NewRequest("GET", u, nil)
	if user != "" {
		req.SetBasicAuth(user, pass)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Loki returned status %d", resp.StatusCode)
	}

	var lr LokiResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, err
	}

	var lines []string
	for _, res := range lr.Data.Result {
		for _, val := range res.Values {
			if len(val) >= 2 {
				lines = append(lines, val[1].(string))
			}
		}
	}
	return lines, nil
}
