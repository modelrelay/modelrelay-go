package sdk

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/modelrelay/modelrelay/sdk/go/headers"
	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

// ResponseBuilder is a fluent builder for /responses requests.
//
// It is the primary way to construct a ResponseRequest, since ResponseRequest is
// intentionally opaque.
type ResponseBuilder struct {
	client *Client
	req    ResponseRequest

	requestID  string
	customerID string
	headers    http.Header
	timeout    *time.Duration
	stream     StreamTimeouts
	retry      *RetryConfig
}

// New returns a fresh builder. Set either Model(...) or CustomerID(...).
func (c *ResponsesClient) New() ResponseBuilder {
	return ResponseBuilder{client: c.client}
}

func (b ResponseBuilder) Provider(provider ProviderID) ResponseBuilder {
	b.req.provider = provider
	return b
}

func (b ResponseBuilder) Model(model ModelID) ResponseBuilder {
	b.req.model = model
	return b
}

func (b ResponseBuilder) MaxOutputTokens(maxOutputTokens int64) ResponseBuilder {
	b.req.maxOutputTokens = maxOutputTokens
	return b
}

func (b ResponseBuilder) Temperature(temp float64) ResponseBuilder {
	b.req.temperature = &temp
	return b
}

func (b ResponseBuilder) Stop(stop ...string) ResponseBuilder {
	clean := make([]string, 0, len(stop))
	for _, s := range stop {
		s = strings.TrimSpace(s)
		if s != "" {
			clean = append(clean, s)
		}
	}
	b.req.stop = clean
	return b
}

func (b ResponseBuilder) OutputFormat(format llm.OutputFormat) ResponseBuilder {
	b.req.outputFormat = &format
	return b
}

func (b ResponseBuilder) Tools(tools []llm.Tool) ResponseBuilder {
	b.req.tools = append([]llm.Tool(nil), tools...)
	return b
}

func (b ResponseBuilder) Tool(tool llm.Tool) ResponseBuilder {
	next := make([]llm.Tool, len(b.req.tools)+1)
	copy(next, b.req.tools)
	next[len(b.req.tools)] = tool
	b.req.tools = next
	return b
}

func (b ResponseBuilder) FunctionTool(name ToolName, description string, parameters json.RawMessage) ResponseBuilder {
	return b.Tool(llm.Tool{
		Type: llm.ToolTypeFunction,
		Function: &llm.FunctionTool{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		},
	})
}

func (b ResponseBuilder) ToolChoice(choice llm.ToolChoice) ResponseBuilder {
	b.req.toolChoice = &choice
	return b
}

func (b ResponseBuilder) ToolChoiceAuto() ResponseBuilder {
	return b.ToolChoice(llm.ToolChoice{Type: llm.ToolChoiceAuto})
}
func (b ResponseBuilder) ToolChoiceRequired() ResponseBuilder {
	return b.ToolChoice(llm.ToolChoice{Type: llm.ToolChoiceRequired})
}
func (b ResponseBuilder) ToolChoiceNone() ResponseBuilder {
	return b.ToolChoice(llm.ToolChoice{Type: llm.ToolChoiceNone})
}

func (b ResponseBuilder) Input(items []llm.InputItem) ResponseBuilder {
	b.req.input = append([]llm.InputItem(nil), items...)
	return b
}

// Items is a variadic version of Input for convenience.
// Example: .Items(llm.NewSystemText("..."), llm.NewUserText("..."))
func (b ResponseBuilder) Items(items ...llm.InputItem) ResponseBuilder {
	b.req.input = append([]llm.InputItem(nil), items...)
	return b
}

func (b ResponseBuilder) Item(item llm.InputItem) ResponseBuilder {
	next := make([]llm.InputItem, len(b.req.input)+1)
	copy(next, b.req.input)
	next[len(b.req.input)] = item
	b.req.input = next
	return b
}

func (b ResponseBuilder) Message(role llm.MessageRole, content string) ResponseBuilder {
	return b.Item(llm.InputItem{
		Type:    llm.InputItemTypeMessage,
		Role:    role,
		Content: []llm.ContentPart{llm.TextPart(content)},
	})
}

func (b ResponseBuilder) System(content string) ResponseBuilder {
	return b.Message(llm.RoleSystem, content)
}
func (b ResponseBuilder) User(content string) ResponseBuilder {
	return b.Message(llm.RoleUser, content)
}
func (b ResponseBuilder) Assistant(content string) ResponseBuilder {
	return b.Message(llm.RoleAssistant, content)
}

func (b ResponseBuilder) ToolResultText(toolCallID ToolCallID, content string) ResponseBuilder {
	return b.Item(llm.InputItem{
		Type:       llm.InputItemTypeMessage,
		Role:       llm.RoleTool,
		ToolCallID: toolCallID,
		Content:    []llm.ContentPart{llm.TextPart(content)},
	})
}

// CustomerID applies X-ModelRelay-Customer-Id and allows omitting Model().
func (b ResponseBuilder) CustomerID(customerID string) ResponseBuilder {
	b.customerID = strings.TrimSpace(customerID)
	return b
}

func (b ResponseBuilder) RequestID(requestID string) ResponseBuilder {
	b.requestID = strings.TrimSpace(requestID)
	return b
}

func (b ResponseBuilder) Header(key, value string) ResponseBuilder {
	k := strings.TrimSpace(key)
	v := strings.TrimSpace(value)
	if k == "" || v == "" {
		return b
	}
	b.headers = cloneHTTPHeader(b.headers)
	b.headers.Add(k, v)
	return b
}

func (b ResponseBuilder) Headers(hdrs map[string]string) ResponseBuilder {
	if len(hdrs) == 0 {
		return b
	}
	b.headers = cloneHTTPHeader(b.headers)
	for key, value := range hdrs {
		k := strings.TrimSpace(key)
		v := strings.TrimSpace(value)
		if k == "" || v == "" {
			continue
		}
		b.headers.Add(k, v)
	}
	return b
}

func (b ResponseBuilder) Timeout(timeout time.Duration) ResponseBuilder {
	b.timeout = &timeout
	return b
}

// StreamTTFTTimeout sets the TTFT timeout for streaming calls (0 disables).
func (b ResponseBuilder) StreamTTFTTimeout(timeout time.Duration) ResponseBuilder {
	b.stream.TTFT = timeout
	return b
}

// StreamIdleTimeout sets the idle timeout for streaming calls (0 disables).
func (b ResponseBuilder) StreamIdleTimeout(timeout time.Duration) ResponseBuilder {
	b.stream.Idle = timeout
	return b
}

// StreamTotalTimeout sets the overall stream timeout (0 disables).
func (b ResponseBuilder) StreamTotalTimeout(timeout time.Duration) ResponseBuilder {
	b.stream.Total = timeout
	return b
}

func (b ResponseBuilder) Retry(cfg RetryConfig) ResponseBuilder {
	retryCfg := cfg
	if retryCfg.BaseBackoff == 0 {
		retryCfg.BaseBackoff = defaultRetryConfig().BaseBackoff
	}
	if retryCfg.MaxBackoff == 0 {
		retryCfg.MaxBackoff = defaultRetryConfig().MaxBackoff
	}
	b.retry = &retryCfg
	return b
}

func (b ResponseBuilder) DisableRetry() ResponseBuilder {
	cfg := RetryConfig{MaxAttempts: 1, BaseBackoff: 0, MaxBackoff: 0, RetryPost: false}
	b.retry = &cfg
	return b
}

func (b ResponseBuilder) Build() (ResponseRequest, []ResponseOption, error) {
	opts := b.buildOptions()
	callOpts := buildResponseCallOptions(opts)
	requireModel := strings.TrimSpace(callOpts.headers.Get(headers.CustomerID)) == ""
	if requireModel && b.client != nil && b.client.hasJWTAccessToken() {
		requireModel = false
	}
	if err := b.req.validate(requireModel); err != nil {
		return ResponseRequest{}, nil, err
	}
	return b.req, opts, nil
}

func (b ResponseBuilder) buildOptions() []ResponseOption {
	var opts []ResponseOption

	if strings.TrimSpace(b.requestID) != "" {
		opts = append(opts, WithRequestID(b.requestID))
	}
	if strings.TrimSpace(b.customerID) != "" {
		opts = append(opts, WithCustomerID(b.customerID))
	}
	for key, values := range sanitizeHeaders(b.headers) {
		for _, value := range values {
			opts = append(opts, WithHeader(key, value))
		}
	}
	if b.timeout != nil {
		opts = append(opts, WithTimeout(*b.timeout))
	}
	if b.stream.TTFT > 0 || b.stream.Idle > 0 || b.stream.Total > 0 {
		opts = append(opts, WithStreamTimeouts(b.stream))
	}
	if b.retry != nil {
		opts = append(opts, WithRetry(*b.retry))
	}

	return opts
}

func cloneHTTPHeader(src http.Header) http.Header {
	if len(src) == 0 {
		return make(http.Header)
	}
	out := make(http.Header, len(src))
	for k, v := range src {
		out[k] = append([]string(nil), v...)
	}
	return out
}
