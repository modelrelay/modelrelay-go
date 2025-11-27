package sdk

import (
	"context"
	"net/http"
	"time"
)

// TelemetryHooks expose observability callbacks without forcing dependencies on the caller.
type TelemetryHooks struct {
	// OnHTTPRequest fires before the HTTP request is sent.
	OnHTTPRequest func(ctx context.Context, req *http.Request)
	// OnHTTPResponse fires after the request completes (even when err != nil).
	OnHTTPResponse func(ctx context.Context, req *http.Request, resp *http.Response, err error, latency time.Duration)
	// OnStreamEvent fires for every streaming event returned by /llm/proxy.
	OnStreamEvent func(ctx context.Context, event StreamEvent)
	// OnLogEntry allows callers to capture SDK log events (info/errors).
	OnLogEntry func(ctx context.Context, entry LogEntry)
	// OnMetric records lightweight counters/gauges for observability dashboards.
	OnMetric func(ctx context.Context, metric Metric)
}

// LogLevel encodes the severity for log hooks.
type LogLevel string

const (
	LogLevelInfo  LogLevel = "info"
	LogLevelError LogLevel = "error"
)

// LogEntry captures structured log details for SDK consumers.
type LogEntry struct {
	Level   LogLevel
	Message string
	Fields  map[string]any
}

// Metric represents a single observability datapoint.
type Metric struct {
	Name   string
	Value  float64
	Labels map[string]string
}

func (t TelemetryHooks) log(ctx context.Context, level LogLevel, msg string, fields map[string]any) {
	if t.OnLogEntry == nil {
		return
	}
	entry := LogEntry{Level: level, Message: msg, Fields: fields}
	t.OnLogEntry(ctx, entry)
}

func (t TelemetryHooks) metric(ctx context.Context, name string, value float64, labels map[string]string) {
	if t.OnMetric == nil {
		return
	}
	t.OnMetric(ctx, Metric{Name: name, Value: value, Labels: labels})
}
