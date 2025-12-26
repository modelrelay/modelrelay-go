// Package testutil provides helpers for SDK tests.
package testutil

import (
	"net/http"
	"net/http/httptest"
	"time"
)

// NDJSONStep describes a line to emit with an optional delay.
type NDJSONStep struct {
	Delay time.Duration
	Line  string
}

// NDJSONServerConfig configures the NDJSON test server.
type NDJSONServerConfig struct {
	Status     int
	Headers    map[string]string
	FinalDelay time.Duration
}

// NewNDJSONServer returns an httptest server that streams NDJSON lines with delays.
func NewNDJSONServer(steps []NDJSONStep, cfg NDJSONServerConfig) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		status := cfg.Status
		if status == 0 {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		for k, v := range cfg.Headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		flusher, _ := w.(http.Flusher)
		for _, step := range steps {
			if step.Delay > 0 {
				time.Sleep(step.Delay)
			}
			_, _ = w.Write([]byte(step.Line + "\n"))
			if flusher != nil {
				flusher.Flush()
			}
		}
		if cfg.FinalDelay > 0 {
			time.Sleep(cfg.FinalDelay)
		}
	}))
}
