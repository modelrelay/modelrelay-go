package sdk

import (
	"encoding/json"
	"strings"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
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

// SubscriptionStatusKind mirrors Stripe subscription lifecycle states.
type SubscriptionStatusKind string

const (
	SubscriptionStatusActive            SubscriptionStatusKind = "active"
	SubscriptionStatusTrialing          SubscriptionStatusKind = "trialing"
	SubscriptionStatusPastDue           SubscriptionStatusKind = "past_due"
	SubscriptionStatusCanceled          SubscriptionStatusKind = "canceled"
	SubscriptionStatusUnpaid            SubscriptionStatusKind = "unpaid"
	SubscriptionStatusIncomplete        SubscriptionStatusKind = "incomplete"
	SubscriptionStatusIncompleteExpired SubscriptionStatusKind = "incomplete_expired"
	SubscriptionStatusPaused            SubscriptionStatusKind = "paused"
)

// ParseSubscriptionStatus normalizes known statuses while preserving unknown values.
func ParseSubscriptionStatus(val string) SubscriptionStatusKind {
	normalized := strings.TrimSpace(strings.ToLower(val))
	switch normalized {
	case "":
		return ""
	case "active":
		return SubscriptionStatusActive
	case "trialing":
		return SubscriptionStatusTrialing
	case "past_due":
		return SubscriptionStatusPastDue
	case "canceled":
		return SubscriptionStatusCanceled
	case "unpaid":
		return SubscriptionStatusUnpaid
	case "incomplete":
		return SubscriptionStatusIncomplete
	case "incomplete_expired":
		return SubscriptionStatusIncompleteExpired
	case "paused":
		return SubscriptionStatusPaused
	default:
		return SubscriptionStatusKind(val)
	}
}

// IsActive reports whether the subscription should be treated as active.
func (s SubscriptionStatusKind) IsActive() bool {
	return s == SubscriptionStatusActive || s == SubscriptionStatusTrialing
}

func (s SubscriptionStatusKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

func (s *SubscriptionStatusKind) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = ParseSubscriptionStatus(raw)
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

// ModelCapability is a workflow-critical model capability identifier.
//
// This aliases the generated OpenAPI type to avoid drift between SDK and API.
type ModelCapability = generated.ModelCapability

const (
	ModelCapabilityTools       ModelCapability = "tools"
	ModelCapabilityVision      ModelCapability = "vision"
	ModelCapabilityWebSearch   ModelCapability = "web_search"
	ModelCapabilityWebFetch    ModelCapability = "web_fetch"
	ModelCapabilityComputerUse ModelCapability = "computer_use"
	ModelCapabilityCodeExec    ModelCapability = "code_execution"
)

// ParseModelCapability trims whitespace and preserves the raw value.
func ParseModelCapability(val string) ModelCapability {
	return ModelCapability(strings.TrimSpace(val))
}

// IsKnownModelCapability reports whether the capability is one of the known constants.
func IsKnownModelCapability(c ModelCapability) bool {
	switch c {
	case ModelCapabilityTools,
		ModelCapabilityVision,
		ModelCapabilityWebSearch,
		ModelCapabilityWebFetch,
		ModelCapabilityComputerUse,
		ModelCapabilityCodeExec:
		return true
	default:
		return strings.TrimSpace(string(c)) != ""
	}
}

// TierCode is a strongly-typed wrapper around tier codes (e.g., "free").
type TierCode string

// NewTierCode constructs a tier code from a raw string, trimming surrounding whitespace.
func NewTierCode(val string) TierCode {
	return TierCode(strings.TrimSpace(val))
}

// IsEmpty reports whether the tier code was left blank.
func (c TierCode) IsEmpty() bool {
	return strings.TrimSpace(string(c)) == ""
}

func (c TierCode) String() string { return string(c) }

func (c TierCode) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(c))
}

func (c *TierCode) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*c = NewTierCode(raw)
	return nil
}

// CustomerExternalID is a strongly-typed wrapper around caller-defined customer identifiers.
type CustomerExternalID string

// NewCustomerExternalID constructs an external customer id from a raw string, trimming whitespace.
func NewCustomerExternalID(val string) CustomerExternalID {
	return CustomerExternalID(strings.TrimSpace(val))
}

// IsEmpty reports whether the external id was left blank.
func (e CustomerExternalID) IsEmpty() bool {
	return strings.TrimSpace(string(e)) == ""
}

func (e CustomerExternalID) String() string { return string(e) }

func (e CustomerExternalID) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(e))
}

func (e *CustomerExternalID) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*e = NewCustomerExternalID(raw)
	return nil
}

// CustomerIdentityProvider identifies an external identity provider namespace (e.g. "oidc", "github", "oidc:https://issuer").
type CustomerIdentityProvider string

func NewCustomerIdentityProvider(val string) CustomerIdentityProvider {
	return CustomerIdentityProvider(strings.TrimSpace(val))
}

func (p CustomerIdentityProvider) IsEmpty() bool {
	return strings.TrimSpace(string(p)) == ""
}

func (p CustomerIdentityProvider) String() string { return string(p) }

func (p CustomerIdentityProvider) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(p))
}

func (p *CustomerIdentityProvider) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*p = NewCustomerIdentityProvider(raw)
	return nil
}

// CustomerIdentitySubject is the provider-scoped subject identifier (e.g. OIDC sub).
type CustomerIdentitySubject string

func NewCustomerIdentitySubject(val string) CustomerIdentitySubject {
	return CustomerIdentitySubject(strings.TrimSpace(val))
}

func (s CustomerIdentitySubject) IsEmpty() bool {
	return strings.TrimSpace(string(s)) == ""
}

func (s CustomerIdentitySubject) String() string { return string(s) }

func (s CustomerIdentitySubject) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

func (s *CustomerIdentitySubject) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*s = NewCustomerIdentitySubject(raw)
	return nil
}

// APIKeyKind encodes the kind of API key.
type APIKeyKind string

const (
	APIKeyKindSecret      APIKeyKind = "secret"
	APIKeyKindPublishable APIKeyKind = "publishable"
)

// ParseAPIKeyKind normalizes known kinds while preserving unknown values.
func ParseAPIKeyKind(val string) APIKeyKind {
	normalized := strings.TrimSpace(strings.ToLower(val))
	switch normalized {
	case "":
		return ""
	case "secret":
		return APIKeyKindSecret
	case "publishable":
		return APIKeyKindPublishable
	default:
		return APIKeyKind(val)
	}
}

func (k APIKeyKind) String() string { return string(k) }

func (k APIKeyKind) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(k))
}

func (k *APIKeyKind) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*k = ParseAPIKeyKind(raw)
	return nil
}
