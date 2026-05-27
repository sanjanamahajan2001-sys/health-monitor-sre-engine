package tui

import (
	"health-monitor/pkg/model"
	"testing"
)

func TestInitialModel(t *testing.T) {
	report := model.Report{}
	m := initialModel(report)
	if !m.ready {
		t.Errorf("Expected model to be ready")
	}
}
