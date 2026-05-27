package logs

import (
	"testing"

	"health-monitor/internal/config"
)

func TestBackendForConfigUsesElastic(t *testing.T) {
	cfg := config.Default()
	cfg.LogBackend = "elastic"
	cfg.ElasticURL = "http://elastic:9200"
	cfg.ElasticIndex = "logs-*"
	backend := BackendForConfig(cfg)
	if backend == nil || backend.Name() != "elastic" {
		t.Fatalf("expected elastic backend, got %#v", backend)
	}
}

func TestBackendForConfigDefaultsToLoki(t *testing.T) {
	cfg := config.Default()
	cfg.LokiURL = "http://loki:3100"
	backend := BackendForConfig(cfg)
	if backend == nil || backend.Name() != "loki" {
		t.Fatalf("expected loki backend, got %#v", backend)
	}
}
