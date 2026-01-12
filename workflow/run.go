package workflow

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

// RunID is the workflow run identifier.
type RunID string

func NewRunID() RunID { return RunID(uuid.NewString()) }
func (id RunID) String() string {
	return string(id)
}
func (id RunID) Valid() bool {
	_, err := uuid.Parse(strings.TrimSpace(string(id)))
	return err == nil
}

func ParseRunID(raw string) (RunID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("run_id required")
	}
	if _, err := uuid.Parse(raw); err != nil {
		return "", fmt.Errorf("invalid run_id: %w", err)
	}
	return RunID(raw), nil
}

// PlanHash is the hash of a compiled workflow plan (hex-encoded sha256).
type PlanHash string

func (h PlanHash) String() string { return string(h) }

func (h PlanHash) Valid() bool {
	_, err := ParsePlanHash(string(h))
	return err == nil
}

func ParsePlanHash(raw string) (PlanHash, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("plan_hash required")
	}
	if len(raw) != 64 {
		return "", errors.New("invalid plan_hash")
	}
	if _, err := hex.DecodeString(raw); err != nil {
		return "", errors.New("invalid plan_hash")
	}
	return PlanHash(raw), nil
}

// StatusV0 represents the status of a workflow run.
type StatusV0 string

const (
	StatusRunning   StatusV0 = "running"
	StatusWaiting   StatusV0 = "waiting"
	StatusSucceeded StatusV0 = "succeeded"
	StatusFailed    StatusV0 = "failed"
	StatusCanceled  StatusV0 = "canceled"
)

// NodeStatus represents the status of a workflow node.
type NodeStatus string

const (
	NodeStatusPending   NodeStatus = "pending"
	NodeStatusRunning   NodeStatus = "running"
	NodeStatusWaiting   NodeStatus = "waiting"
	NodeStatusSucceeded NodeStatus = "succeeded"
	NodeStatusFailed    NodeStatus = "failed"
	NodeStatusCanceled  NodeStatus = "canceled"
)

// EventTypeV0 identifies the type of a workflow run event.
type EventTypeV0 string

const (
	EventEnvelopeVersionV0 = "v2"

	// ArtifactKeyNodeOutputV0 is the artifact key for a node's final output payload.
	ArtifactKeyNodeOutputV0 = "node_output.v0"
	// ArtifactKeyRunOutputsV0 is the artifact key for a run's final exported outputs payload.
	ArtifactKeyRunOutputsV0 = "run_outputs.v0"

	EventRunCompiled  EventTypeV0 = "run_compiled"
	EventRunStarted   EventTypeV0 = "run_started"
	EventRunCompleted EventTypeV0 = "run_completed"
	EventRunFailed    EventTypeV0 = "run_failed"
	EventRunCanceled  EventTypeV0 = "run_canceled"

	EventNodeLLMCall    EventTypeV0 = "node_llm_call"
	EventNodeToolCall   EventTypeV0 = "node_tool_call"
	EventNodeToolResult EventTypeV0 = "node_tool_result"
	EventNodeWaiting    EventTypeV0 = "node_waiting"
	EventNodeUserAsk    EventTypeV0 = "node_user_ask"
	EventNodeUserAnswer EventTypeV0 = "node_user_answer"

	EventNodeStarted     EventTypeV0 = "node_started"
	EventNodeSucceeded   EventTypeV0 = "node_succeeded"
	EventNodeFailed      EventTypeV0 = "node_failed"
	EventNodeOutputDelta EventTypeV0 = "node_output_delta"
	EventNodeOutput      EventTypeV0 = "node_output"
)

// StreamEventKind represents the type of streaming event from an LLM provider.
type StreamEventKind string

const (
	StreamEventKindMessageStart StreamEventKind = "message_start"
	StreamEventKindMessageDelta StreamEventKind = "message_delta"
	StreamEventKindMessageStop  StreamEventKind = "message_stop"
	StreamEventKindToolUseStart StreamEventKind = "tool_use_start"
	StreamEventKindToolUseDelta StreamEventKind = "tool_use_delta"
	StreamEventKindToolUseStop  StreamEventKind = "tool_use_stop"
)

// PayloadInfo contains metadata about an artifact payload.
type PayloadInfo struct {
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
	Included bool   `json:"included"`
}

// PayloadArtifact identifies an artifact payload and its metadata.
type PayloadArtifact struct {
	ArtifactKey string      `json:"artifact_key"`
	Info        PayloadInfo `json:"info"`
}

// NodeError represents an error from a node execution.
type NodeError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"` // Raw error details from the provider
}

// TokenUsage contains token usage statistics.
type TokenUsage struct {
	InputTokens  int64 `json:"input_tokens,omitempty"`
	OutputTokens int64 `json:"output_tokens,omitempty"`
	TotalTokens  int64 `json:"total_tokens,omitempty"`
}

// NodeOutputDelta contains streaming output delta from a node.
type NodeOutputDelta struct {
	Kind StreamEventKind `json:"kind"`

	TextDelta  string `json:"text_delta,omitempty"`
	ResponseID string `json:"response_id,omitempty"`
	Model      string `json:"model,omitempty"`
}

// NodeLLMCall contains information about an LLM call within a node.
type NodeLLMCall struct {
	Step      int64  `json:"step"`
	RequestID string `json:"request_id"`

	Provider   string     `json:"provider,omitempty"`
	Model      string     `json:"model,omitempty"`
	ResponseID string     `json:"response_id,omitempty"`
	StopReason string     `json:"stop_reason,omitempty"`
	Usage      TokenUsage `json:"usage,omitempty"`
}

// NodeResult contains the result of a node execution.
type NodeResult struct {
	ID        NodeID     `json:"id"`
	Type      NodeTypeV1 `json:"type"`
	Status    NodeStatus `json:"status"`
	StartedAt time.Time  `json:"started_at,omitempty"`
	EndedAt   time.Time  `json:"ended_at,omitempty"`

	Output json.RawMessage `json:"output,omitempty"`
	Error  *NodeError      `json:"error,omitempty"`
}

// EventV0Envelope is the stable, append-only wire envelope for workflow run history.
type EventV0Envelope struct {
	EnvelopeVersion string      `json:"envelope_version"`
	RunID           RunID       `json:"run_id"`
	Seq             int64       `json:"seq"`
	TS              time.Time   `json:"ts"`
	Type            EventTypeV0 `json:"type"`

	NodeID NodeID `json:"node_id,omitempty"`

	PlanHash *PlanHash  `json:"plan_hash,omitempty"`
	Error    *NodeError `json:"error,omitempty"`

	LLMCall    *NodeLLMCall    `json:"llm_call,omitempty"`
	ToolCall   *NodeToolCall   `json:"tool_call,omitempty"`
	ToolResult *NodeToolResult `json:"tool_result,omitempty"`
	Waiting    *NodeWaiting    `json:"waiting,omitempty"`
	UserAsk    *NodeUserAsk    `json:"user_ask,omitempty"`
	UserAnswer *NodeUserAnswer `json:"user_answer,omitempty"`

	Delta *NodeOutputDelta `json:"delta,omitempty"`

	Output  *PayloadArtifact `json:"output,omitempty"`
	Outputs *PayloadArtifact `json:"outputs,omitempty"`
}

// ToolCallID is a unique identifier for a tool call.
// This is an alias to llm.ToolCallID.
type ToolCallID = llm.ToolCallID

// ToolName is the name of a tool.
// This is an alias to llm.ToolName.
type ToolName = llm.ToolName

// ToolCall represents a tool call reference in run payloads.
// Arguments are optional and omitted when absent.
type ToolCall struct {
	ID        ToolCallID `json:"id"`
	Name      ToolName   `json:"name"`
	Arguments string     `json:"arguments,omitempty"`
}

// ToolCallWithArguments represents a tool call that must include arguments.
type ToolCallWithArguments struct {
	ID        ToolCallID `json:"id"`
	Name      ToolName   `json:"name"`
	Arguments string     `json:"arguments"`
}

// NodeToolCall contains information about a tool call within a node.
type NodeToolCall struct {
	Step      int64                 `json:"step"`
	RequestID string                `json:"request_id"`
	ToolCall  ToolCallWithArguments `json:"tool_call"`
}

// NodeToolResult contains the result of a tool call.
type NodeToolResult struct {
	Step      int64    `json:"step"`
	RequestID string   `json:"request_id"`
	ToolCall  ToolCall `json:"tool_call"`
	Output    string   `json:"output"`
	Error     string   `json:"error,omitempty"`
}

// PendingToolCall represents a pending tool call awaiting execution.
type PendingToolCall struct {
	ToolCall ToolCallWithArguments `json:"tool_call"`
}

// NodeWaiting contains information about a node waiting for tool results.
type NodeWaiting struct {
	Step             int64             `json:"step"`
	RequestID        string            `json:"request_id"`
	PendingToolCalls []PendingToolCall `json:"pending_tool_calls"`
	Reason           string            `json:"reason"`
}

// UserAskOption is a multiple-choice option for user.ask.
type UserAskOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// NodeUserAsk captures a user.ask prompt emitted by a node.
type NodeUserAsk struct {
	Step          int64                 `json:"step"`
	RequestID     string                `json:"request_id"`
	ToolCall      ToolCallWithArguments `json:"tool_call"`
	Question      string                `json:"question"`
	Options       []UserAskOption       `json:"options,omitempty"`
	AllowFreeform bool                  `json:"allow_freeform"`
}

// NodeUserAnswer captures the user's response to a user.ask prompt.
type NodeUserAnswer struct {
	Step       int64    `json:"step"`
	RequestID  string   `json:"request_id"`
	ToolCall   ToolCall `json:"tool_call"`
	Answer     string   `json:"answer"`
	IsFreeform bool     `json:"is_freeform"`
}

// ProviderID identifies an LLM provider.
type ProviderID string

func (id ProviderID) String() string { return string(id) }

// ModelID identifies a model within a provider.
type ModelID string

func (id ModelID) String() string { return string(id) }

// CostSummaryV0 summarizes the cost of a workflow run.
type CostSummaryV0 struct {
	TotalUSDCents int64            `json:"total_usd_cents"`
	LineItems     []CostLineItemV0 `json:"line_items"`
}

// CostLineItemV0 represents a line item in a cost summary.
type CostLineItemV0 struct {
	ProviderID   ProviderID `json:"provider_id"`
	Model        ModelID    `json:"model"`
	Requests     int64      `json:"requests"`
	InputTokens  int64      `json:"input_tokens"`
	OutputTokens int64      `json:"output_tokens"`
	USDCents     int64      `json:"usd_cents"`
}
