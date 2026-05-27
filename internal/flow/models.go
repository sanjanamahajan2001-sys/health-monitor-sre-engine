package flow

import "strings"

type SLO struct {
	ID               string
	Service          string
	Objective        float64
	Window     string
	PromQL     string
	Type             string
	ErrorQuery       string
	TotalQuery       string
	ThresholdSeconds float64
}

type Flow struct {
	ID       string
	Name     string
	Services []string
	SLOs     []SLO
	Metadata map[string]string
}

func (f Flow) DisplayName() string {
	if strings.TrimSpace(f.Name) != "" {
		return strings.TrimSpace(f.Name)
	}
	return f.ID
}

type LoadResult struct {
	Flows    []Flow
	Files    []string
	Warnings []string
}
