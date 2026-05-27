package flow

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"health-monitor/pkg/model"
)

type ActiveIncidentProvider func() ([]IncidentSummary, error)

var activeIncidentProvider ActiveIncidentProvider

func SetActiveIncidentProvider(provider ActiveIncidentProvider) {
	activeIncidentProvider = provider
}

func HandleCLI(args []string) int {
	if len(args) == 0 {
		printUsage()
		return 1
	}
	if isHelpArg(args[0]) {
		printUsage()
		return 0
	}
	switch args[0] {
	case "list":
		return handleList(args[1:])
	case "validate":
		return handleValidate(args[1:])
	default:
		return handleView(args)
	}
}

func handleValidate(args []string) int {
	fs := flag.NewFlagSet("flow validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	result, err := LoadOnce()
	if err != nil {
		printLoadError(err)
		return 1
	}
	for _, warning := range result.Warnings {
		fmt.Fprintln(os.Stderr, "Warning:", warning)
	}
	if len(result.Flows) == 0 {
		fmt.Print("No flows configured.\n")
		return 0
	}
	fmt.Printf("Flow config OK (%d flow(s) from %d file(s))\n", len(result.Flows), len(result.Files))
	return 0
}

func handleList(args []string) int {
	fs := flag.NewFlagSet("flow list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOutput := fs.Bool("json", false, "output json")
	flagArgs, positionals := splitArgs(args)
	if len(positionals) > 0 {
		fmt.Fprintln(os.Stderr, "Unknown arguments:", strings.Join(positionals, " "))
		return 1
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 1
	}
	result, err := LoadOnce()
	if err != nil {
		printLoadError(err)
		return 1
	}
	for _, warning := range result.Warnings {
		fmt.Fprintln(os.Stderr, "Warning:", warning)
	}
	if len(result.Flows) == 0 {
		fmt.Print("Flow not found\n")
		return 0
	}
	active, err := fetchActiveIncidents()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to load incidents:", err.Error())
		return 1
	}
	flows := make([]Flow, len(result.Flows))
	copy(flows, result.Flows)
	sort.Slice(flows, func(i, j int) bool {
		return flows[i].ID < flows[j].ID
	})
	activeServices := activeServiceSet(active)
	counts := flowIncidentCounts(flows, active)
	degraded := make(map[string]bool, len(result.Flows))
	for _, flow := range flows {
		degraded[flow.ID] = IsDegraded(flow, activeServices)
	}
	if *jsonOutput {
		payload, err := FormatFlowListJSON(flows, degraded, counts)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to render flow list:", err.Error())
			return 1
		}
		fmt.Println(payload)
		return 0
	}
	fmt.Print(FormatFlowListCLI(flows, degraded, counts))
	return 0
}

func handleView(args []string) int {
	fs := flag.NewFlagSet("flow view", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOutput := fs.Bool("json", false, "output json")
	flagArgs, positionals := splitArgs(args)
	if err := fs.Parse(flagArgs); err != nil {
		return 1
	}
	id := strings.TrimSpace(strings.Join(positionals, " "))
	if id == "" {
		fmt.Fprintln(os.Stderr, "Flow not found")
		return 1
	}
	result, err := LoadOnce()
	if err != nil {
		printLoadError(err)
		return 1
	}
	for _, warning := range result.Warnings {
		fmt.Fprintln(os.Stderr, "Warning:", warning)
	}
	if len(result.Flows) == 0 {
		fmt.Print("No flows configured.\n")
		return 0
	}
	flows := make([]Flow, len(result.Flows))
	copy(flows, result.Flows)
	sort.Slice(flows, func(i, j int) bool {
		return flows[i].ID < flows[j].ID
	})
	catalog := NewCatalog(flows)
	flow, ok := catalog.Get(id)
	if !ok {
		fmt.Fprintln(os.Stderr, "Flow not found")
		return 1
	}
	active, err := fetchActiveIncidents()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to load incidents:", err.Error())
		return 1
	}
	activeServices := activeServiceSet(active)
	degraded := IsDegraded(flow, activeServices)
	incidents := filterIncidentsForFlow(active, flow)
	if *jsonOutput {
		payload, err := FormatFlowViewJSON(flow, incidents, degraded)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Failed to render flow:", err.Error())
			return 1
		}
		fmt.Println(payload)
		return 0
	}
	fmt.Print(FormatFlowViewCLI(flow, incidents, degraded))
	return 0
}

func fetchActiveIncidents() ([]IncidentSummary, error) {
	if activeIncidentProvider == nil {
		return nil, errors.New("incident provider not configured")
	}
	return activeIncidentProvider()
}

func filterIncidentsForFlow(incidents []IncidentSummary, flow Flow) []IncidentSummary {
	var out []IncidentSummary
	for _, inc := range incidents {
		if flowHasService(flow, inc.Service) {
			out = append(out, inc)
		}
	}
	return out
}

func activeServiceSet(incidents []IncidentSummary) map[string]struct{} {
	services := make(map[string]struct{})
	for _, inc := range incidents {
		service := strings.TrimSpace(inc.Service)
		if service != "" {
			services[service] = struct{}{}
		}
	}
	return services
}

func printLoadError(err error) {
	switch {
	case IsNoConfig(err):
		fmt.Fprintln(os.Stderr, "Flows disabled (no config found)")
	case IsInvalidConfig(err):
		fmt.Fprintln(os.Stderr, "Invalid flows config — skipping")
	default:
		fmt.Fprintln(os.Stderr, "Failed to load flows:", err.Error())
	}
}

func isHelpArg(arg string) bool {
	switch strings.TrimSpace(arg) {
	case "-h", "--help", "help":
		return true
	default:
		return false
	}
}

func printUsage() {
	fmt.Print(`Flow Commands
=============

Version: ` + model.Version + `

USAGE
  health-monitor flow list [--json]
  health-monitor flow validate
  health-monitor flow <id>
  health-monitor flow <id> --json
  health-monitor flow --help

CONFIG PATHS
  /etc/health-monitor/flows.yaml
  /etc/health-monitor/flows.d/*.yaml
  ~/.health-monitor/flows.yaml
  ~/.health-monitor/flows.d/*.yaml

EXAMPLES
  health-monitor flow list
  health-monitor flow validate
  health-monitor flow order_flow
  health-monitor flow order_flow --json
`)
}

func flowIncidentCounts(flows []Flow, incidents []IncidentSummary) map[string]int {
	counts := make(map[string]int, len(flows))
	for _, flowItem := range flows {
		for _, inc := range incidents {
			if flowHasService(flowItem, strings.TrimSpace(inc.Service)) {
				counts[flowItem.ID]++
			}
		}
	}
	return counts
}

func splitArgs(args []string) (flags []string, positionals []string) {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
		} else {
			positionals = append(positionals, arg)
		}
	}
	return flags, positionals
}
