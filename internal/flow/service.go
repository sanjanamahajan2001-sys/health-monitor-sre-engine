package flow

import "strings"

type Catalog struct {
	flows []Flow
	byID  map[string]Flow
}

func NewCatalog(flows []Flow) *Catalog {
	byID := make(map[string]Flow, len(flows))
	for _, flow := range flows {
		byID[flow.ID] = flow
	}
	return &Catalog{
		flows: flows,
		byID:  byID,
	}
}

func (c *Catalog) List() []Flow {
	return c.flows
}

func (c *Catalog) Get(id string) (Flow, bool) {
	id = strings.TrimSpace(id)
	flow, ok := c.byID[id]
	return flow, ok
}

func (c *Catalog) ImpactedFlows(service string) []Flow {
	service = strings.TrimSpace(service)
	if service == "" {
		return nil
	}
	var impacted []Flow
	for _, flow := range c.flows {
		if flowHasService(flow, service) {
			impacted = append(impacted, flow)
		}
	}
	return impacted
}

func IsDegraded(flow Flow, activeServices map[string]struct{}) bool {
	for _, service := range flow.Services {
		if _, ok := activeServices[service]; ok {
			return true
		}
	}
	return false
}

func flowHasService(flow Flow, service string) bool {
	for _, candidate := range flow.Services {
		if candidate == service {
			return true
		}
	}
	return false
}
