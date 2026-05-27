package model

// ServiceMetadata stores discovered info for a service to help generate better flows
type ServiceMetadata struct {
	LatencyMetric string  `json:"latency_metric,omitempty" yaml:"latency_metric,omitempty"`
	RequestMetric string  `json:"request_metric,omitempty" yaml:"request_metric,omitempty"`
	ErrorLabel    string  `json:"error_label,omitempty" yaml:"error_label,omitempty"`
	P95Baseline   float64 `json:"p95_baseline,omitempty" yaml:"p95_baseline,omitempty"`
	P99Baseline   float64 `json:"p99_baseline,omitempty" yaml:"p99_baseline,omitempty"`
	SuccessRate   float64 `json:"success_rate,omitempty" yaml:"success_rate,omitempty"`
	RPS           float64 `json:"rps,omitempty" yaml:"rps,omitempty"`
	ServiceLabel  string  `json:"service_label,omitempty" yaml:"service_label,omitempty"`
	MetricSchema  string  `json:"metric_schema,omitempty" yaml:"metric_schema,omitempty"`
}
