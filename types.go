package sdk

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelrelay/modelrelay/sdk/go/headers"
	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

// ProviderID is a strongly-typed wrapper around provider identifiers.
// Most callers should not set this; the server routes based on the model.
type ProviderID string

func NewProviderID(val string) ProviderID { return ProviderID(strings.TrimSpace(val)) }
func (p ProviderID) IsEmpty() bool        { return strings.TrimSpace(string(p)) == "" }
func (p ProviderID) String() string       { return string(p) }

// ResponseRequest is an opaque request object for the /responses endpoint.
//
// Callers should construct requests via the fluent builder returned by
// `client.Responses.New()` (preferred) or via package-level helpers.
//
// This type intentionally does not expose struct fields to avoid "stringly-typed"
// composite literals and to keep the request shape evolvable without breaking
// callers.
type ResponseRequest struct {
	provider        ProviderID
	model           ModelID
	input           []llm.InputItem
	outputFormat    *llm.OutputFormat
	maxOutputTokens int64
	temperature     *float64
	stop            []string
	tools           []llm.Tool
	toolChoice      *llm.ToolChoice
}

// Validate returns an error when required fields are missing.
func (r ResponseRequest) Validate() error {
	return r.validate(true)
}

func (r ResponseRequest) validate(requireModel bool) error {
	if requireModel && r.model.IsEmpty() {
		return fmt.Errorf("model is required")
	}
	if len(r.input) == 0 {
		return fmt.Errorf("input is required")
	}
	// The SDK does not validate model identifiers beyond non-emptiness.
	// Callers may pass arbitrary custom ids; the server performs
	// authoritative validation so new models can be adopted without
	// requiring an SDK upgrade.
	if rf := r.outputFormat; rf != nil && rf.Type == llm.OutputFormatTypeJSONSchema {
		if rf.JSONSchema == nil || strings.TrimSpace(rf.JSONSchema.Name) == "" || len(rf.JSONSchema.Schema) == 0 {
			return fmt.Errorf("output_format.json_schema.name and schema are required when type=json_schema")
		}
	}
	return nil
}

// Input returns a copy of the input items for validation and introspection.
// Mutating the returned slice does not affect the request.
func (r ResponseRequest) Input() []llm.InputItem {
	return append([]llm.InputItem(nil), r.input...)
}

// Response wraps the server response and surfaces the echoed request ID.
type Response struct {
	ID         string           `json:"id"`
	Provider   string           `json:"provider,omitempty"`
	Output     []llm.OutputItem `json:"output"`
	StopReason StopReason       `json:"stop_reason,omitempty"`
	Model      ModelID          `json:"model"`
	Usage      Usage            `json:"usage"`
	RequestID  string           `json:"-"`
	Citations  []llm.Citation   `json:"citations,omitempty"`
}

// StreamHandle exposes the streaming interface plus associated metadata.
type StreamHandle struct {
	RequestID      string
	stream         streamReader
	startedAt      time.Time
	firstTokenTime time.Time // Set when first text content is received
}

type streamReader interface {
	Next() (StreamEvent, bool, error)
	Close() error
}

// Next advances the stream, returning false when the stream is complete.
// Each event includes an Elapsed field showing time since stream start.
func (s *StreamHandle) Next() (StreamEvent, bool, error) {
	ev, ok, err := s.stream.Next()
	if err != nil || !ok {
		return ev, ok, err
	}

	// Populate timing information
	now := time.Now()
	if !s.startedAt.IsZero() {
		ev.Elapsed = now.Sub(s.startedAt)
	}

	// Track first token time for TTFT
	if s.firstTokenTime.IsZero() && ev.TextDelta != "" {
		s.firstTokenTime = now
	}

	return ev, ok, nil
}

// Close terminates the underlying stream.
func (s *StreamHandle) Close() error {
	return s.stream.Close()
}

// TTFT returns the time-to-first-token as observed during streaming.
// Returns 0 if no content has been received yet.
func (s *StreamHandle) TTFT() time.Duration {
	if s.firstTokenTime.IsZero() || s.startedAt.IsZero() {
		return 0
	}
	ttft := s.firstTokenTime.Sub(s.startedAt)
	if ttft < 0 {
		return 0
	}
	return ttft
}

// StartedAt returns when the stream request was initiated.
func (s *StreamHandle) StartedAt() time.Time {
	return s.startedAt
}

// Elapsed returns the time since the stream started.
func (s *StreamHandle) Elapsed() time.Duration {
	if s.startedAt.IsZero() {
		return 0
	}
	return time.Since(s.startedAt)
}

// Collect drains the stream into an aggregated Response using the same
// semantics as ChatStream. The stream is closed when the call returns.
func (s *StreamHandle) Collect(ctx context.Context) (*Response, error) {
	return newResponseStream(s).Collect(ctx)
}

// ResponseStreamMetrics reports end-to-end stream timings and metadata as observed by the SDK.
// TTFT is measured as time from request start to the first non-empty content update.
type ResponseStreamMetrics struct {
	TTFT     time.Duration
	Duration time.Duration
	Usage    *Usage
	Model    ModelID
	ID       string
}

// CollectWithMetrics drains the stream into an aggregated Response and returns stream timing metadata.
// The stream is closed when the call returns.
func (s *StreamHandle) CollectWithMetrics(ctx context.Context) (*Response, ResponseStreamMetrics, error) {
	return newResponseStream(s).CollectWithMetrics(ctx)
}

// ResponseOption customizes outgoing responses requests (headers, request IDs, etc.).
type ResponseOption func(*responseCallOptions)

type responseCallOptions struct {
	headers http.Header
	timeout *time.Duration
	retry   *RetryConfig
	stream  StreamTimeouts
}

// WithRequestID sets the X-ModelRelay-Request-Id header for the request.
func WithRequestID(requestID string) ResponseOption {
	return func(opts *responseCallOptions) {
		clean := strings.TrimSpace(requestID)
		if clean == "" {
			return
		}
		if opts.headers == nil {
			opts.headers = make(http.Header)
		}
		opts.headers.Set(headers.RequestID, clean)
	}
}

// WithCustomerID sets the X-ModelRelay-Customer-Id header for customer-attributed requests.
// When this header is set, the customer's subscription tier (if any) determines model defaults.
func WithCustomerID(customerID string) ResponseOption {
	return func(opts *responseCallOptions) {
		clean := strings.TrimSpace(customerID)
		if clean == "" {
			return
		}
		if opts.headers == nil {
			opts.headers = make(http.Header)
		}
		opts.headers.Set(headers.CustomerID, clean)
	}
}

// WithHeader attaches an arbitrary header to the underlying HTTP request.
func WithHeader(key, value string) ResponseOption {
	return func(opts *responseCallOptions) {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return
		}
		if opts.headers == nil {
			opts.headers = make(http.Header)
		}
		opts.headers.Add(strings.TrimSpace(key), strings.TrimSpace(value))
	}
}

// WithHeaders attaches multiple headers to the underlying HTTP request.
func WithHeaders(hdrs map[string]string) ResponseOption {
	return func(opts *responseCallOptions) {
		if len(hdrs) == 0 {
			return
		}
		if opts.headers == nil {
			opts.headers = make(http.Header)
		}
		for key, value := range hdrs {
			k := strings.TrimSpace(key)
			v := strings.TrimSpace(value)
			if k == "" || v == "" {
				continue
			}
			opts.headers.Add(k, v)
		}
	}
}

// WithTimeout overrides the request timeout for this call (0 disables timeout).
func WithTimeout(timeout time.Duration) ResponseOption {
	return func(opts *responseCallOptions) {
		opts.timeout = &timeout
	}
}

// WithRetry overrides the retry policy for this call.
func WithRetry(cfg RetryConfig) ResponseOption {
	return func(opts *responseCallOptions) {
		retryCfg := cfg
		if retryCfg.BaseBackoff == 0 {
			retryCfg.BaseBackoff = defaultRetryConfig().BaseBackoff
		}
		if retryCfg.MaxBackoff == 0 {
			retryCfg.MaxBackoff = defaultRetryConfig().MaxBackoff
		}
		opts.retry = &retryCfg
	}
}

// DisableRetry forces a single attempt for this call.
func DisableRetry() ResponseOption {
	return func(opts *responseCallOptions) {
		cfg := RetryConfig{MaxAttempts: 1, BaseBackoff: 0, MaxBackoff: 0, RetryPost: false}
		opts.retry = &cfg
	}
}

// StreamTimeouts configures streaming timeouts for /responses streams.
//
// TTFT: time until the first non-empty content update is observed.
// Idle: maximum time between successive NDJSON records.
// Total: overall stream deadline.
type StreamTimeouts struct {
	TTFT  time.Duration
	Idle  time.Duration
	Total time.Duration
}

// WithStreamTimeouts configures streaming timeouts for /responses streams.
func WithStreamTimeouts(timeouts StreamTimeouts) ResponseOption {
	return func(opts *responseCallOptions) {
		opts.stream = timeouts
	}
}

// WithStreamTTFTTimeout sets the TTFT timeout for streams (0 disables).
func WithStreamTTFTTimeout(timeout time.Duration) ResponseOption {
	return func(opts *responseCallOptions) {
		opts.stream.TTFT = timeout
	}
}

// WithStreamIdleTimeout sets the idle timeout for streams (0 disables).
func WithStreamIdleTimeout(timeout time.Duration) ResponseOption {
	return func(opts *responseCallOptions) {
		opts.stream.Idle = timeout
	}
}

// WithStreamTotalTimeout sets the total stream timeout (0 disables).
func WithStreamTotalTimeout(timeout time.Duration) ResponseOption {
	return func(opts *responseCallOptions) {
		opts.stream.Total = timeout
	}
}

func buildResponseCallOptions(options []ResponseOption) responseCallOptions {
	if len(options) == 0 {
		return responseCallOptions{}
	}
	cfg := responseCallOptions{}
	for _, opt := range options {
		if opt == nil {
			continue
		}
		opt(&cfg)
	}
	cfg.headers = sanitizeHeaders(cfg.headers)
	return cfg
}

func applyResponseHeaders(req *http.Request, opts responseCallOptions) {
	for key, values := range opts.headers {
		req.Header.Del(key)
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
}
