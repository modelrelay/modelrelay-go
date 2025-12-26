package sdk

import (
	"errors"
	"strings"

	"github.com/modelrelay/modelrelay/sdk/go/workflow"
)

// Re-export workflow types for internal SDK use.
// External users should prefer importing the workflow package directly:
//
//	import "github.com/modelrelay/modelrelay/sdk/go/workflow"
//	spec := workflow.SpecV0{...}
type (
	WorkflowKind              = workflow.Kind
	WorkflowNodeType          = workflow.NodeType
	NodeID                    = workflow.NodeID
	OutputName                = workflow.OutputName
	JSONPointer               = workflow.JSONPointer
	WorkflowExecutionV0       = workflow.ExecutionV0
	WorkflowNodeV0            = workflow.NodeV0
	WorkflowEdgeV0            = workflow.EdgeV0
	WorkflowOutputRefV0       = workflow.OutputRefV0
	WorkflowSpecV0            = workflow.SpecV0
	WorkflowIssue             = workflow.Issue
	WorkflowValidationError   = workflow.ValidationError
	ToolExecutionModeV0       = workflow.ToolExecutionModeV0
	ToolExecutionV0           = workflow.ToolExecutionV0
	LLMResponsesToolLimitsV0  = workflow.LLMResponsesToolLimitsV0
	LLMResponsesBindingEncodingV0 = workflow.LLMResponsesBindingEncodingV0
	LLMResponsesBindingV0     = workflow.LLMResponsesBindingV0
	RunID                     = workflow.RunID
	PlanHash                  = workflow.PlanHash
	RunEventTypeV0            = workflow.EventTypeV0
	RunStatusV0               = workflow.StatusV0
	NodeStatusV0              = workflow.NodeStatusV0
	NodeErrorV0               = workflow.NodeErrorV0
	NodeResultV0              = workflow.NodeResultV0
	RunEventV0Envelope        = workflow.EventV0Envelope
	PayloadInfoV0             = workflow.PayloadInfoV0
	TokenUsageV0              = workflow.TokenUsageV0
	NodeOutputDeltaV0         = workflow.NodeOutputDeltaV0
	NodeLLMCallV0             = workflow.NodeLLMCallV0
	FunctionToolCallV0        = workflow.FunctionToolCallV0
	NodeToolCallV0            = workflow.NodeToolCallV0
	NodeToolResultV0          = workflow.NodeToolResultV0
	PendingToolCallV0         = workflow.PendingToolCallV0
	NodeWaitingV0             = workflow.NodeWaitingV0
	RunCostSummaryV0          = workflow.CostSummaryV0
	RunCostLineItemV0         = workflow.CostLineItemV0
	StreamEventKind           = workflow.StreamEventKind
)

// Re-export workflow constants.
const (
	WorkflowKindV0                        = workflow.KindV0
	WorkflowNodeTypeLLMResponses          = workflow.NodeTypeLLMResponses
	WorkflowNodeTypeJoinAll               = workflow.NodeTypeJoinAll
	WorkflowNodeTypeTransformJSON         = workflow.NodeTypeTransformJSON
	ToolExecutionModeServer               = workflow.ToolExecutionModeServer
	ToolExecutionModeClient               = workflow.ToolExecutionModeClient
	LLMResponsesBindingEncodingJSON       = workflow.LLMResponsesBindingEncodingJSON
	LLMResponsesBindingEncodingJSONString = workflow.LLMResponsesBindingEncodingJSONString
	RunEventEnvelopeVersionV0             = workflow.EventEnvelopeVersionV0
	ArtifactKeyNodeOutputV0               = workflow.ArtifactKeyNodeOutputV0
	ArtifactKeyRunOutputsV0               = workflow.ArtifactKeyRunOutputsV0
	RunEventRunCompiled                   = workflow.EventRunCompiled
	RunEventRunStarted                    = workflow.EventRunStarted
	RunEventRunCompleted                  = workflow.EventRunCompleted
	RunEventRunFailed                     = workflow.EventRunFailed
	RunEventRunCanceled                   = workflow.EventRunCanceled
	RunEventNodeLLMCall                   = workflow.EventNodeLLMCall
	RunEventNodeToolCall                  = workflow.EventNodeToolCall
	RunEventNodeToolResult                = workflow.EventNodeToolResult
	RunEventNodeWaiting                   = workflow.EventNodeWaiting
	RunEventNodeStarted                   = workflow.EventNodeStarted
	RunEventNodeSucceeded                 = workflow.EventNodeSucceeded
	RunEventNodeFailed                    = workflow.EventNodeFailed
	RunEventNodeOutputDelta               = workflow.EventNodeOutputDelta
	RunEventNodeOutput                    = workflow.EventNodeOutput
	RunStatusRunning                      = workflow.StatusRunning
	RunStatusWaiting                      = workflow.StatusWaiting
	RunStatusSucceeded                    = workflow.StatusSucceeded
	RunStatusFailed                       = workflow.StatusFailed
	RunStatusCanceled                     = workflow.StatusCanceled
	NodeStatusPending                     = workflow.NodeStatusPending
	NodeStatusRunning                     = workflow.NodeStatusRunning
	NodeStatusWaiting                     = workflow.NodeStatusWaiting
	NodeStatusSucceeded                   = workflow.NodeStatusSucceeded
	NodeStatusFailed                      = workflow.NodeStatusFailed
	NodeStatusCanceled                    = workflow.NodeStatusCanceled
	StreamEventKindMessageStart           = workflow.StreamEventKindMessageStart
	StreamEventKindMessageDelta           = workflow.StreamEventKindMessageDelta
	StreamEventKindMessageStop            = workflow.StreamEventKindMessageStop
	StreamEventKindToolUseStart           = workflow.StreamEventKindToolUseStart
	StreamEventKindToolUseDelta           = workflow.StreamEventKindToolUseDelta
	StreamEventKindToolUseStop            = workflow.StreamEventKindToolUseStop
)

// Re-export workflow functions.
var (
	NewNodeID      = workflow.NewNodeID
	NewOutputName  = workflow.NewOutputName
	NewJSONPointer = workflow.NewJSONPointer
	NewRunID       = workflow.NewRunID
	ParseRunID     = workflow.ParseRunID
	ParsePlanHash  = workflow.ParsePlanHash
)

// ResponseID is a stable identifier for a /responses completion.
// This type belongs in the sdk package (not workflow) as it's for /responses endpoint.
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
