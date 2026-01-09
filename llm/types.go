package llm

import (
	"context"
	"encoding/json"
	"time"
)

// ResponseRequest captures the public /responses JSON contract.
//
// Note: This request model is provider-agnostic. Provider adapters are
// responsible for translating it into vendor-specific payloads.
type ResponseRequest struct {
	// Provider optionally forces routing to a specific provider (advanced).
	Provider string `json:"provider,omitempty"`
	// Model is optional for customer-attributed requests (subscription tier controls model).
	Model string `json:"model,omitempty"`

	// Input is the ordered list of input items (messages, tool results, etc.).
	Input []InputItem `json:"input"`

	// OutputFormat configures structured outputs (JSON schema) when supported.
	OutputFormat *OutputFormat `json:"output_format,omitempty"`

	// MaxOutputTokens sets the maximum output tokens budget.
	MaxOutputTokens int64    `json:"max_output_tokens,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
	Stop            []string `json:"stop,omitempty"`

	// Tools defines tools the model may invoke during the response.
	Tools []Tool `json:"tools,omitempty"`
	// ToolChoice controls when/how the model should use tools.
	ToolChoice *ToolChoice `json:"tool_choice,omitempty"`
}

// MessageRole identifies the author of a chat message.
type MessageRole string

const (
	// RoleUser identifies input from the human.
	RoleUser MessageRole = "user"
	// RoleAssistant identifies the model's response.
	RoleAssistant MessageRole = "assistant"
	// RoleSystem identifies instructions for the model's behavior.
	RoleSystem MessageRole = "system"
	// RoleTool identifies results from tool/function calls.
	RoleTool MessageRole = "tool"
)

type ContentPartType string

const (
	ContentPartTypeText ContentPartType = "text"
)

// ContentPart represents one chunk of message content.
// Today we support text only; additional content types are additive.
type ContentPart struct {
	Type ContentPartType `json:"type"`
	Text string          `json:"text"`
}

func TextPart(text string) ContentPart {
	return ContentPart{Type: ContentPartTypeText, Text: text}
}

type InputItemType string

const (
	InputItemTypeMessage InputItemType = "message"
)

// InputItem is a tagged union representing a single input item.
// Currently only message items are implemented.
type InputItem struct {
	Type InputItemType `json:"type"`

	// Message fields (type="message")
	Role       MessageRole   `json:"role,omitempty"`
	Content    []ContentPart `json:"content,omitempty"`
	ToolCalls  []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID ToolCallID    `json:"tool_call_id,omitempty"`
}

type OutputItemType string

const (
	OutputItemTypeMessage OutputItemType = "message"
)

// OutputItem is a tagged union representing a single output item.
// Currently only message items are implemented.
type OutputItem struct {
	Type OutputItemType `json:"type"`

	// Message fields (type="message")
	Role      MessageRole   `json:"role,omitempty"`
	Content   []ContentPart `json:"content,omitempty"`
	ToolCalls []ToolCall    `json:"tool_calls,omitempty"`
}

// OutputFormatType captures the provider-agnostic output format options.
type OutputFormatType string

const (
	OutputFormatTypeText       OutputFormatType = "text"
	OutputFormatTypeJSONSchema OutputFormatType = "json_schema"
)

// JSONSchemaFormat models the json_schema payload for structured outputs.
type JSONSchemaFormat struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

// OutputFormat configures structured output options when supported.
type OutputFormat struct {
	Type       OutputFormatType  `json:"type"`
	JSONSchema *JSONSchemaFormat `json:"json_schema,omitempty"`
}

// IsStructured reports whether the format requires structured output handling.
func (f *OutputFormat) IsStructured() bool {
	if f == nil {
		return false
	}
	return f.Type == OutputFormatTypeJSONSchema
}

// Response is returned from the /responses endpoint.
type Response struct {
	Provider   string       `json:"provider,omitempty"`
	ID         string       `json:"id"`
	Output     []OutputItem `json:"output"`
	StopReason string       `json:"stop_reason,omitempty"`
	Model      string       `json:"model"`
	Usage      Usage        `json:"usage"`
	Citations  []Citation   `json:"citations,omitempty"`
}

// Text returns the first text content block found in the output.
func (r *Response) Text() string {
	if r == nil {
		return ""
	}
	for _, item := range r.Output {
		if item.Type != OutputItemTypeMessage {
			continue
		}
		for _, part := range item.Content {
			if part.Type == ContentPartTypeText && part.Text != "" {
				return part.Text
			}
		}
	}
	return ""
}

// Usage provides token accounting metadata.
type Usage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

// ToolType identifies the kind of tool.
type ToolType string

const (
	ToolTypeFunction      ToolType = "function"
	ToolTypeWeb           ToolType = "web"
	ToolTypeXSearch       ToolType = "x_search"
	ToolTypeCodeExecution ToolType = "code_execution"
)

// WebToolIntent describes the user's intent for web access.
// Providers map intent to their supported web tooling.
type WebToolIntent string

const (
	WebIntentAuto      WebToolIntent = "auto"
	WebIntentSearchWeb WebToolIntent = "search_web"
	WebIntentFetchURL  WebToolIntent = "fetch_url"
)

// Tool represents a tool the model can invoke.
type Tool struct {
	Type          ToolType        `json:"type"`
	Function      *FunctionTool   `json:"function,omitempty"`
	Web           *WebToolConfig  `json:"web,omitempty"`
	XSearch       *XSearchConfig  `json:"x_search,omitempty"`
	CodeExecution *CodeExecConfig `json:"code_execution,omitempty"`
}

// FunctionTool defines a custom function the model can call.
type FunctionTool struct {
	Name        ToolName        `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// WebToolConfig configures web search/fetch intent and constraints.
type WebToolConfig struct {
	AllowedDomains  []string      `json:"allowed_domains,omitempty"`
	ExcludedDomains []string      `json:"excluded_domains,omitempty"`
	MaxUses         *int          `json:"max_uses,omitempty"` // Anthropic only
	Intent          WebToolIntent `json:"intent,omitempty"`
}

// XSearchConfig configures X/Twitter search (Grok only).
type XSearchConfig struct {
	AllowedHandles  []string   `json:"allowed_handles,omitempty"`
	ExcludedHandles []string   `json:"excluded_handles,omitempty"`
	FromDate        *time.Time `json:"from_date,omitempty"`
	ToDate          *time.Time `json:"to_date,omitempty"`
}

// CodeExecConfig configures code execution behavior.
type CodeExecConfig struct {
	// Provider-specific options can be added here.
}

// ToolChoiceType controls when the model should use tools.
type ToolChoiceType string

const (
	ToolChoiceAuto     ToolChoiceType = "auto"
	ToolChoiceRequired ToolChoiceType = "required"
	ToolChoiceNone     ToolChoiceType = "none"
)

// ToolChoice specifies tool selection behavior.
type ToolChoice struct {
	Type     ToolChoiceType `json:"type"`
	Function *ToolName      `json:"function,omitempty"` // Force specific function
}

// ToolCall represents a tool invocation requested by the model.
type ToolCall struct {
	ID       ToolCallID    `json:"id"`
	Type     ToolType      `json:"type"`
	Function *FunctionCall `json:"function,omitempty"`
}

// FunctionCall contains function invocation details.
type FunctionCall struct {
	Name      ToolName `json:"name"`
	Arguments string   `json:"arguments"` // JSON string
}

// ============================================================================
// Factory Functions
// ============================================================================

func NewUserText(text string) InputItem {
	return InputItem{Type: InputItemTypeMessage, Role: RoleUser, Content: []ContentPart{TextPart(text)}}
}

func NewAssistantText(text string) InputItem {
	return InputItem{Type: InputItemTypeMessage, Role: RoleAssistant, Content: []ContentPart{TextPart(text)}}
}

func NewSystemText(text string) InputItem {
	return InputItem{Type: InputItemTypeMessage, Role: RoleSystem, Content: []ContentPart{TextPart(text)}}
}

func NewToolResultText(toolCallID ToolCallID, text string) InputItem {
	return InputItem{Type: InputItemTypeMessage, Role: RoleTool, ToolCallID: toolCallID, Content: []ContentPart{TextPart(text)}}
}

// NewToolCall creates a function tool call.
func NewToolCall(id ToolCallID, name ToolName, args string) ToolCall {
	return ToolCall{
		ID:       id,
		Type:     ToolTypeFunction,
		Function: NewFunctionCall(name, args),
	}
}

// NewFunctionCall creates a function call.
func NewFunctionCall(name ToolName, args string) *FunctionCall {
	return &FunctionCall{Name: name, Arguments: args}
}

// NewUsage creates a Usage with auto-calculated total if zero.
func NewUsage(input, output, total int64) Usage {
	if total == 0 {
		total = input + output
	}
	return Usage{
		InputTokens:  input,
		OutputTokens: output,
		TotalTokens:  total,
	}
}

// Citation represents a source from web search results.
type Citation struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// StreamEventKind represents the type of streaming event.
// This is the canonical definition - SDK implementations (TypeScript ChatEventType,
// Rust StreamEventKind) should be kept in sync with these values.
type StreamEventKind string

const (
	StreamEventKindUnknown        StreamEventKind = ""
	StreamEventKindMessageStart   StreamEventKind = "message_start"
	StreamEventKindMessageDelta   StreamEventKind = "message_delta"
	StreamEventKindMessageStop    StreamEventKind = "message_stop"
	StreamEventKindReasoningDelta StreamEventKind = "reasoning_delta"
	StreamEventKindToolUseStart   StreamEventKind = "tool_use_start"
	StreamEventKindToolUseDelta   StreamEventKind = "tool_use_delta"
	StreamEventKindToolUseStop    StreamEventKind = "tool_use_stop"
	StreamEventKindPing           StreamEventKind = "ping"
	StreamEventKindCustom         StreamEventKind = "custom"
)

// StreamEvent reflects a single provider event.
type StreamEvent struct {
	Kind       StreamEventKind
	Name       string
	Data       []byte // Raw provider data (deprecated: use normalized fields)
	Usage      *Usage
	ResponseID string
	Model      string
	StopReason string
	// TextDelta contains the text fragment for message_delta events.
	TextDelta string
	// ReasoningDelta contains reasoning/thinking content for reasoning_delta events.
	// This is emitted by reasoning models (e.g., Grok with reasoning enabled) before
	// the main text output. Consumers can choose to display or ignore this content.
	ReasoningDelta string
	// ToolCalls contains completed tool calls when Kind is tool_use_stop or message_stop.
	ToolCalls []ToolCall
	// ToolCallDelta contains incremental tool call data when Kind is tool_use_delta.
	ToolCallDelta *ToolCallDelta
}

// ToolCallDelta represents an incremental update to a tool call during streaming.
type ToolCallDelta struct {
	Index    int                `json:"index"`
	ID       ToolCallID         `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function *FunctionCallDelta `json:"function,omitempty"`
}

// FunctionCallDelta contains incremental function call data.
type FunctionCallDelta struct {
	Name      ToolName `json:"name,omitempty"`
	Arguments string   `json:"arguments,omitempty"` // Partial JSON string
}

// EventName returns the SSE event name that should be emitted for this event.
func (e StreamEvent) EventName() string {
	if e.Name != "" {
		return e.Name
	}
	return string(e.Kind)
}

// Stream allows providers to expose streaming responses.
type Stream interface {
	Next() (StreamEvent, bool, error)
	Close() error
}

// MarshalEvent is a helper to encode arbitrary payloads.
func MarshalEvent(payload any) []byte {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return data
}

// DeltaPayload is the typed structure for message_delta streaming events.
type DeltaPayload struct {
	Type  string    `json:"type"`
	Delta TextDelta `json:"delta"`
}

// TextDelta contains the text fragment for a delta event.
type TextDelta struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// StartPayload is the typed structure for message_start streaming events.
type StartPayload struct {
	Type    string        `json:"type"`
	Message *StartMessage `json:"message,omitempty"`
}

// StartMessage contains optional response metadata.
type StartMessage struct {
	ID    string `json:"id,omitempty"`
	Model string `json:"model,omitempty"`
}

// StopPayload is the typed structure for message_stop streaming events.
type StopPayload struct {
	Type       string `json:"type"`
	StopReason string `json:"stop_reason"`
}

// BuildDeltaPayload creates a typed delta payload for streaming.
func BuildDeltaPayload(text string) []byte {
	return MarshalEvent(DeltaPayload{
		Type: "message_delta",
		Delta: TextDelta{
			Type: "text_delta",
			Text: text,
		},
	})
}

// BuildStartPayload creates a typed start payload for streaming.
func BuildStartPayload(responseID, model string) []byte {
	payload := StartPayload{Type: "message_start"}
	if responseID != "" || model != "" {
		payload.Message = &StartMessage{
			ID:    responseID,
			Model: model,
		}
	}
	return MarshalEvent(payload)
}

// BuildStopPayload creates a typed stop payload for streaming.
func BuildStopPayload(reason string) []byte {
	return MarshalEvent(StopPayload{
		Type:       "message_stop",
		StopReason: reason,
	})
}

// UsageDeltaPayload is the typed structure for delta events with usage info.
type UsageDeltaPayload struct {
	Type  string `json:"type"`
	Usage Usage  `json:"usage"`
}

// BuildUsageDeltaPayload creates a typed usage delta payload for streaming.
func BuildUsageDeltaPayload(usage Usage) []byte {
	return MarshalEvent(UsageDeltaPayload{
		Type:  "message_delta",
		Usage: usage,
	})
}

// ToolUseStartPayload is the typed structure for tool_use_start streaming events.
type ToolUseStartPayload struct {
	Type     string `json:"type"`
	Index    int    `json:"index"`
	ID       string `json:"id"`
	ToolType string `json:"tool_type"`
	Name     string `json:"name,omitempty"`
}

// ToolUseDeltaPayload is the typed structure for tool_use_delta streaming events.
type ToolUseDeltaPayload struct {
	Type      string `json:"type"`
	Index     int    `json:"index"`
	Arguments string `json:"arguments,omitempty"` // Partial JSON string
}

// ToolUseStopPayload is the typed structure for tool_use_stop streaming events.
type ToolUseStopPayload struct {
	Type     string    `json:"type"`
	ToolCall *ToolCall `json:"tool_call,omitempty"`
}

// BuildToolUseStartPayload creates a tool_use_start payload for streaming.
func BuildToolUseStartPayload(index int, id, toolType, name string) []byte {
	return MarshalEvent(ToolUseStartPayload{
		Type:     "tool_use_start",
		Index:    index,
		ID:       id,
		ToolType: toolType,
		Name:     name,
	})
}

// BuildToolUseDeltaPayload creates a tool_use_delta payload for streaming.
func BuildToolUseDeltaPayload(index int, arguments string) []byte {
	return MarshalEvent(ToolUseDeltaPayload{
		Type:      "tool_use_delta",
		Index:     index,
		Arguments: arguments,
	})
}

// BuildToolUseStopPayload creates a tool_use_stop payload for streaming.
func BuildToolUseStopPayload(toolCall *ToolCall) []byte {
	return MarshalEvent(ToolUseStopPayload{
		Type:     "tool_use_stop",
		ToolCall: toolCall,
	})
}

// Provider exposes a specific LLM vendor.
type Provider interface {
	ID() string
	DisplayName() string // Human-friendly name (e.g., "Anthropic", "xAI")
	Capabilities() ProviderCapabilities
	CreateResponse(ctx context.Context, req ResponseRequest) (*Response, error)
	StreamResponse(ctx context.Context, req ResponseRequest) (Stream, error)
}

// ProviderInfo contains identifying metadata for a provider.
type ProviderInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// ProviderCapabilities advertise optional features per vendor.
type ProviderCapabilities struct {
	Streaming            bool                        `json:"streaming"`
	MaxOutputTokens      int64                       `json:"max_output_tokens"`
	SupportsSystemPrompt bool                        `json:"supports_system_prompt"`
	SupportsOutputFormat bool                        `json:"supports_output_format"`
	Tools                map[ToolType]ToolCapability `json:"tools,omitempty"`
}

// ToolCapability describes a tool's availability and configuration options.
type ToolCapability struct {
	Supported         bool           `json:"supported"`
	Options           []string       `json:"options,omitempty"`
	PricingPer1KCents *int           `json:"pricing_per_1k_cents,omitempty"`
	Web               *WebCapability `json:"web,omitempty"`
}

// WebCapability captures provider-level web abilities.
type WebCapability struct {
	SupportsSearch bool `json:"supports_search"`
	SupportsFetch  bool `json:"supports_fetch"`
}

// ValidateTools checks that all tools in the request are supported by this provider.
// Returns a ValidationError if any tool is unsupported.
func (c ProviderCapabilities) ValidateTools(tools []Tool) error {
	if len(tools) == 0 {
		return nil
	}
	if c.Tools == nil {
		return NewValidationError("provider does not support tools")
	}
	for _, t := range tools {
		capability, ok := c.Tools[t.Type]
		if !ok || !capability.Supported {
			return NewValidationError("provider does not support tool type %q", t.Type)
		}
	}
	return nil
}

// Context defines the subset of context.Context used by providers.
