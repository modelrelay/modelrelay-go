package sdk

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// WorkflowKind identifies the versioned workflow spec kind.
type WorkflowKind string

const (
	WorkflowKindV0 WorkflowKind = "workflow.v0"
)

type NodeID string

func NewNodeID(val string) NodeID { return NodeID(strings.TrimSpace(val)) }
func (id NodeID) String() string  { return string(id) }
func (id NodeID) Valid() bool     { return strings.TrimSpace(string(id)) != "" }

type OutputName string

func NewOutputName(val string) OutputName { return OutputName(strings.TrimSpace(val)) }
func (n OutputName) String() string       { return string(n) }
func (n OutputName) Valid() bool          { return strings.TrimSpace(string(n)) != "" }

// JSONPointer is an RFC 6901 JSON Pointer.
// The SDK does not validate semantics beyond basic formatting.
type JSONPointer string

func NewJSONPointer(val string) JSONPointer { return JSONPointer(strings.TrimSpace(val)) }
func (p JSONPointer) String() string        { return string(p) }
func (p JSONPointer) Valid() bool {
	if strings.TrimSpace(string(p)) == "" {
		return true
	}
	return strings.HasPrefix(string(p), "/")
}

type WorkflowNodeType string

const (
	WorkflowNodeTypeLLMResponses  WorkflowNodeType = "llm.responses"
	WorkflowNodeTypeJoinAll       WorkflowNodeType = "join.all"
	WorkflowNodeTypeTransformJSON WorkflowNodeType = "transform.json"
)

type ToolExecutionModeV0 string

const (
	ToolExecutionModeServer ToolExecutionModeV0 = "server"
	ToolExecutionModeClient ToolExecutionModeV0 = "client"
)

type ToolExecutionV0 struct {
	Mode ToolExecutionModeV0 `json:"mode"`
}

type LLMResponsesToolLimitsV0 struct {
	MaxLLMCallsPerNode  *int64 `json:"max_llm_calls,omitempty"`
	MaxToolCallsPerStep *int64 `json:"max_tool_calls_per_step,omitempty"`
	WaitTTLMS           *int64 `json:"wait_ttl_ms,omitempty"`
}

type LLMResponsesBindingEncodingV0 string

const (
	LLMResponsesBindingEncodingJSON       LLMResponsesBindingEncodingV0 = "json"
	LLMResponsesBindingEncodingJSONString LLMResponsesBindingEncodingV0 = "json_string"
)

type LLMResponsesBindingV0 struct {
	From     NodeID                        `json:"from"`
	Pointer  JSONPointer                   `json:"pointer,omitempty"`
	To       JSONPointer                   `json:"to"`
	Encoding LLMResponsesBindingEncodingV0 `json:"encoding,omitempty"`
}

type WorkflowExecutionV0 struct {
	MaxParallelism *int64 `json:"max_parallelism,omitempty"`
	NodeTimeoutMS  *int64 `json:"node_timeout_ms,omitempty"`
	RunTimeoutMS   *int64 `json:"run_timeout_ms,omitempty"`
}

type WorkflowNodeV0 struct {
	ID    NodeID           `json:"id"`
	Type  WorkflowNodeType `json:"type"`
	Input json.RawMessage  `json:"input,omitempty"`
}

type WorkflowEdgeV0 struct {
	From NodeID `json:"from"`
	To   NodeID `json:"to"`
}

type WorkflowOutputRefV0 struct {
	Name    OutputName  `json:"name"`
	From    NodeID      `json:"from"`
	Pointer JSONPointer `json:"pointer,omitempty"`
}

// WorkflowSpecV0 is the request payload shape for workflow.v0.
type WorkflowSpecV0 struct {
	Kind      WorkflowKind          `json:"kind"`
	Name      string                `json:"name,omitempty"`
	Execution *WorkflowExecutionV0  `json:"execution,omitempty"`
	Nodes     []WorkflowNodeV0      `json:"nodes"`
	Edges     []WorkflowEdgeV0      `json:"edges,omitempty"`
	Outputs   []WorkflowOutputRefV0 `json:"outputs"`
}

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

// ResponseID is a stable identifier for a /responses completion.
//
// Response IDs are opaque strings and should be treated as such.
type ResponseID string

func (id ResponseID) String() string { return string(id) }

func (id ResponseID) Valid() bool { return strings.TrimSpace(string(id)) != "" }

func ParseResponseID(raw string) (ResponseID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("response_id required")
	}
	return ResponseID(raw), nil
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

type RunEventTypeV0 string

const (
	RunEventEnvelopeVersionV0 = "v0"

	// ArtifactKeyNodeOutputV0 is the artifact key for a node's final output payload.
	ArtifactKeyNodeOutputV0 = "node_output.v0"
	// ArtifactKeyRunOutputsV0 is the artifact key for a run's final exported outputs payload.
	ArtifactKeyRunOutputsV0 = "run_outputs.v0"

	RunEventRunCompiled  RunEventTypeV0 = "run_compiled"
	RunEventRunStarted   RunEventTypeV0 = "run_started"
	RunEventRunCompleted RunEventTypeV0 = "run_completed"
	RunEventRunFailed    RunEventTypeV0 = "run_failed"
	RunEventRunCanceled  RunEventTypeV0 = "run_canceled"

	RunEventNodeLLMCall    RunEventTypeV0 = "node_llm_call"
	RunEventNodeToolCall   RunEventTypeV0 = "node_tool_call"
	RunEventNodeToolResult RunEventTypeV0 = "node_tool_result"
	RunEventNodeWaiting    RunEventTypeV0 = "node_waiting"

	RunEventNodeStarted     RunEventTypeV0 = "node_started"
	RunEventNodeSucceeded   RunEventTypeV0 = "node_succeeded"
	RunEventNodeFailed      RunEventTypeV0 = "node_failed"
	RunEventNodeOutputDelta RunEventTypeV0 = "node_output_delta"
	RunEventNodeOutput      RunEventTypeV0 = "node_output"
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

type NodeOutputDeltaV0 struct {
	Kind StreamEventKind `json:"kind"`

	TextDelta  string `json:"text_delta,omitempty"`
	ResponseID string `json:"response_id,omitempty"`
	Model      string `json:"model,omitempty"`
}

type TokenUsageV0 struct {
	InputTokens  int64 `json:"input_tokens,omitempty"`
	OutputTokens int64 `json:"output_tokens,omitempty"`
	TotalTokens  int64 `json:"total_tokens,omitempty"`
}

type NodeLLMCallV0 struct {
	Step      int64  `json:"step"`
	RequestID string `json:"request_id"`

	Provider   string       `json:"provider,omitempty"`
	Model      string       `json:"model,omitempty"`
	ResponseID string       `json:"response_id,omitempty"`
	StopReason string       `json:"stop_reason,omitempty"`
	Usage      TokenUsageV0 `json:"usage,omitempty"`
}

type FunctionToolCallV0 struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type NodeToolCallV0 struct {
	Step      int64              `json:"step"`
	RequestID string             `json:"request_id"`
	ToolCall  FunctionToolCallV0 `json:"tool_call"`
}

type NodeToolResultV0 struct {
	Step       int64  `json:"step"`
	RequestID  string `json:"request_id"`
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name"`
	Output     string `json:"output"`
}

type PendingToolCallV0 struct {
	ToolCallID string `json:"tool_call_id"`
	Name       string `json:"name"`
	Arguments  string `json:"arguments"`
}

type NodeWaitingV0 struct {
	Step             int64               `json:"step"`
	RequestID        string              `json:"request_id"`
	PendingToolCalls []PendingToolCallV0 `json:"pending_tool_calls"`
	Reason           string              `json:"reason"`
}

// RunEventV0Envelope is the stable, append-only wire envelope for workflow run history.
type RunEventV0Envelope struct {
	EnvelopeVersion string         `json:"envelope_version"`
	RunID           RunID          `json:"run_id"`
	Seq             int64          `json:"seq"`
	TS              time.Time      `json:"ts"`
	Type            RunEventTypeV0 `json:"type"`

	NodeID NodeID `json:"node_id,omitempty"`

	PlanHash *PlanHash    `json:"plan_hash,omitempty"`
	Error    *NodeErrorV0 `json:"error,omitempty"`

	LLMCall    *NodeLLMCallV0    `json:"llm_call,omitempty"`
	ToolCall   *NodeToolCallV0   `json:"tool_call,omitempty"`
	ToolResult *NodeToolResultV0 `json:"tool_result,omitempty"`
	Waiting    *NodeWaitingV0    `json:"waiting,omitempty"`

	Delta *NodeOutputDeltaV0 `json:"delta,omitempty"`

	OutputInfo  *PayloadInfoV0 `json:"output_info,omitempty"`
	ArtifactKey string         `json:"artifact_key,omitempty"`

	OutputsArtifactKey string         `json:"outputs_artifact_key,omitempty"`
	OutputsInfo        *PayloadInfoV0 `json:"outputs_info,omitempty"`
}

type PayloadInfoV0 struct {
	Bytes    int64  `json:"bytes"`
	SHA256   string `json:"sha256"`
	Included bool   `json:"included"`
}

type RunStatusV0 string

const (
	RunStatusRunning   RunStatusV0 = "running"
	RunStatusWaiting   RunStatusV0 = "waiting"
	RunStatusSucceeded RunStatusV0 = "succeeded"
	RunStatusFailed    RunStatusV0 = "failed"
	RunStatusCanceled  RunStatusV0 = "canceled"
)

type NodeStatusV0 string

const (
	NodeStatusPending   NodeStatusV0 = "pending"
	NodeStatusRunning   NodeStatusV0 = "running"
	NodeStatusWaiting   NodeStatusV0 = "waiting"
	NodeStatusSucceeded NodeStatusV0 = "succeeded"
	NodeStatusFailed    NodeStatusV0 = "failed"
	NodeStatusCanceled  NodeStatusV0 = "canceled"
)

type NodeErrorV0 struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"` // Raw error details from the provider
}

type NodeResultV0 struct {
	ID        NodeID           `json:"id"`
	Type      WorkflowNodeType `json:"type"`
	Status    NodeStatusV0     `json:"status"`
	StartedAt time.Time        `json:"started_at,omitempty"`
	EndedAt   time.Time        `json:"ended_at,omitempty"`

	Output json.RawMessage `json:"output,omitempty"`
	Error  *NodeErrorV0    `json:"error,omitempty"`
}

type RunCostSummaryV0 struct {
	TotalUSDCents int64               `json:"total_usd_cents"`
	LineItems     []RunCostLineItemV0 `json:"line_items"`
}

type RunCostLineItemV0 struct {
	ProviderID   ProviderID `json:"provider_id"`
	Model        ModelID    `json:"model"`
	Requests     int64      `json:"requests"`
	InputTokens  int64      `json:"input_tokens"`
	OutputTokens int64      `json:"output_tokens"`
	USDCents     int64      `json:"usd_cents"`
}

type WorkflowIssue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// WorkflowValidationError is returned by workflow compilation/validation endpoints.
type WorkflowValidationError struct {
	Issues []WorkflowIssue `json:"issues"`
}

func (e WorkflowValidationError) Error() string {
	if len(e.Issues) == 0 {
		return "workflow spec invalid"
	}
	if len(e.Issues) == 1 {
		return "workflow spec invalid: " + e.Issues[0].Message
	}
	return fmt.Sprintf("workflow spec invalid: %d issues", len(e.Issues))
}
