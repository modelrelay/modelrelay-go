package sdk

import (
	"net/http"

	llm "github.com/recall-gpt/modelrelay/llmproxy"
)

const (
	requestIDHeader = "X-ModelRelay-Chat-Request-Id"
)

// ProxyRequest mirrors the SaaS /llm/proxy JSON contract.
type ProxyRequest struct {
	Provider    string
	Model       string
	MaxTokens   int64
	Temperature *float64
	Messages    []llm.ProxyMessage
	Metadata    map[string]string
}

// ProxyResponse wraps the server response and surfaces the echoed request ID.
type ProxyResponse struct {
	llm.ProxyResponse
	RequestID string
}

// StreamHandle exposes the streaming interface plus associated metadata.
type StreamHandle struct {
	llm.Stream
	RequestID string
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
