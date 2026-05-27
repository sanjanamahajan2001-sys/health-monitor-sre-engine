package alert

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"health-monitor/internal/config"
	"health-monitor/pkg/model"
	"gopkg.in/yaml.v3"
)

// AlertConfig matches the user's provided sample YAML structure
type AlertConfig struct {
	Alerts []AlertRule `yaml:"alerts"`
}

type AlertRule struct {
	Match    map[string]string `yaml:"match"`
	Severity string            `yaml:"severity"`
	Mode     string            `yaml:"mode"`
	Title    string            `yaml:"title"`
}

// GenerateDefaultAlerts creates a default alerts.d/<profile>.yaml for a profile
func GenerateDefaultAlerts(profileName string, services []string, metadata map[string]model.ServiceMetadata) error {
	if len(services) == 0 {
		return nil
	}

	pm := config.GetProfileManager()
	alertsDir := filepath.Join(filepath.Dir(pm.GetProfilePath(profileName)), "..", "alerts.d")
	
	// Create alerts.d if it doesn't exist
	if err := os.MkdirAll(alertsDir, 0755); err != nil {
		return err
	}

	alertsPath := filepath.Join(alertsDir, profileName+".yaml")
	
	alertConf := AlertConfig{
		Alerts: []AlertRule{},
	}

	for _, svc := range services {
		label := "service"
		if meta, ok := metadata[svc]; ok && meta.ServiceLabel != "" {
			label = meta.ServiceLabel
		}
		
		// P1 for Degradation (Success Rate / SLO)
		alertConf.Alerts = append(alertConf.Alerts, AlertRule{
			Match: map[string]string{
				label: svc,
			},
			Severity: "P1",
			Mode:     "auto",
			Title:    fmt.Sprintf("%s Service Degradation", strings.Title(strings.ReplaceAll(svc, "_", " "))),
		})
	}

	data, err := yaml.Marshal(alertConf)
	if err != nil {
		return err
	}

	return os.WriteFile(alertsPath, data, 0600)
}
