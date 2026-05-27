package alert

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sort"

	"health-monitor/internal/config"
	"health-monitor/internal/incident"

	"gopkg.in/yaml.v3"
)

var (
	ErrNoConfig      = errors.New("no alert config found")
	ErrInvalidConfig = errors.New("invalid alert config")
)

type LoadResult struct {
	Rules    []Rule
	Files    []string
	Warnings []string
}

func Load() (LoadResult, error) {
	return LoadFromLocations(defaultLocations())
}

// LoadProfileAware loads alerts using profile-aware paths
func LoadProfileAware() (LoadResult, error) {
	// Try profile-specific path first
	alertsPath := config.GetAlertsPath()
	if alertsPath != "" {
		return LoadFromLocations([]string{alertsPath})
	}
	
	// Fallback to default locations
	return LoadFromLocations(defaultLocations())
}

func LoadFromLocations(locations []string) (LoadResult, error) {
	files, err := expandLocations(locations)
	if err != nil {
		return LoadResult{}, err
	}
	if len(files) == 0 {
		return LoadResult{}, ErrNoConfig
	}
	rules, warnings, err := loadFiles(files)
	if err != nil {
		return LoadResult{Files: files, Warnings: warnings}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	return LoadResult{Rules: rules, Files: files, Warnings: warnings}, nil
}

func IsNoConfig(err error) bool {
	return errors.Is(err, ErrNoConfig)
}

func IsInvalidConfig(err error) bool {
	return errors.Is(err, ErrInvalidConfig)
}

func defaultLocations() []string {
	if override := strings.TrimSpace(os.Getenv("HEALTH_MONITOR_ALERT_PATHS")); override != "" {
		paths := filepath.SplitList(override)
		if len(paths) > 0 {
			return paths
		}
	}
	home, _ := os.UserHomeDir()
	return []string{
		"/etc/health-monitor/alerts.yaml",
		"/etc/health-monitor/alerts.d/*.yaml",
		filepath.Join(home, ".health-monitor", "alerts.yaml"),
		filepath.Join(home, ".health-monitor", "alerts.d", "*.yaml"),
	}
}

func expandLocations(locations []string) ([]string, error) {
	seen := make(map[string]struct{})
	var files []string
	for _, loc := range locations {
		loc = strings.TrimSpace(loc)
		if loc == "" {
			continue
		}
		if hasGlob(loc) {
			matches, err := filepath.Glob(loc)
			if err != nil {
				return nil, err
			}
			for _, match := range matches {
				if _, ok := seen[match]; ok {
					continue
				}
				if info, err := os.Stat(match); err == nil && !info.IsDir() {
					seen[match] = struct{}{}
					files = append(files, match)
				}
			}
			continue
		}
		if _, ok := seen[loc]; ok {
			continue
		}
		if info, err := os.Stat(loc); err == nil {
			if info.IsDir() {
				matches, err := filepath.Glob(filepath.Join(loc, "*.yaml"))
				if err != nil {
					return nil, err
				}
				for _, match := range matches {
					if _, ok := seen[match]; ok {
						continue
					}
					if info, err := os.Stat(match); err == nil && !info.IsDir() {
						seen[match] = struct{}{}
						files = append(files, match)
					}
				}
			} else {
				seen[loc] = struct{}{}
				files = append(files, loc)
			}
		}
	}
	sort.Strings(files)
	return files, nil
}

func hasGlob(value string) bool {
	return strings.ContainsAny(value, "*?[")
}

type alertFile struct {
	Alerts []ruleConfig `yaml:"alerts"`
}

type ruleConfig struct {
	Match    map[string]string `yaml:"match"`
	Severity string            `yaml:"severity"`
	Title    string            `yaml:"title"`
	Mode     string            `yaml:"mode"`
}

func loadFiles(files []string) ([]Rule, []string, error) {
	var rules []Rule
	var warnings []string
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skipping %s: %v", path, err))
			continue
		}
		var parsed alertFile
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			warnings = append(warnings, fmt.Sprintf("skipping %s: invalid yaml: %v", path, err))
			continue
		}
		if len(parsed.Alerts) == 0 {
			warnings = append(warnings, fmt.Sprintf("skipping %s: missing or empty alerts section", path))
			continue
		}
		for idx, cfg := range parsed.Alerts {
			rule, err := normalizeRule(cfg)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("skipping alert rule %d in %s: %v", idx+1, path, err))
				continue
			}
			rule.Source = path
			rules = append(rules, rule)
		}
	}
	if len(rules) == 0 && len(warnings) > 0 {
		return nil, warnings, errors.New("no valid alert rules")
	}
	return rules, warnings, nil
}

func normalizeRule(cfg ruleConfig) (Rule, error) {
	match, err := normalizeMatch(cfg.Match)
	if err != nil {
		return Rule{}, err
	}
	service := match["service"]
	if service == "" {
		service = match["job"] // Fallback for bare-metal/prometheus-style jobs
	}
	if strings.TrimSpace(service) == "" {
		return Rule{}, errors.New("service or job is required in match section")
	}
	// alertname is optional - generic rules can match any alertname for a service
	severity, err := incident.ParseSeverity(cfg.Severity)
	if err != nil {
		return Rule{}, err
	}
	mode := parseMode(cfg.Mode)
	if mode == "" {
		return Rule{}, errors.New("mode must be auto or suggest")
	}
	return Rule{
		Match:    match,
		Severity: severity,
		Title:    strings.TrimSpace(cfg.Title),
		Mode:     mode,
	}, nil
}

func normalizeMatch(input map[string]string) (map[string]string, error) {
	if len(input) == 0 {
		return nil, errors.New("match section is empty")
	}
	clean := make(map[string]string, len(input))
	for key, value := range input {
		trimmedKey := strings.ToLower(strings.TrimSpace(key))
		trimmedValue := strings.TrimSpace(value)
		if trimmedKey == "" {
			return nil, errors.New("match key is empty")
		}
		if trimmedValue == "" {
			return nil, fmt.Errorf("match value for %q is empty", trimmedKey)
		}
		clean[trimmedKey] = trimmedValue
	}
	return clean, nil
}

func parseMode(value string) Mode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return ModeSuggest
	case string(ModeSuggest):
		return ModeSuggest
	case string(ModeAuto):
		return ModeAuto
	default:
		return ""
	}
}
