package sdk

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	llm "github.com/modelrelay/modelrelay/llmproxy"
)

const (
	requestIDHeader = "X-ModelRelay-Chat-Request-Id"
)

// ProxyRequest mirrors the SaaS /llm/proxy JSON contract using typed enums.
type ProxyRequest struct {
	Provider      ProviderID
	Model         ModelID
	MaxTokens     int64
	Temperature   *float64
	Messages      []llm.ProxyMessage
	Metadata      map[string]string
	Stop          []string
	StopSequences []string
}

// Validate returns an error when required fields are missing.
func (r ProxyRequest) Validate() error {
	if r.Model.IsEmpty() {
		return fmt.Errorf("model is required")
	}
	if len(r.Messages) == 0 {
		return fmt.Errorf("at least one message is required")
	}
	return nil
}

// ProxyResponse wraps the server response and surfaces the echoed request ID.
type ProxyResponse struct {
	ID         string     `json:"id"`
	Provider   ProviderID `json:"provider"`
	Content    []string   `json:"content"`
	StopReason StopReason `json:"stop_reason,omitempty"`
	Model      ModelID    `json:"model"`
	Usage      Usage      `json:"usage"`
	RequestID  string     `json:"-"`
}

// StreamHandle exposes the streaming interface plus associated metadata.
type StreamHandle struct {
	RequestID string
	stream    *sseStream
}

// Next advances the stream, returning false when the stream is complete.
func (s *StreamHandle) Next() (StreamEvent, bool, error) {
	return s.stream.Next()
}

// Close terminates the underlying stream.
func (s *StreamHandle) Close() error {
	return s.stream.Close()
}

// ProxyOption customizes outgoing proxy requests (headers, request IDs, etc.).
type ProxyOption func(*proxyCallOptions)

type proxyCallOptions struct {
	headers  http.Header
	metadata map[string]string
	timeout  *time.Duration
	retry    *RetryConfig
}

// WithRequestID sets the X-ModelRelay-Chat-Request-Id header for the request.
func WithRequestID(requestID string) ProxyOption {
	return func(opts *proxyCallOptions) {
		clean := strings.TrimSpace(requestID)
		if clean == "" {
			return
		}
		if opts.headers == nil {
			opts.headers = make(http.Header)
		}
		opts.headers.Set(requestIDHeader, clean)
	}
}

// WithHeader attaches an arbitrary header to the underlying HTTP request.
func WithHeader(key, value string) ProxyOption {
	return func(opts *proxyCallOptions) {
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
func WithHeaders(headers map[string]string) ProxyOption {
	return func(opts *proxyCallOptions) {
		if len(headers) == 0 {
			return
		}
		if opts.headers == nil {
			opts.headers = make(http.Header)
		}
		for key, value := range headers {
			k := strings.TrimSpace(key)
			v := strings.TrimSpace(value)
			if k == "" || v == "" {
				continue
			}
			opts.headers.Add(k, v)
		}
	}
}

// WithMetadataEntry adds a single metadata key/value to the request payload.
func WithMetadataEntry(key, value string) ProxyOption {
	return func(opts *proxyCallOptions) {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return
		}
		if opts.metadata == nil {
			opts.metadata = make(map[string]string)
		}
		opts.metadata[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
}

// WithMetadata merges the provided metadata map into the request payload.
func WithMetadata(metadata map[string]string) ProxyOption {
	return func(opts *proxyCallOptions) {
		if len(metadata) == 0 {
			return
		}
		if opts.metadata == nil {
			opts.metadata = make(map[string]string, len(metadata))
		}
		for key, value := range metadata {
			k := strings.TrimSpace(key)
			v := strings.TrimSpace(value)
			if k == "" || v == "" {
				continue
			}
			opts.metadata[k] = v
		}
	}
}

// WithTimeout overrides the request timeout for this call (0 disables timeout).
func WithTimeout(timeout time.Duration) ProxyOption {
	return func(opts *proxyCallOptions) {
		opts.timeout = &timeout
	}
}

// WithRetry overrides the retry policy for this call.
func WithRetry(cfg RetryConfig) ProxyOption {
	return func(opts *proxyCallOptions) {
		copy := cfg
		if copy.BaseBackoff == 0 {
			copy.BaseBackoff = defaultRetryConfig().BaseBackoff
		}
		if copy.MaxBackoff == 0 {
			copy.MaxBackoff = defaultRetryConfig().MaxBackoff
		}
		opts.retry = &copy
	}
}

// DisableRetry forces a single attempt for this call.
func DisableRetry() ProxyOption {
	return func(opts *proxyCallOptions) {
		cfg := RetryConfig{MaxAttempts: 1, BaseBackoff: 0, MaxBackoff: 0, RetryPost: false}
		opts.retry = &cfg
	}
}

func buildProxyCallOptions(options []ProxyOption) proxyCallOptions {
	if len(options) == 0 {
		return proxyCallOptions{}
	}
	cfg := proxyCallOptions{}
	for _, opt := range options {
		if opt == nil {
			continue
		}
		opt(&cfg)
	}
	cfg.headers = sanitizeHeaders(cfg.headers)
	cfg.metadata = sanitizeMetadata(cfg.metadata)
	return cfg
}

func applyProxyHeaders(req *http.Request, opts proxyCallOptions) {
	for key, values := range opts.headers {
		req.Header.Del(key)
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
}
