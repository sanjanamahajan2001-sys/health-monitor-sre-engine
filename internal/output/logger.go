package output

import (
	"log"
	"os"
)

// LogLevel represents the logging level
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelMetric
	LevelInfo
	LevelWarn
	LevelError
)

var (
	// DefaultLogLevel is Info unless HEALTH_MONITOR_DEBUG is set
	currentLogLevel = LevelInfo
	originalOutput  = os.Stderr
)

func init() {
	if os.Getenv("HEALTH_MONITOR_DEBUG") == "1" || os.Getenv("DEBUG") == "1" {
		currentLogLevel = LevelDebug
	}
}

// Silence redirects all log output to a null device (for TUI mode)
func Silence() {
	log.SetOutput(os.NewFile(0, os.DevNull))
}

// Unsilence restores log output to stderr
func Unsilence() {
	log.SetOutput(originalOutput)
}

// Debugf logs a debug message if the current log level is LevelDebug
func Debugf(format string, args ...interface{}) {
	if currentLogLevel <= LevelDebug {
		log.Printf("DEBUG: "+format, args...)
	}
}

// Metricf logs a metrics message if the current log level is LevelMetric or lower
func Metricf(format string, args ...interface{}) {
	if currentLogLevel <= LevelMetric {
		log.Printf("METRICS: "+format, args...)
	}
}

// Infof logs an info message if the current log level is LevelInfo or lower
func Infof(format string, args ...interface{}) {
	if currentLogLevel <= LevelInfo {
		log.Printf("INFO: "+format, args...)
	}
}

// Warnf logs a warning message if the current log level is LevelWarn or lower
func Warnf(format string, args ...interface{}) {
	if currentLogLevel <= LevelWarn {
		log.Printf("WARN: "+format, args...)
	}
}

// Errorf logs an error message if the current log level is LevelError or lower
func Errorf(format string, args ...interface{}) {
	if currentLogLevel <= LevelError {
		log.Printf("ERROR: "+format, args...)
	}
}

// Successf logs a success message (always visible by default)
func Successf(format string, args ...interface{}) {
	log.Printf("SUCCESS: "+format, args...)
}

// IsDebugEnabled returns true if debug logging is enabled
func IsDebugEnabled() bool {
	return currentLogLevel <= LevelDebug
}

// SetLogLevel manually overrides the log level
func SetLogLevel(level LogLevel) {
	currentLogLevel = level
}
