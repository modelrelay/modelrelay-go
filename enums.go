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

// ModelID is a strongly-typed wrapper around model identifiers.
type ModelID string

// NewModelID constructs a model identifier from a raw string, trimming
// surrounding whitespace but otherwise preserving the value exactly.
func NewModelID(val string) ModelID {
	return ModelID(strings.TrimSpace(val))
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
	*m = NewModelID(raw)
	return nil
}
