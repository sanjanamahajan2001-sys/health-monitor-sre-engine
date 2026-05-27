package rules

import (
	"health-monitor/internal/analyse/gpu"
	"health-monitor/pkg/model"
)

func GPUStatus(r gpu.Result) (model.Status, string) {
	if !r.Present {
		return model.SAFE, "No GPU present"
	}
	if r.UtilPct == 0 && r.UsedMB > 500 {
		return model.CHECK, "Possible GPU memory retention while idle"
	}
	if r.TempC >= 80 {
		return model.CHECK, "High GPU temperature"
	}
	return model.SAFE, "GPU operating normally"
}
