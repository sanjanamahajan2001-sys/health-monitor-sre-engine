package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"health-monitor/internal/config"
	"health-monitor/pkg/model"
)

// EmptyOr returns the value if not empty, otherwise the fallback
func EmptyOr(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func FormatDeltaLatency(delta float64) string {
	if math.IsNaN(delta) {
		return "N/A"
	}
	prefix := ""
	if delta > 0 {
		prefix = "+"
	} else if delta < 0 {
		prefix = "-"
	}
	return prefix + FormatLatency(math.Abs(delta))
}

func FormatDeltaPercent(delta float64) string {
	if math.IsNaN(delta) {
		return "N/A"
	}
	prefix := ""
	if delta > 0 {
		prefix = "+"
	} else if delta < 0 {
		prefix = "-"
	}
	return prefix + fmt.Sprintf("%.2f%%", math.Abs(delta))
}

func FormatDeltaNumber(delta float64) string {
	if math.IsNaN(delta) {
		return "N/A"
	}
	prefix := ""
	if delta > 0 {
		prefix = "+"
	} else if delta < 0 {
		prefix = "-"
	}
	return prefix + fmt.Sprintf("%.2f", math.Abs(delta))
}

func PromP95Query(latency *model.APILatency) string {
	if latency == nil || latency.LatencyMetric == "" {
		return ""
	}
	labels := latency.LabelSelector
	if labels == "" {
		labels = "{}"
	}
	return fmt.Sprintf(`histogram_quantile(0.95, sum(rate(%s_bucket%s[%s])) by (le))`, latency.LatencyMetric, labels, latency.Window)
}

func BuildGrafanaExploreURL(baseURL string, datasource string, datasourceUID string, datasourceType string, query string, from string, to string) string {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(query) == "" {
		return ""
	}
	
	type grafanaQuery struct {
		RefId string `json:"refId"`
		Expr  string `json:"expr"`
	}
	type grafanaRange struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	type grafanaLeft struct {
		Datasource interface{}    `json:"datasource"`
		Queries    []grafanaQuery `json:"queries"`
		Range      grafanaRange   `json:"range"`
	}

	var ds interface{}
	if strings.TrimSpace(datasourceUID) != "" && strings.TrimSpace(datasourceType) != "" {
		ds = map[string]string{
			"uid":  datasourceUID,
			"type": datasourceType,
		}
	} else if strings.TrimSpace(datasource) != "" {
		ds = datasource
	} else {
		if strings.TrimSpace(datasource) != "" {
			ds = datasource
		} else {
			return ""
		}
	}

	left := grafanaLeft{
		Datasource: ds,
		Queries:    []grafanaQuery{{RefId: "A", Expr: query}},
		Range:      grafanaRange{From: from, To: to},
	}

	leftJSON, err := json.Marshal(left)
	if err != nil {
		return ""
	}

	return strings.TrimSuffix(baseURL, "/") + "/explore?left=" + QueryEscapeStrict(string(leftJSON))
}

func QueryEscapeStrict(value string) string {
	escaped := url.QueryEscape(value)
	return strings.ReplaceAll(escaped, "+", "%20")
}

func SanitizeLogLine(value string) string {
	v := strings.ReplaceAll(value, "\n", " ")
	v = strings.ReplaceAll(v, "\r", "")
	v = strings.ReplaceAll(v, "\t", " ")
	return strings.TrimSpace(v)
}

func EscapeLogQLString(val string) string {
	v := strings.ReplaceAll(val, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	return v
}

func NormalizeLokiRegex(value string) string {
	if strings.TrimSpace(value) == "" {
		return value
	}
	return strings.ReplaceAll(value, "\\\\", "\\")
}

func SanitizeLokiRegex(value string) (string, bool) {
	if strings.TrimSpace(value) == "" {
		return value, false
	}
	sanitized := strings.ReplaceAll(value, `\d`, `[0-9]`)
	if sanitized != value {
		return sanitized, true
	}
	return value, false
}

func ValidateLokiRegex(value string) (string, string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return value, ""
	}
	if len(trimmed) > 256 {
		return "", "Loki regex ignored: too long"
	}
	if _, err := regexp.Compile(trimmed); err != nil {
		return "", "Loki regex ignored: invalid regex"
	}
	return trimmed, ""
}

func FormatLatency(value float64) string {
	if math.IsNaN(value) {
		return "N/A"
	}
	if value < 0.001 {
		return fmt.Sprintf("%.2fµs", value*1000000)
	}
	if value < 1.0 {
		return fmt.Sprintf("%.2fms", value*1000)
	}
	return fmt.Sprintf("%.2fs", value)
}

func FormatPercent(value float64) string {
	if math.IsNaN(value) {
		return "N/A"
	}
	return fmt.Sprintf("%.2f%%", value)
}

func FetchGrafanaDatasource(cfg config.Config, name string) (string, string, error) {
	baseURL := strings.TrimSuffix(cfg.GrafanaURL, "/")
	if baseURL == "" || name == "" {
		return "", "", errors.New("grafana url or datasource name missing")
	}
	req, err := http.NewRequest(http.MethodGet, baseURL+"/api/datasources/name/"+url.PathEscape(name), nil)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(cfg.GrafanaToken) != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.GrafanaToken)
	}
	if strings.TrimSpace(cfg.GrafanaUser) != "" || strings.TrimSpace(cfg.GrafanaPass) != "" {
		req.SetBasicAuth(cfg.GrafanaUser, cfg.GrafanaPass)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("grafana datasource lookup failed: %s", resp.Status)
	}
	var payload struct {
		UID  string `json:"uid"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", "", err
	}
	if payload.UID == "" || payload.Type == "" {
		return "", "", errors.New("grafana datasource response missing uid/type")
	}
	return payload.UID, payload.Type, nil
}
