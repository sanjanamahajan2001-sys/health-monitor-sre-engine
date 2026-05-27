package commands

import (
	"fmt"
	"os"

	"health-monitor/internal/guide"
	"health-monitor/internal/incident"
)

// GuideSeeder implements the guide.Seeder interface
type GuideSeeder struct {
	service *incident.Service
}

func (s *GuideSeeder) Seed(scenario string) error {
	if s.service == nil {
		// Initialize the sandbox on demand if it hasn't been yet
		sandboxDir, err := CreateTempSandbox()
		if err != nil {
			return err
		}
		
		// Set env vars for the sandbox
		os.Setenv("HEALTH_MONITOR_PROFILE", "demo-guide")
		
		store, err := incident.NewStore()
		if err != nil {
			return err
		}
		
		s.service, err = incident.NewService(store)
		if err != nil {
			return err
		}

		// Initial seed
		_ = SeedDemoData(store, sandboxDir)
	}

	return SeedScenarioData(s.service, scenario)
}

// HandleGuideCommand launches the interactive guided tour
func HandleGuideCommand(args []string) int {
	seeder := &GuideSeeder{}
	model, err := guide.NewUnifiedGuideModel(seeder)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing guide: %v\n", err)
		return 1
	}

	if err := guide.Run(model); err != nil {
		fmt.Fprintf(os.Stderr, "Error running guide: %v\n", err)
		return 1
	}

	return 0
}
