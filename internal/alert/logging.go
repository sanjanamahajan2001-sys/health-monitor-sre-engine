package alert

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

type logEntry struct {
	Time   string            `json:"time"`
	Level  string            `json:"level"`
	Msg    string            `json:"msg"`
	Fields map[string]string `json:"fields,omitempty"`
}

var logFormat atomic.Value

func SetLogFormat(value string) {
	clean := strings.ToLower(strings.TrimSpace(value))
	if clean == "" {
		clean = "json"
	}
	logFormat.Store(clean)
}

func logEvent(level string, msg string, fields map[string]string) {
	if format, ok := logFormat.Load().(string); ok && format == "text" {
		fmt.Fprintf(os.Stderr, "%s: %s\n", strings.ToUpper(level), msg)
		return
	}
	entry := logEntry{
		Time:  time.Now().Format(time.RFC3339),
		Level: level,
		Msg:   msg,
	}
	if len(fields) > 0 {
		entry.Fields = fields
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return
	}
	_, _ = os.Stderr.Write(append(payload, '\n'))
}

func logWarn(msg string, fields map[string]string) {
	logEvent("warn", msg, fields)
}

func logError(msg string, fields map[string]string) {
	logEvent("error", msg, fields)
}
