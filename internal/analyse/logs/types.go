package logs

import (
	"strings"

	"health-monitor/internal/config"
)

type Scope struct {
	Service    string
	Route      string
	Dependency string
}

type SignatureCount struct {
	Signature string
	Count     int
	Percent   float64
}

type CorrelationRequest struct {
	Scope      Scope
	Window     string
	ErrorRegex string
	MinSamples int
	MaxResults int
	MaxLogs    int
}

type CorrelationResult struct {
	Signatures []SignatureCount
	Query      string
	Notes      []string
	Samples    int
	MinSamples int
}

type LogBackend interface {
	Name() string
	Correlate(req CorrelationRequest) (CorrelationResult, error)
}

func BackendForConfig(cfg config.Config) LogBackend {
	backend := strings.TrimSpace(strings.ToLower(cfg.LogBackend))
	switch backend {
	case "elastic", "opensearch":
		return ElasticBackend{Config: cfg}
	case "loki":
		if cfg.LokiURL != "" {
			return LokiBackend{Config: cfg}
		}
		return nil
	}
	if cfg.LokiURL != "" {
		return LokiBackend{Config: cfg}
	}
	return nil
}

func DefaultMinSamples() int {
	return defaultMinSamples
}
