package sdk

import (
	"context"
	"fmt"
	"time"

	llm "github.com/modelrelay/modelrelay/llmproxy"
)

// NewProxyRequest constructs a validated request with the required fields set.
func NewProxyRequest(model ModelID, messages []llm.ProxyMessage) (ProxyRequest, error) {
	req := ProxyRequest{
		Model:    model,
		Messages: messages,
	}
	return req, req.Validate()
}

// ProxyRequestBuilder provides a fluent builder with validation.
type ProxyRequestBuilder struct {
	req ProxyRequest
}

// NewProxyRequestBuilder seeds the builder with a model identifier.
func NewProxyRequestBuilder(model ModelID) *ProxyRequestBuilder {
	return &ProxyRequestBuilder{
		req: ProxyRequest{Model: model},
	}
}

// Provider sets the provider identifier.
func (b *ProxyRequestBuilder) Provider(provider ProviderID) *ProxyRequestBuilder {
	b.req.Provider = provider
	return b
}

// Model overrides the model identifier.
func (b *ProxyRequestBuilder) Model(model ModelID) *ProxyRequestBuilder {
	b.req.Model = model
	return b
}

// MaxTokens sets the max tokens limit.
func (b *ProxyRequestBuilder) MaxTokens(max int64) *ProxyRequestBuilder {
	b.req.MaxTokens = max
	return b
}

// Temperature sets the sampling temperature.
func (b *ProxyRequestBuilder) Temperature(temp float64) *ProxyRequestBuilder {
	b.req.Temperature = &temp
	return b
}

// Message appends a chat message.
func (b *ProxyRequestBuilder) Message(role, content string) *ProxyRequestBuilder {
	b.req.Messages = append(b.req.Messages, llm.ProxyMessage{Role: role, Content: content})
	return b
}

// System appends a system message.
func (b *ProxyRequestBuilder) System(content string) *ProxyRequestBuilder {
	return b.Message("system", content)
}

// User appends a user message.
func (b *ProxyRequestBuilder) User(content string) *ProxyRequestBuilder {
	return b.Message("user", content)
}

// Assistant appends an assistant message.
func (b *ProxyRequestBuilder) Assistant(content string) *ProxyRequestBuilder {
	return b.Message("assistant", content)
}

// ResponseFormat sets the structured output configuration.
func (b *ProxyRequestBuilder) ResponseFormat(format llm.ResponseFormat) *ProxyRequestBuilder {
	b.req.ResponseFormat = &format
	return b
}

// Messages replaces the existing message list.
func (b *ProxyRequestBuilder) Messages(msgs []llm.ProxyMessage) *ProxyRequestBuilder {
	b.req.Messages = msgs
	return b
}

// Metadata sets the metadata map.
func (b *ProxyRequestBuilder) Metadata(metadata map[string]string) *ProxyRequestBuilder {
	b.req.Metadata = metadata
	return b
}

// MetadataEntry adds a single metadata key/value.
func (b *ProxyRequestBuilder) MetadataEntry(key, value string) *ProxyRequestBuilder {
	if key == "" || value == "" {
		return b
	}
	if b.req.Metadata == nil {
		b.req.Metadata = make(map[string]string)
	}
	b.req.Metadata[key] = value
	return b
}

// Stop sets the stop sequences (OpenAI-style stop).
func (b *ProxyRequestBuilder) Stop(stop []string) *ProxyRequestBuilder {
	b.req.Stop = stop
	return b
}

// StopSequences sets Anthropic-style stop sequences.
func (b *ProxyRequestBuilder) StopSequences(stop []string) *ProxyRequestBuilder {
	b.req.StopSequences = stop
	return b
}

// Build validates and returns the request.
func (b *ProxyRequestBuilder) Build() (ProxyRequest, error) {
	if b.req.Model.IsEmpty() {
		return ProxyRequest{}, fmt.Errorf("model is required")
	}
	if len(b.req.Messages) == 0 {
		return ProxyRequest{}, fmt.Errorf("at least one message is required")
	}
	return b.req, nil
}

// ChatBuilder offers a fluent API that mirrors ProxyRequest fields and wires
// directly into LLMClient helpers for blocking and streaming calls.
type ChatBuilder struct {
	client  *LLMClient
	req     ProxyRequest
	options []ProxyOption
}

// Chat seeds a ChatBuilder with the given model identifier.
func (c *LLMClient) Chat(model ModelID) *ChatBuilder {
	return &ChatBuilder{client: c, req: ProxyRequest{Model: model}}
}

// Provider sets the provider identifier.
func (b *ChatBuilder) Provider(provider ProviderID) *ChatBuilder {
	b.req.Provider = provider
	return b
}

// Model overrides the model identifier.
func (b *ChatBuilder) Model(model ModelID) *ChatBuilder {
	b.req.Model = model
	return b
}

// MaxTokens sets the max tokens limit.
func (b *ChatBuilder) MaxTokens(max int64) *ChatBuilder {
	b.req.MaxTokens = max
	return b
}

// Temperature sets the sampling temperature.
func (b *ChatBuilder) Temperature(temp float64) *ChatBuilder {
	b.req.Temperature = &temp
	return b
}

// Message appends a chat message.
func (b *ChatBuilder) Message(role, content string) *ChatBuilder {
	b.req.Messages = append(b.req.Messages, llm.ProxyMessage{Role: role, Content: content})
	return b
}

// System appends a system message.
func (b *ChatBuilder) System(content string) *ChatBuilder {
	return b.Message("system", content)
}

// User appends a user message.
func (b *ChatBuilder) User(content string) *ChatBuilder {
	return b.Message("user", content)
}

// Assistant appends an assistant message.
func (b *ChatBuilder) Assistant(content string) *ChatBuilder {
	return b.Message("assistant", content)
}

// Messages replaces the existing message list.
func (b *ChatBuilder) Messages(msgs []llm.ProxyMessage) *ChatBuilder {
	b.req.Messages = msgs
	return b
}

// Metadata sets the metadata map.
func (b *ChatBuilder) Metadata(metadata map[string]string) *ChatBuilder {
	b.req.Metadata = metadata
	return b
}

// MetadataEntry adds a single metadata key/value.
func (b *ChatBuilder) MetadataEntry(key, value string) *ChatBuilder {
	if key == "" || value == "" {
		return b
	}
	if b.req.Metadata == nil {
		b.req.Metadata = make(map[string]string)
	}
	b.req.Metadata[key] = value
	return b
}

// Stop sets the stop sequences (OpenAI-style stop).
func (b *ChatBuilder) Stop(stop ...string) *ChatBuilder {
	b.req.Stop = append([]string(nil), stop...)
	return b
}

// StopSequences sets Anthropic-style stop sequences.
func (b *ChatBuilder) StopSequences(stop ...string) *ChatBuilder {
	b.req.StopSequences = append([]string(nil), stop...)
	return b
}

// RequestID sets the X-ModelRelay-Chat-Request-Id header for the request.
func (b *ChatBuilder) RequestID(requestID string) *ChatBuilder {
	return b.Option(WithRequestID(requestID))
}

// Header attaches an arbitrary header to the underlying HTTP request.
func (b *ChatBuilder) Header(key, value string) *ChatBuilder {
	return b.Option(WithHeader(key, value))
}

// Headers attaches multiple headers to the underlying HTTP request.
func (b *ChatBuilder) Headers(headers map[string]string) *ChatBuilder {
	return b.Option(WithHeaders(headers))
}

// Timeout overrides the request timeout for this call (0 disables timeout).
func (b *ChatBuilder) Timeout(timeout time.Duration) *ChatBuilder {
	return b.Option(WithTimeout(timeout))
}

// Retry overrides the retry policy for this call.
func (b *ChatBuilder) Retry(cfg RetryConfig) *ChatBuilder {
	return b.Option(WithRetry(cfg))
}

// DisableRetry forces a single attempt for this call.
func (b *ChatBuilder) DisableRetry() *ChatBuilder {
	return b.Option(DisableRetry())
}

// Option appends a raw ProxyOption for advanced scenarios.
func (b *ChatBuilder) Option(opt ProxyOption) *ChatBuilder {
	if opt != nil {
		b.options = append(b.options, opt)
	}
	return b
}

// Build validates and returns the request and accumulated options.
func (b *ChatBuilder) Build() (ProxyRequest, []ProxyOption, error) {
	if err := b.req.Validate(); err != nil {
		return ProxyRequest{}, nil, err
	}
	return b.req, append([]ProxyOption(nil), b.options...), nil
}

// Send performs a blocking completion and returns the aggregated response.
func (b *ChatBuilder) Send(ctx context.Context) (*ProxyResponse, error) {
	req, opts, err := b.Build()
	if err != nil {
		return nil, err
	}
	return b.client.ProxyMessage(ctx, req, opts...)
}

// Stream opens a streaming SSE connection for chat completions with a helper adapter.
func (b *ChatBuilder) Stream(ctx context.Context) (*ChatStream, error) {
	req, opts, err := b.Build()
	if err != nil {
		return nil, err
	}
	handle, err := b.client.ProxyStream(ctx, req, opts...)
	if err != nil {
		return nil, err
	}
	return newChatStream(handle), nil
}

// Collect opens a streaming chat request and aggregates the response, allowing
// callers to use the streaming endpoint while receiving a blocking-style result.
func (b *ChatBuilder) Collect(ctx context.Context) (*ProxyResponse, error) {
	stream, err := b.Stream(ctx)
	if err != nil {
		return nil, err
	}
	return stream.Collect(ctx)
}
