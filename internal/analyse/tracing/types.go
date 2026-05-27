package tracing

import "time"

type Backend string

const (
	BackendTempo  Backend = "tempo"
	BackendJaeger Backend = "jaeger"
)

const MaxSpansPerTrace = 2000

type Client struct {
	BaseURL string
	Token   string
	User    string
	Pass    string
	Timeout time.Duration
	QPS     float64
	Backend Backend
}

type Trace struct {
	TraceID     string
	Duration    time.Duration
	RootSpan    Span
	SlowestSpan Span
	HasDBSpan   bool
	SpanCount   int
	Truncated   bool
}

type Span struct {
	SpanID      string
	ParentID    string
	Service     string
	PeerService string
	Name        string
	Duration    time.Duration
	Start       time.Time
	IsDB        bool
}
