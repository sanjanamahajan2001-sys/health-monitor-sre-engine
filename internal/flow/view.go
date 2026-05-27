package flow

// Legacy plain renderers are in cli_view.go (FormatFlowListPlain/FormatFlowViewPlain)

type IncidentSummary struct {
	ID       string
	Severity string
	Title    string
	Service  string
	State    string
}

func FormatFlowList(flows []Flow, degraded map[string]bool, counts map[string]int) string {
	return FormatFlowListPlain(flows, degraded, counts)
}

func FormatFlowView(flow Flow, incidents []IncidentSummary, degraded bool) string {
	return FormatFlowViewPlain(flow, incidents, degraded)
}

// JSON helpers are in cli_view.go (FormatFlowViewJSON/FormatFlowListJSON).
