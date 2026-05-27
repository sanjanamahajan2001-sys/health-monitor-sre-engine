package rules

import "health-monitor/internal/analyse/cleanup"

func CleanupMessages(r cleanup.Result) []string {
	var msgs []string

	if r.DiskUsagePct >= 80 {
		msgs = append(msgs, "Disk usage is high — consider cleaning logs or unused data")
	}
	if r.VarLogMB > 500 {
		msgs = append(msgs, "/var/log is large — ensure log rotation is configured")
	}
	if r.VarCacheMB > 200 {
		msgs = append(msgs, "/var/cache can be cleaned safely if required")
	}

	if len(msgs) == 0 {
		msgs = append(msgs, "No cleanup actions required. System looks healthy.")
	}

	return msgs
}
