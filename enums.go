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
	ProviderOpenAI         ProviderID = "openai"
	ProviderAnthropic      ProviderID = "anthropic"
	ProviderGoogleAIStudio ProviderID = "google-ai-studio"
	ProviderXAI            ProviderID = "xai"
	ProviderEcho           ProviderID = "echo"
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
	case "google-ai-studio":
		return ProviderGoogleAIStudio
	case "xai":
		return ProviderXAI
	case "echo":
		return ProviderEcho
	default:
		return ProviderID(trimmed)
	}
}

// IsOther reports whether the provider is not one of the built-in constants.
func (p ProviderID) IsOther() bool {
	switch p {
	case ProviderOpenAI, ProviderAnthropic, ProviderGoogleAIStudio, ProviderXAI, ProviderEcho:
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
	*p = ProviderID(raw)
	return nil
}

// ModelID enumerates common model identifiers while allowing arbitrary ids.
type ModelID string

const (
	// OpenAI models (provider-agnostic identifiers)
	ModelGPT4o     ModelID = "gpt-4o"
	ModelGPT4oMini ModelID = "gpt-4o-mini"
	ModelGPT51     ModelID = "gpt-5.1"

	// Anthropic models (provider-agnostic identifiers)
	ModelClaude3_5HaikuLatest  ModelID = "claude-3-5-haiku-latest"
	ModelClaude3_5SonnetLatest ModelID = "claude-3-5-sonnet-latest"
	// Claude Opus 4.5 (short identifier; older dated ids are treated as legacy).
	ModelClaudeOpus4_5  ModelID = "claude-opus-4-5"
	ModelClaude3_5Haiku ModelID = "claude-3.5-haiku"

	// xAI / Grok models
	ModelGrok2                   ModelID = "grok-2"
	ModelGrok4_1FastNonReasoning ModelID = "grok-4-1-fast-non-reasoning"
	ModelGrok4_1FastReasoning    ModelID = "grok-4-1-fast-reasoning"

	// Google AI Studio / Gemini models
	ModelGemini3ProPreview ModelID = "gemini-3-pro-preview"

	// Internal echo model used for testing.
	ModelEcho1 ModelID = "echo-1"
)

// ParseModelID normalizes well-known models and preserves custom identifiers.
func ParseModelID(val string) ModelID {
	trimmed := strings.TrimSpace(val)
	switch strings.ToLower(trimmed) {
	case "":
		return ""
	// OpenAI – provider-agnostic identifiers only.
	case "gpt-4o":
		return ModelGPT4o
	case "gpt-4o-mini":
		return ModelGPT4oMini
	case "gpt-5.1":
		return ModelGPT51

	// Anthropic – provider-agnostic identifiers only.
	case "claude-3-5-haiku-latest":
		return ModelClaude3_5HaikuLatest
	case "claude-3-5-sonnet-latest":
		return ModelClaude3_5SonnetLatest
	case "claude-opus-4-5":
		return ModelClaudeOpus4_5
	case "claude-3.5-haiku":
		return ModelClaude3_5Haiku

	// Google AI Studio / Gemini.
	case "gemini-3-pro-preview":
		return ModelGemini3ProPreview

	// xAI / Grok – already provider-agnostic.
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
	case ModelGPT4o, ModelGPT4oMini, ModelGPT51, ModelClaude3_5HaikuLatest,
		ModelClaude3_5SonnetLatest, ModelClaudeOpus4_5, ModelClaude3_5Haiku,
		ModelGrok2, ModelGrok4_1FastNonReasoning, ModelGrok4_1FastReasoning,
		ModelGemini3ProPreview, ModelEcho1:
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

// IsKnown reports whether the model is one of the built-in SDK constants.
// Callers that need to support custom models should construct ModelID directly
// instead of relying on ParseModelID.
func (m ModelID) IsKnown() bool {
	switch m {
	case "", // empty => let server apply tier defaults
		ModelGPT4o, ModelGPT4oMini, ModelGPT51,
		ModelClaude3_5HaikuLatest, ModelClaude3_5SonnetLatest, ModelClaudeOpus4_5, ModelClaude3_5Haiku,
		ModelGrok2, ModelGrok4_1FastNonReasoning, ModelGrok4_1FastReasoning,
		ModelGemini3ProPreview,
		ModelEcho1:
		return true
	default:
		return false
	}
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
