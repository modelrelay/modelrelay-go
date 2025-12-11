package sdk

import (
	"context"
	"fmt"
	"time"

	llm "github.com/modelrelay/modelrelay/providers"
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

// Model overrides the model identifier.
func (b *ProxyRequestBuilder) Model(model ModelID) *ProxyRequestBuilder {
	b.req.Model = model
	return b
}

// MaxTokens sets the max tokens limit.
func (b *ProxyRequestBuilder) MaxTokens(maxTokens int64) *ProxyRequestBuilder {
	b.req.MaxTokens = maxTokens
	return b
}

// Temperature sets the sampling temperature.
func (b *ProxyRequestBuilder) Temperature(temp float64) *ProxyRequestBuilder {
	b.req.Temperature = &temp
	return b
}

// Message appends a chat message.
func (b *ProxyRequestBuilder) Message(role llm.MessageRole, content string) *ProxyRequestBuilder {
	b.req.Messages = append(b.req.Messages, llm.ProxyMessage{Role: role, Content: content})
	return b
}

// System appends a system message.
func (b *ProxyRequestBuilder) System(content string) *ProxyRequestBuilder {
	return b.Message(llm.RoleSystem, content)
}

// User appends a user message.
func (b *ProxyRequestBuilder) User(content string) *ProxyRequestBuilder {
	return b.Message(llm.RoleUser, content)
}

// Assistant appends an assistant message.
func (b *ProxyRequestBuilder) Assistant(content string) *ProxyRequestBuilder {
	return b.Message(llm.RoleAssistant, content)
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

// Tools sets the tool list.
func (b *ProxyRequestBuilder) Tools(tools []llm.Tool) *ProxyRequestBuilder {
	b.req.Tools = tools
	return b
}

// Tool appends a single tool.
func (b *ProxyRequestBuilder) Tool(tool llm.Tool) *ProxyRequestBuilder {
	b.req.Tools = append(b.req.Tools, tool)
	return b
}

// FunctionTool appends a function tool with the given name, description, and parameters.
func (b *ProxyRequestBuilder) FunctionTool(name, description string, parameters []byte) *ProxyRequestBuilder {
	b.req.Tools = append(b.req.Tools, llm.Tool{
		Type: llm.ToolTypeFunction,
		Function: &llm.FunctionTool{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		},
	})
	return b
}

// ToolChoice sets the tool choice.
func (b *ProxyRequestBuilder) ToolChoice(choice llm.ToolChoice) *ProxyRequestBuilder {
	b.req.ToolChoice = &choice
	return b
}

// ToolChoiceAuto sets tool_choice to auto.
func (b *ProxyRequestBuilder) ToolChoiceAuto() *ProxyRequestBuilder {
	return b.ToolChoice(llm.ToolChoice{Type: llm.ToolChoiceAuto})
}

// ToolChoiceRequired sets tool_choice to required.
func (b *ProxyRequestBuilder) ToolChoiceRequired() *ProxyRequestBuilder {
	return b.ToolChoice(llm.ToolChoice{Type: llm.ToolChoiceRequired})
}

// ToolChoiceNone sets tool_choice to none.
func (b *ProxyRequestBuilder) ToolChoiceNone() *ProxyRequestBuilder {
	return b.ToolChoice(llm.ToolChoice{Type: llm.ToolChoiceNone})
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

// Model overrides the model identifier.
func (b *ChatBuilder) Model(model ModelID) *ChatBuilder {
	b.req.Model = model
	return b
}

// MaxTokens sets the max tokens limit.
func (b *ChatBuilder) MaxTokens(maxTokens int64) *ChatBuilder {
	b.req.MaxTokens = maxTokens
	return b
}

// Temperature sets the sampling temperature.
func (b *ChatBuilder) Temperature(temp float64) *ChatBuilder {
	b.req.Temperature = &temp
	return b
}

// Message appends a chat message.
func (b *ChatBuilder) Message(role llm.MessageRole, content string) *ChatBuilder {
	b.req.Messages = append(b.req.Messages, llm.ProxyMessage{Role: role, Content: content})
	return b
}

// System appends a system message.
func (b *ChatBuilder) System(content string) *ChatBuilder {
	return b.Message(llm.RoleSystem, content)
}

// User appends a user message.
func (b *ChatBuilder) User(content string) *ChatBuilder {
	return b.Message(llm.RoleUser, content)
}

// Assistant appends an assistant message.
func (b *ChatBuilder) Assistant(content string) *ChatBuilder {
	return b.Message(llm.RoleAssistant, content)
}

// Messages replaces the existing message list.
func (b *ChatBuilder) Messages(msgs []llm.ProxyMessage) *ChatBuilder {
	b.req.Messages = msgs
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

// Tools sets the tool list.
func (b *ChatBuilder) Tools(tools []llm.Tool) *ChatBuilder {
	b.req.Tools = tools
	return b
}

// Tool appends a single tool.
func (b *ChatBuilder) Tool(tool llm.Tool) *ChatBuilder {
	b.req.Tools = append(b.req.Tools, tool)
	return b
}

// FunctionTool appends a function tool with the given name, description, and parameters.
func (b *ChatBuilder) FunctionTool(name, description string, parameters []byte) *ChatBuilder {
	b.req.Tools = append(b.req.Tools, llm.Tool{
		Type: llm.ToolTypeFunction,
		Function: &llm.FunctionTool{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		},
	})
	return b
}

// ToolChoice sets the tool choice.
func (b *ChatBuilder) ToolChoice(choice llm.ToolChoice) *ChatBuilder {
	b.req.ToolChoice = &choice
	return b
}

// ToolChoiceAuto sets tool_choice to auto.
func (b *ChatBuilder) ToolChoiceAuto() *ChatBuilder {
	return b.ToolChoice(llm.ToolChoice{Type: llm.ToolChoiceAuto})
}

// ToolChoiceRequired sets tool_choice to required.
func (b *ChatBuilder) ToolChoiceRequired() *ChatBuilder {
	return b.ToolChoice(llm.ToolChoice{Type: llm.ToolChoiceRequired})
}

// ToolChoiceNone sets tool_choice to none.
func (b *ChatBuilder) ToolChoiceNone() *ChatBuilder {
	return b.ToolChoice(llm.ToolChoice{Type: llm.ToolChoiceNone})
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

// CustomerChatBuilder offers a fluent API for customer-attributed chat requests
// where the tier determines the model. This builder does NOT accept a model parameter
// because the customer's tier controls which model is used.
type CustomerChatBuilder struct {
	client     *LLMClient
	customerID string
	req        ProxyRequest
	options    []ProxyOption
}

// ChatForCustomer seeds a CustomerChatBuilder for a customer-attributed request.
// The customer's tier determines the model - no model parameter is needed or allowed.
// The customerID is sent via the X-ModelRelay-Customer-Id header.
//
// Example:
//
//	stream, err := client.LLM.ChatForCustomer("user-123").
//	    MaxTokens(256).
//	    User("Hello!").
//	    Stream(ctx)
func (c *LLMClient) ChatForCustomer(customerID string) *CustomerChatBuilder {
	return &CustomerChatBuilder{
		client:     c,
		customerID: customerID,
		req:        ProxyRequest{},
	}
}

// MaxTokens sets the max tokens limit.
func (b *CustomerChatBuilder) MaxTokens(maxTokens int64) *CustomerChatBuilder {
	b.req.MaxTokens = maxTokens
	return b
}

// Temperature sets the sampling temperature.
func (b *CustomerChatBuilder) Temperature(temp float64) *CustomerChatBuilder {
	b.req.Temperature = &temp
	return b
}

// Message appends a chat message.
func (b *CustomerChatBuilder) Message(role llm.MessageRole, content string) *CustomerChatBuilder {
	b.req.Messages = append(b.req.Messages, llm.ProxyMessage{Role: role, Content: content})
	return b
}

// System appends a system message.
func (b *CustomerChatBuilder) System(content string) *CustomerChatBuilder {
	return b.Message(llm.RoleSystem, content)
}

// User appends a user message.
func (b *CustomerChatBuilder) User(content string) *CustomerChatBuilder {
	return b.Message(llm.RoleUser, content)
}

// Assistant appends an assistant message.
func (b *CustomerChatBuilder) Assistant(content string) *CustomerChatBuilder {
	return b.Message(llm.RoleAssistant, content)
}

// Messages replaces the existing message list.
func (b *CustomerChatBuilder) Messages(msgs []llm.ProxyMessage) *CustomerChatBuilder {
	b.req.Messages = msgs
	return b
}

// Stop sets the stop sequences (OpenAI-style stop).
func (b *CustomerChatBuilder) Stop(stop ...string) *CustomerChatBuilder {
	b.req.Stop = append([]string(nil), stop...)
	return b
}

// StopSequences sets Anthropic-style stop sequences.
func (b *CustomerChatBuilder) StopSequences(stop ...string) *CustomerChatBuilder {
	b.req.StopSequences = append([]string(nil), stop...)
	return b
}

// Tools sets the tool list.
func (b *CustomerChatBuilder) Tools(tools []llm.Tool) *CustomerChatBuilder {
	b.req.Tools = tools
	return b
}

// Tool appends a single tool.
func (b *CustomerChatBuilder) Tool(tool llm.Tool) *CustomerChatBuilder {
	b.req.Tools = append(b.req.Tools, tool)
	return b
}

// FunctionTool appends a function tool with the given name, description, and parameters.
func (b *CustomerChatBuilder) FunctionTool(name, description string, parameters []byte) *CustomerChatBuilder {
	b.req.Tools = append(b.req.Tools, llm.Tool{
		Type: llm.ToolTypeFunction,
		Function: &llm.FunctionTool{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		},
	})
	return b
}

// ToolChoice sets the tool choice.
func (b *CustomerChatBuilder) ToolChoice(choice llm.ToolChoice) *CustomerChatBuilder {
	b.req.ToolChoice = &choice
	return b
}

// ToolChoiceAuto sets tool_choice to auto.
func (b *CustomerChatBuilder) ToolChoiceAuto() *CustomerChatBuilder {
	return b.ToolChoice(llm.ToolChoice{Type: llm.ToolChoiceAuto})
}

// ToolChoiceRequired sets tool_choice to required.
func (b *CustomerChatBuilder) ToolChoiceRequired() *CustomerChatBuilder {
	return b.ToolChoice(llm.ToolChoice{Type: llm.ToolChoiceRequired})
}

// ToolChoiceNone sets tool_choice to none.
func (b *CustomerChatBuilder) ToolChoiceNone() *CustomerChatBuilder {
	return b.ToolChoice(llm.ToolChoice{Type: llm.ToolChoiceNone})
}

// RequestID sets the X-ModelRelay-Chat-Request-Id header for the request.
func (b *CustomerChatBuilder) RequestID(requestID string) *CustomerChatBuilder {
	return b.Option(WithRequestID(requestID))
}

// Header attaches an arbitrary header to the underlying HTTP request.
func (b *CustomerChatBuilder) Header(key, value string) *CustomerChatBuilder {
	return b.Option(WithHeader(key, value))
}

// Headers attaches multiple headers to the underlying HTTP request.
func (b *CustomerChatBuilder) Headers(headers map[string]string) *CustomerChatBuilder {
	return b.Option(WithHeaders(headers))
}

// Timeout overrides the request timeout for this call (0 disables timeout).
func (b *CustomerChatBuilder) Timeout(timeout time.Duration) *CustomerChatBuilder {
	return b.Option(WithTimeout(timeout))
}

// Retry overrides the retry policy for this call.
func (b *CustomerChatBuilder) Retry(cfg RetryConfig) *CustomerChatBuilder {
	return b.Option(WithRetry(cfg))
}

// DisableRetry forces a single attempt for this call.
func (b *CustomerChatBuilder) DisableRetry() *CustomerChatBuilder {
	return b.Option(DisableRetry())
}

// Option appends a raw ProxyOption for advanced scenarios.
func (b *CustomerChatBuilder) Option(opt ProxyOption) *CustomerChatBuilder {
	if opt != nil {
		b.options = append(b.options, opt)
	}
	return b
}

// Build validates and returns the customer ID, request, and accumulated options.
// Unlike ChatBuilder.Build(), this does NOT require a model because
// the customer's tier determines the model.
func (b *CustomerChatBuilder) Build() (string, ProxyRequest, []ProxyOption, error) {
	if b.customerID == "" {
		return "", ProxyRequest{}, nil, fmt.Errorf("customer ID is required")
	}
	if len(b.req.Messages) == 0 {
		return "", ProxyRequest{}, nil, fmt.Errorf("at least one message is required")
	}
	return b.customerID, b.req, append([]ProxyOption(nil), b.options...), nil
}

// Send performs a blocking completion and returns the aggregated response.
func (b *CustomerChatBuilder) Send(ctx context.Context) (*ProxyResponse, error) {
	customerID, req, opts, err := b.Build()
	if err != nil {
		return nil, err
	}
	return b.client.ProxyCustomerMessage(ctx, customerID, req, opts...)
}

// Stream opens a streaming SSE connection for chat completions with a helper adapter.
func (b *CustomerChatBuilder) Stream(ctx context.Context) (*ChatStream, error) {
	customerID, req, opts, err := b.Build()
	if err != nil {
		return nil, err
	}
	handle, err := b.client.ProxyCustomerStream(ctx, customerID, req, opts...)
	if err != nil {
		return nil, err
	}
	return newChatStream(handle), nil
}

// Collect opens a streaming chat request and aggregates the response, allowing
// callers to use the streaming endpoint while receiving a blocking-style result.
func (b *CustomerChatBuilder) Collect(ctx context.Context) (*ProxyResponse, error) {
	stream, err := b.Stream(ctx)
	if err != nil {
		return nil, err
	}
	return stream.Collect(ctx)
}
