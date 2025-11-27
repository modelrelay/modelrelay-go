package sdk

import (
	"fmt"
	"net/http"

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
	headers http.Header
}

// WithRequestID sets the X-ModelRelay-Chat-Request-Id header for the request.
func WithRequestID(requestID string) ProxyOption {
	return func(opts *proxyCallOptions) {
		if requestID == "" {
			return
		}
		if opts.headers == nil {
			opts.headers = make(http.Header)
		}
		opts.headers.Set(requestIDHeader, requestID)
	}
}

// WithHeader attaches an arbitrary header to the underlying HTTP request.
func WithHeader(key, value string) ProxyOption {
	return func(opts *proxyCallOptions) {
		if key == "" || value == "" {
			return
		}
		if opts.headers == nil {
			opts.headers = make(http.Header)
		}
		opts.headers.Add(key, value)
	}
}

func applyProxyOptions(req *http.Request, options []ProxyOption) {
	if len(options) == 0 {
		return
	}
	cfg := proxyCallOptions{}
	for _, opt := range options {
		if opt == nil {
			continue
		}
		opt(&cfg)
	}
	if len(cfg.headers) == 0 {
		return
	}
	for key, values := range cfg.headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
}
