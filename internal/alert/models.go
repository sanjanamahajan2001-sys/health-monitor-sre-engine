package alert

import (
	"time"

	"health-monitor/internal/incident"
)

type WebhookPayload struct {
	Receiver string  `json:"receiver"`
	Status   string  `json:"status"`
	Alerts   []Alert `json:"alerts"`
}

type Alert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
}

type Mode string

const (
	ModeSuggest Mode = "suggest"
	ModeAuto    Mode = "auto"
)

type Rule struct {
	Match    map[string]string
	Severity incident.Severity
	Title    string
	Mode     Mode
	Source   string
}
