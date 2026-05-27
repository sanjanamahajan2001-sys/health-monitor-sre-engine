package model

import (
	"time"
)

type Status string

const (
	SAFE    Status = "SAFE"
	CHECK   Status = "CHECK"
	RISK    Status = "RISK"
	UNKNOWN Status = "UNKNOWN"
)

/*
Single top-level report
*/
type Report struct {
	Summary               []SummaryItem
	Metrics               []Metric
	Vars                  []VarItem
	Cleanup               []string
	APILatency            *APILatency
	APINote               string
	APIConfig             *APIConfigSummary
	APM                   *DependencyAnalysis
	Tracing               *TraceSummary
	CorrelationSummary    *CorrelationSummary
	CorrelationAlternates []CorrelationSummary
	InfraStatus           *InfraStatus
	ServiceDrilldown      *ServiceDrilldown
	AllServices           []ServiceOverview
	IsDemo                bool
}

/*
Infrastructure health (Kubernetes/EKS)
*/
type InfraStatus struct {
	ClusterName      string
	Provider         string // eks, kubernetes
	Region           string
	NodesReady       int
	NodesTotal       int
	PodsReady        int
	PodsTotal        int
	Restarts         int
	NamespaceHealth  []NamespaceHealth
	WorkloadHealth   []WorkloadHealth
	ClusterMetrics   *ClusterMetrics
	NoDataReasons    []string
}

type ClusterMetrics struct {
	CPUUsage          float64 // Percentage (0-100)
	MemoryUsage       float64 // Percentage
	DiskUsage         float64 // Percentage
	DiskSizeTotalGB   float64
	DiskSizeUsedGB    float64
	NetworkInRate     float64 // KB/s
	NetworkOutRate    float64 // KB/s
	PodsTotal         int
	PodsReady         int
	Restarts          int
	NodesTotal        int
	NodesReady        int
	ActiveIncidents   int
	TopNamespaces     []NamespaceUsage
}

type NamespaceUsage struct {
	Name        string
	CPUUsage    float64
	MemoryUsage float64
}

type NamespaceHealth struct {
	Name       string
	PodsReady  int
	PodsTotal  int
	Restarts   int
	Status     Status
}

type WorkloadHealth struct {
	Name       string
	Namespace  string
	Type       string // Deployment, StatefulSet, etc.
	Ready      int
	Desired    int
	Restarts   int
	Status     Status
}

/*
Top summary row (DISK / MEMORY / GPU / SSH)
*/
type SummaryItem struct {
	Name   string
	Status Status
	Value  string
}

/*
Metric table rows
*/
type Metric struct {
	Name  string
	Value string
}

/*
/var usage rows
*/
type VarItem struct {
	Size int // in MB
	Path string
}

/*
API latency snapshot from Prometheus
*/
type APILatency struct {
	Service                string
	Route                  string
	Window                 string
	P90                    float64
	P95                    float64
	P99                    float64
	BaselineWindow         string
	BaselineP95            float64
	BaselineErrorRate      float64
	BaselineRPS            float64
	DeltaP95               float64
	DeltaErrorRate         float64
	DeltaRPS               float64
	ErrorRate              float64
	ClientErrorRate        float64
	ServerErrorRate        float64
	ErrorLabel             string
	RPS                    float64
	Status                 Status
	Note                   string
	Confidence             string
	TopEndpointsLimit      int
	TopEndpoints           []EndpointLatency
	TopEndpointsRPS        []EndpointRate
	TopEndpointsRPSNote    string
	TopServicesRPS         []ServiceRate
	TopServicesRPSNote     string
	ActiveAlerts           []string
	TopEndpointRegressions []EndpointRegression
	RegressionNote         string
	SpikeNote              string
	LogSamples             []string
	LogNote                string
	NoDataReasons          []NoDataReason
	RemediationHints       []string
	LokiCorrelationHints   []string
	LatencyMetric          string
	RequestMetric          string
	ServiceLabel           string
	RouteLabel             string
	LabelSelector          string
}

type ServiceDrilldown struct {
	Service           string
	Namespace         string
	GoldenSignals     GoldenSignals
	SLOs              []SLOResult
	RecentDeployments []DeploymentEvent
	CorrelatedLogs    []string
	TraceLinks        []string
}

type ServiceOverview struct {
	Name      string
	Namespace string
	Status    Status
	RPS       float64
	ErrorRate float64
	P95       float64
}

type GoldenSignals struct {
	RequestsPerSecond float64
	ErrorRate         float64
	LatencyP95        float64
	LatencyBaseline   float64
}

type SLOResult struct {
	Name      string
	Target    float64
	Current   float64
	Status    Status
	Remaining float64 // Error budget remaining
}

type DeploymentEvent struct {
	Time    time.Time
	Version string
	Status  string
}

/*
Trace-aware diagnostics summary
*/
type TraceSummary struct {
	Service       string
	Purpose       string
	Window        string
	TopSlowTraces []TraceItem
	Links         []TraceLink
	Note          string
	Backend       string
	BackendURL    string
}

type TraceItem struct {
	TraceID       string
	TotalDuration string
	RootOperation string
	SlowestSpan   TraceSpanSummary
}

type TraceSpanSummary struct {
	Service   string
	Operation string
	Duration  string
}

type TraceLink struct {
	Label string
	URL   string
}

/*
Correlated log summary for anomaly investigation
*/
type CorrelationSummary struct {
	Backend       string
	Window        string
	Query         string
	Notes         []string
	NoDataReasons []string
	Samples       int
	MinSamples    int
	Confidence    string
	Scope         CorrelationScope
	Signatures    []CorrelationSignature
}

type CorrelationScope struct {
	Service    string
	Route      string
	Dependency string
}

type CorrelationSignature struct {
	Signature string
	Count     int
	Percent   float64
}

/*
Per-endpoint latency snapshot
*/
type EndpointLatency struct {
	Route string
	P95   float64
}

type EndpointRegression struct {
	Route       string
	NowP95      float64
	BaselineP95 float64
	DeltaP95    float64
}

type EndpointRate struct {
	Route string
	RPS   float64
}

type ServiceRate struct {
	Service string
	RPS     float64
}

type NoDataReason struct {
	Area         string
	Reason       string
	Metric       string
	Labels       string
	Window       string
	SuggestedFix string
}

type DependencyAnalysis struct {
	Edges                 []DependencyEdge
	RankedCauses          []DependencyCause
	SuspectedUpstream     string
	CorrelationConfidence string
	Evidence              []string
	ReasoningSteps        []string
	NextChecks            []string
	PossibleFixes         []string
	NoDataReasons         []NoDataReason
	Note                  string
	Window                string
	Metric                string
	SourceLabel           string
	DestinationLabel      string
	RouteLabel            string
}

type DependencyEdge struct {
	Source        string
	Destination   string
	Route         string
	P95           float64
	BaselineP95   float64
	DeltaP95      float64
	RPS           float64
	ErrorRate     float64
	ConfidenceTag string
}

type DependencyCause struct {
	Path       string
	Score      float64
	Confidence string
}

/*
API config snapshot for help/config views
*/
type APIConfigSummary struct {
	ActiveConfigPath string
	SystemConfigPath string
	UserConfigPath   string
	TokenFilePath    string
	TokenSource      string
	DisableUpdates   bool
	AutoDiscover     bool
	PrometheusURL    string
	APIService       string
	APIRoute         string
	AuthMode         string
	HasToken         bool
	HasUserPass      bool
	Window           string
	LatencyMetric    string
	RequestMetric    string
	ServiceLabel     string
	RouteLabel       string
	ErrorLabel       string
	ErrorRegex       string
	ExtraSelectors   string
}
