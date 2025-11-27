package sdk

import (
	"fmt"

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
