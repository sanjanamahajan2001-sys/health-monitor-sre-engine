package flow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"health-monitor/internal/config"
	"gopkg.in/yaml.v3"
)

var (
	ErrNoConfig      = errors.New("no flow config found")
	ErrInvalidConfig = errors.New("invalid flow config")
)

var (
	loadOncePerProfile     = make(map[string]*sync.Once)
	cachedResultPerProfile = make(map[string]LoadResult)
	cachedErrPerProfile    = make(map[string]error)
	cacheMutex             sync.RWMutex
)

func InvalidateCache() {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	
	activeProfile := config.GetProfileManager().GetActiveProfile()
	delete(loadOncePerProfile, activeProfile)
	delete(cachedResultPerProfile, activeProfile)
	delete(cachedErrPerProfile, activeProfile)
}

// StartWatcher starts a polling watcher that invalidates the flow cache on file changes.
func StartWatcher(ctx context.Context, interval time.Duration, onChange func()) {
	go func() {
		lastManifest := make(map[string]time.Time)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Only watch the profile-specific path if it exists
				flowsPath := config.GetFlowsPath()
				locations := []string{flowsPath}
				if flowsPath == "" {
					locations = defaultLocations()
				}
				
				files, err := expandLocations(locations)
				if err != nil {
					continue
				}

				changed := false
				newManifest := make(map[string]time.Time)
				for _, f := range files {
					info, err := os.Stat(f)
					if err != nil {
						continue
					}
					newManifest[f] = info.ModTime()
					if lastTime, ok := lastManifest[f]; !ok || !info.ModTime().Equal(lastTime) {
						changed = true
					}
				}

				// Check for deletions
				if len(newManifest) != len(lastManifest) && len(lastManifest) > 0 {
					changed = true
				}

				if changed {
					lastManifest = newManifest
					InvalidateCache()
					if onChange != nil {
						onChange()
					}
				}
			}
		}
	}()
}

func LoadOnce() (LoadResult, error) {
	pm := config.GetProfileManager()
	activeProfile := pm.GetActiveProfile()
	
	cacheMutex.RLock()
	once, exists := loadOncePerProfile[activeProfile]
	cacheMutex.RUnlock()
	
	if !exists {
		cacheMutex.Lock()
		once = &sync.Once{}
		loadOncePerProfile[activeProfile] = once
		cacheMutex.Unlock()
	}
	
	once.Do(func() {
		result, err := LoadProfileAware()
		cacheMutex.Lock()
		cachedResultPerProfile[activeProfile] = result
		cachedErrPerProfile[activeProfile] = err
		cacheMutex.Unlock()
	})
	
	cacheMutex.RLock()
	defer cacheMutex.RUnlock()
	
	return cachedResultPerProfile[activeProfile], cachedErrPerProfile[activeProfile]
}

func ResetCacheForTest() {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	
	loadOncePerProfile = make(map[string]*sync.Once)
	cachedResultPerProfile = make(map[string]LoadResult)
	cachedErrPerProfile = make(map[string]error)
}

func Load() (LoadResult, error) {
	return LoadFromLocations(defaultLocations())
}

// LoadProfileAware loads flows using profile-aware paths (defaults to active profile)
func LoadProfileAware() (LoadResult, error) {
	return LoadForProfile(config.GetProfileManager().GetActiveProfile())
}

// LoadForProfile loads flows specifically for a given profile
func LoadForProfile(profileName string) (LoadResult, error) {
	pm := config.GetProfileManager()
	
	// 1. Try profile-specific path from ProfileManager
	flowsPath := pm.GetFlowsPathForProfile(profileName)
	if flowsPath != "" {
		// If it's the base flows.yaml, we should still search flows.d if it's default
		// but if GetFlowsPathForProfile returns a specific file, we load just that.
		if !strings.HasSuffix(flowsPath, "flows.yaml") {
			return LoadFromLocations([]string{flowsPath})
		}
	}

	// 2. Build locations list similar to defaultLocations() but for specific profile
	statePath := pm.GetStatePathForProfile(profileName)
	locations := []string{
		filepath.Join(pm.GetConfigBasePath(), "profiles", profileName+"-flows.yaml"),
		filepath.Join(pm.GetConfigBasePath(), "flows.d", profileName+".yaml"),
		filepath.Join(statePath, "flows.yaml"),
		filepath.Join(statePath, "flows.d", "*.yaml"),
		filepath.Join(pm.GetConfigBasePath(), "flows.yaml"),
		filepath.Join(pm.GetConfigBasePath(), "flows.d", "*.yaml"),
	}

	return LoadFromLocations(locations)
}

func LoadFromLocations(locations []string) (LoadResult, error) {
	files, err := expandLocations(locations)
	if err != nil {
		return LoadResult{}, err
	}
	if len(files) == 0 {
		return LoadResult{}, ErrNoConfig
	}
	flows, warnings, err := loadFiles(files)
	if err != nil {
		return LoadResult{Files: files, Warnings: warnings}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	return LoadResult{Flows: flows, Files: files, Warnings: warnings}, nil
}

func IsNoConfig(err error) bool {
	return errors.Is(err, ErrNoConfig)
}

func IsInvalidConfig(err error) bool {
	return errors.Is(err, ErrInvalidConfig)
}

func defaultLocations() []string {
	pm := config.GetProfileManager()
	activeProfile := pm.GetActiveProfile()
	
	// Check for profile-specific override first
	if override := strings.TrimSpace(os.Getenv("HEALTH_MONITOR_PROFILES__" + strings.ToUpper(activeProfile) + "__FLOW_PATHS")); override != "" {
		paths := filepath.SplitList(override)
		if len(paths) > 0 {
			return paths
		}
	}
	
	// Check for global override
	if override := strings.TrimSpace(os.Getenv("HEALTH_MONITOR_FLOW_PATHS")); override != "" {
		paths := filepath.SplitList(override)
		if len(paths) > 0 {
			return paths
		}
	}
	
	profileStateDir := pm.GetStatePath()
	
	// Base locations to check
	baseDirs := []string{
		pm.GetConfigBasePath(),
	}
	
	var locations []string
	
	// If a profile is active, look for profile-specific files first in all base dirs
	if activeProfile != "" && activeProfile != "default" {
		for _, dir := range baseDirs {
			// Profile-specific flows.yaml or inside flows.d
			locations = append(locations,
				filepath.Join(dir, "profiles", activeProfile+"-flows.yaml"),
				filepath.Join(dir, "flows.d", activeProfile+".yaml"),
			)
		}
	}
	
	// Add profile-specific state path locations
	if activeProfile != "default" {
		locations = append(locations,
			filepath.Join(profileStateDir, "flows.yaml"),
			filepath.Join(profileStateDir, "flows.d", "*.yaml"),
		)
	}

	// Always allow global flows.yaml
	for _, dir := range baseDirs {
		locations = append(locations, filepath.Join(dir, "flows.yaml"))
	}
	
	// If NO profile specific flows were found (we check this during expansion), 
	// we only then want the wildcard. But loader expansion happens later.
	// For now, let's keep the wildcard as last resort.
	for _, dir := range baseDirs {
		locations = append(locations, filepath.Join(dir, "flows.d", "*.yaml"))
	}
	
	return locations
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

type flowFile struct {
	Flows map[string]flowConfig `yaml:"flows"`
}

type flowConfig struct {
	Name     string      `yaml:"name"`
	Services []string    `yaml:"services"`
	SLOs     []sloConfig `yaml:"slos"`
}

type sloConfig struct {
	ID               string  `yaml:"id"`
	Service          string  `yaml:"service"`
	Objective        float64 `yaml:"objective"`
	Window           string  `yaml:"window"`
	PromQL           string  `yaml:"promql"`
	Type             string  `yaml:"type"`
	ErrorQuery       string  `yaml:"error_query"`
	TotalQuery       string  `yaml:"total_query"`
	ThresholdSeconds float64 `yaml:"threshold_seconds"`
	
	// Legacy aliases for backward compatibility with older generated files
	LatencyQuery string `yaml:"latency_query"` 
	Threshold    float64 `yaml:"threshold"`
}

func loadFiles(files []string) ([]Flow, []string, error) {
	flowsByID := make(map[string]Flow)
	var warnings []string
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			msg := fmt.Sprintf("skipping %s: %v", path, err)
			if os.IsPermission(err) {
				msg += " (hint: try running with sudo)"
			}
			warnings = append(warnings, msg)
			continue
		}
		var parsed flowFile
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			warnings = append(warnings, fmt.Sprintf("skipping %s: invalid yaml", path))
			continue
		}
		if len(parsed.Flows) == 0 {
			warnings = append(warnings, fmt.Sprintf("skipping %s: missing or empty flows section", path))
			continue
		}
		for id, cfg := range parsed.Flows {
			id = strings.TrimSpace(id)
			if id == "" {
				warnings = append(warnings, fmt.Sprintf("skipping empty flow id in %s", path))
				continue
			}
			if _, exists := flowsByID[id]; exists {
				warnings = append(warnings, fmt.Sprintf("skipping duplicate flow id %q in %s", id, path))
				continue
			}
			services, err := normalizeServices(cfg.Services)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("skipping flow %q in %s: %v", id, path, err))
				continue
			}
			slos := make([]SLO, 0, len(cfg.SLOs))
			for _, s := range cfg.SLOs {
				svc := strings.TrimSpace(s.Service)
				if svc == "" && len(services) > 0 {
					svc = services[0]
				}
				promql := strings.TrimSpace(s.PromQL)
				if promql == "" && s.LatencyQuery != "" {
					promql = strings.TrimSpace(s.LatencyQuery)
				}
				
				threshold := s.ThresholdSeconds
				if threshold == 0 && s.Threshold != 0 {
					threshold = s.Threshold
				}

				// Validation
				window := strings.TrimSpace(s.Window)
				if window != "" {
					if _, err := time.ParseDuration(window); err != nil {
						warnings = append(warnings, fmt.Sprintf("flow %q SLO %q: invalid window %q", id, s.ID, window))
					}
				}
				if s.Objective < 0 || s.Objective > 100 {
					warnings = append(warnings, fmt.Sprintf("flow %q SLO %q: objective must be 0-100 (got %f)", id, s.ID, s.Objective))
				}

				slos = append(slos, SLO{
					ID:               strings.TrimSpace(s.ID),
					Service:          svc,
					Objective:        s.Objective,
					Window:           window,
					PromQL:           promql,
					Type:             strings.TrimSpace(s.Type),
					ErrorQuery:       strings.TrimSpace(s.ErrorQuery),
					TotalQuery:       strings.TrimSpace(s.TotalQuery),
					ThresholdSeconds: threshold,
				})
			}

			flowsByID[id] = Flow{
				ID:       id,
				Name:     strings.TrimSpace(cfg.Name),
				Services: services,
				SLOs:     slos,
			}
		}
	}
	flows := make([]Flow, 0, len(flowsByID))
	for _, flow := range flowsByID {
		flows = append(flows, flow)
	}
	sort.Slice(flows, func(i, j int) bool {
		return flows[i].ID < flows[j].ID
	})
	if len(flows) == 0 && len(warnings) > 0 {
		return nil, warnings, errors.New("no valid flow entries")
	}
	return flows, warnings, nil
}

func normalizeServices(services []string) ([]string, error) {
	if len(services) == 0 {
		return nil, errors.New("services list is empty")
	}
	seen := make(map[string]struct{})
	var cleaned []string
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service == "" {
			return nil, errors.New("service name is empty")
		}
		if _, ok := seen[service]; ok {
			continue
		}
		seen[service] = struct{}{}
		cleaned = append(cleaned, service)
	}
	if len(cleaned) == 0 {
		return nil, errors.New("services list is empty")
	}
	sort.Strings(cleaned)
	return cleaned, nil
}
