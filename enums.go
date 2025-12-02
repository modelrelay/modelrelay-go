package sdk

import (
	"encoding/json"
	"strings"
)

// StopReason encodes the reason a generation ended.
type StopReason string

const (
	StopReasonCompleted     StopReason = "completed"
	StopReasonStop          StopReason = "stop"
	StopReasonStopSequence  StopReason = "stop_sequence"
	StopReasonEndTurn       StopReason = "end_turn"
	StopReasonMaxTokens     StopReason = "max_tokens"
	StopReasonMaxLength     StopReason = "max_len"
	StopReasonMaxContext    StopReason = "max_context"
	StopReasonToolCalls     StopReason = "tool_calls"
	StopReasonTimeLimit     StopReason = "time_limit"
	StopReasonContentFilter StopReason = "content_filter"
	StopReasonIncomplete    StopReason = "incomplete"
	StopReasonUnknown       StopReason = "unknown"
)

// ParseStopReason normalizes known stop reasons while keeping vendor-specific values.
func ParseStopReason(val string) StopReason {
	normalized := strings.TrimSpace(strings.ToLower(val))
	switch normalized {
	case "":
		return ""
	case "completed":
		return StopReasonCompleted
	case "stop":
		return StopReasonStop
	case "stop_sequence":
		return StopReasonStopSequence
	case "end_turn":
		return StopReasonEndTurn
	case "max_tokens":
		return StopReasonMaxTokens
	case "max_len", "length":
		return StopReasonMaxLength
	case "max_context":
		return StopReasonMaxContext
	case "tool_calls":
		return StopReasonToolCalls
	case "time_limit":
		return StopReasonTimeLimit
	case "content_filter":
		return StopReasonContentFilter
	case "incomplete":
		return StopReasonIncomplete
	case "unknown":
		return StopReasonUnknown
	default:
		return StopReason(val)
	}
}

// IsOther reports whether the value is not one of the known constants.
func (s StopReason) IsOther() bool {
	switch s {
	case StopReasonCompleted, StopReasonStop, StopReasonStopSequence, StopReasonEndTurn,
		StopReasonMaxTokens, StopReasonMaxLength, StopReasonMaxContext, StopReasonToolCalls,
		StopReasonTimeLimit, StopReasonContentFilter, StopReasonIncomplete, StopReasonUnknown:
		return false
	default:
		return strings.TrimSpace(string(s)) != ""
	}
}

func (s StopReason) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

func (s *StopReason) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = ParseStopReason(raw)
	return nil
}

// ProviderID enumerates known providers with an escape hatch for custom ids.
type ProviderID string

const (
	ProviderOpenAI     ProviderID = "openai"
	ProviderAnthropic  ProviderID = "anthropic"
	ProviderGrok       ProviderID = "grok"
	ProviderOpenRouter ProviderID = "openrouter"
	ProviderEcho       ProviderID = "echo"
)

// ParseProviderID normalizes known providers and preserves custom identifiers.
func ParseProviderID(val string) ProviderID {
	trimmed := strings.TrimSpace(val)
	switch strings.ToLower(trimmed) {
	case "":
		return ""
	case "openai":
		return ProviderOpenAI
	case "anthropic":
		return ProviderAnthropic
	case "grok":
		return ProviderGrok
	case "openrouter":
		return ProviderOpenRouter
	case "echo":
		return ProviderEcho
	default:
		return ProviderID(trimmed)
	}
}

// IsOther reports whether the provider is not one of the built-in constants.
func (p ProviderID) IsOther() bool {
	switch p {
	case ProviderOpenAI, ProviderAnthropic, ProviderGrok, ProviderOpenRouter, ProviderEcho:
		return false
	default:
		return strings.TrimSpace(string(p)) != ""
	}
}

// IsEmpty reports whether the provider is unset.
func (p ProviderID) IsEmpty() bool {
	return strings.TrimSpace(string(p)) == ""
}

func (p ProviderID) String() string {
	return string(p)
}

func (p ProviderID) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(p))
}

func (p *ProviderID) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*p = ParseProviderID(raw)
	return nil
}

// ModelID enumerates common model identifiers while allowing arbitrary ids.
type ModelID string

const (
	ModelOpenAIGPT4o                   ModelID = "openai/gpt-4o"
	ModelOpenAIGPT4oMini               ModelID = "openai/gpt-4o-mini"
	ModelOpenAIGPT51                   ModelID = "openai/gpt-5.1"
	ModelAnthropicClaude35HaikuLatest  ModelID = "anthropic/claude-3-5-haiku-latest"
	ModelAnthropicClaude35SonnetLatest ModelID = "anthropic/claude-3-5-sonnet-latest"
	ModelAnthropicClaudeOpus45         ModelID = "anthropic/claude-opus-4-5-20251101"
	ModelAnthropicClaude35Haiku        ModelID = "anthropic/claude-3.5-haiku"
	ModelGrok2                         ModelID = "grok-2"
	ModelGrok4_1FastNonReasoning       ModelID = "grok-4-1-fast-non-reasoning"
	ModelGrok4_1FastReasoning          ModelID = "grok-4-1-fast-reasoning"
	ModelEcho1                         ModelID = "echo-1"
)

// ParseModelID normalizes well-known models and preserves custom identifiers.
func ParseModelID(val string) ModelID {
	trimmed := strings.TrimSpace(val)
	switch strings.ToLower(trimmed) {
	case "":
		return ""
	case "openai/gpt-4o":
		return ModelOpenAIGPT4o
	case "openai/gpt-4o-mini":
		return ModelOpenAIGPT4oMini
	case "openai/gpt-5.1":
		return ModelOpenAIGPT51
	case "anthropic/claude-3-5-haiku-latest":
		return ModelAnthropicClaude35HaikuLatest
	case "anthropic/claude-3-5-sonnet-latest":
		return ModelAnthropicClaude35SonnetLatest
	case "anthropic/claude-opus-4-5-20251101":
		return ModelAnthropicClaudeOpus45
	case "anthropic/claude-3.5-haiku":
		return ModelAnthropicClaude35Haiku
	case "grok-2":
		return ModelGrok2
	case "grok-4-1-fast-non-reasoning":
		return ModelGrok4_1FastNonReasoning
	case "grok-4-1-fast-reasoning":
		return ModelGrok4_1FastReasoning
	case "echo-1":
		return ModelEcho1
	default:
		return ModelID(trimmed)
	}
}

// IsOther reports whether the model is not one of the built-in constants.
func (m ModelID) IsOther() bool {
	switch m {
	case ModelOpenAIGPT4o, ModelOpenAIGPT4oMini, ModelOpenAIGPT51, ModelAnthropicClaude35HaikuLatest,
		ModelAnthropicClaude35SonnetLatest, ModelAnthropicClaudeOpus45, ModelAnthropicClaude35Haiku,
		ModelGrok2, ModelGrok4_1FastNonReasoning, ModelGrok4_1FastReasoning, ModelEcho1:
		return false
	default:
		return strings.TrimSpace(string(m)) != ""
	}
}

// IsEmpty reports whether the model was left blank.
func (m ModelID) IsEmpty() bool {
	return strings.TrimSpace(string(m)) == ""
}

func (m ModelID) String() string {
	return string(m)
}

func (m ModelID) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(m))
}

func (m *ModelID) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = ParseModelID(raw)
	return nil
}
