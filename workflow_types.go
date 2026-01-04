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
//	spec := workflow.SpecV1{...}
type (
	WorkflowKind                  = workflow.Kind
	WorkflowNodeTypeV1            = workflow.NodeTypeV1
	NodeID                        = workflow.NodeID
	OutputName                    = workflow.OutputName
	JSONPointer                   = workflow.JSONPointer
	JSONPath                      = workflow.JSONPath
	PlaceholderName               = workflow.PlaceholderName
	InputName                     = workflow.InputName
	WorkflowExecutionV1           = workflow.ExecutionV1
	WorkflowNodeV1                = workflow.NodeV1
	WorkflowEdgeV1                = workflow.EdgeV1
	WorkflowOutputRefV1           = workflow.OutputRefV1
	WorkflowSpecV1                = workflow.SpecV1
	WorkflowIssue                 = workflow.Issue
	WorkflowValidationError       = workflow.ValidationError
	ConditionSourceV1             = workflow.ConditionSourceV1
	ConditionOpV1                 = workflow.ConditionOpV1
	ConditionV1                   = workflow.ConditionV1
	ToolExecutionModeV1           = workflow.ToolExecutionModeV1
	ToolExecutionV1               = workflow.ToolExecutionV1
	LLMResponsesToolLimitsV1      = workflow.LLMResponsesToolLimitsV1
	LLMResponsesBindingEncodingV1 = workflow.LLMResponsesBindingEncodingV1
	LLMResponsesBindingV1         = workflow.LLMResponsesBindingV1
	RetryConfigV1                 = workflow.RetryConfigV1
	MapFanoutItemsV1              = workflow.MapFanoutItemsV1
	MapFanoutItemBindingV1        = workflow.MapFanoutItemBindingV1
	MapFanoutSubNodeV1            = workflow.MapFanoutSubNodeV1
	MapFanoutNodeInputV1          = workflow.MapFanoutNodeInputV1
	JoinAnyNodeInputV1            = workflow.JoinAnyNodeInputV1
	JoinCollectNodeInputV1        = workflow.JoinCollectNodeInputV1
	RunID                         = workflow.RunID
	PlanHash                      = workflow.PlanHash
	RunEventTypeV0                = workflow.EventTypeV0
	RunStatusV0                   = workflow.StatusV0
	NodeStatusV0                  = workflow.NodeStatusV0
	NodeErrorV0                   = workflow.NodeErrorV0
	NodeResultV0                  = workflow.NodeResultV0
	RunEventV0Envelope            = workflow.EventV0Envelope
	PayloadInfoV0                 = workflow.PayloadInfoV0
	TokenUsageV0                  = workflow.TokenUsageV0
	NodeOutputDeltaV0             = workflow.NodeOutputDeltaV0
	NodeLLMCallV0                 = workflow.NodeLLMCallV0
	FunctionToolCallV0            = workflow.FunctionToolCallV0
	NodeToolCallV0                = workflow.NodeToolCallV0
	NodeToolResultV0              = workflow.NodeToolResultV0
	PendingToolCallV0             = workflow.PendingToolCallV0
	NodeWaitingV0                 = workflow.NodeWaitingV0
	RunCostSummaryV0              = workflow.CostSummaryV0
	RunCostLineItemV0             = workflow.CostLineItemV0
	StreamEventKind               = workflow.StreamEventKind
)

// Re-export workflow constants.
const (
	WorkflowKindV1                          = workflow.KindV1
	WorkflowNodeTypeV1LLMResponses          = workflow.NodeTypeV1LLMResponses
	WorkflowNodeTypeV1RouteSwitch           = workflow.NodeTypeV1RouteSwitch
	WorkflowNodeTypeV1JoinAll               = workflow.NodeTypeV1JoinAll
	WorkflowNodeTypeV1JoinAny               = workflow.NodeTypeV1JoinAny
	WorkflowNodeTypeV1JoinCollect           = workflow.NodeTypeV1JoinCollect
	WorkflowNodeTypeV1TransformJSON         = workflow.NodeTypeV1TransformJSON
	WorkflowNodeTypeV1MapFanout             = workflow.NodeTypeV1MapFanout
	ConditionSourceNodeOutput               = workflow.ConditionSourceNodeOutput
	ConditionSourceNodeStatus               = workflow.ConditionSourceNodeStatus
	ConditionOpEquals                       = workflow.ConditionOpEquals
	ConditionOpMatches                      = workflow.ConditionOpMatches
	ConditionOpExists                       = workflow.ConditionOpExists
	ToolExecutionModeServerV1               = workflow.ToolExecutionModeServerV1
	ToolExecutionModeClientV1               = workflow.ToolExecutionModeClientV1
	LLMResponsesBindingEncodingJSONV1       = workflow.LLMResponsesBindingEncodingJSONV1
	LLMResponsesBindingEncodingJSONStringV1 = workflow.LLMResponsesBindingEncodingJSONStringV1
	LLMTextOutputPointer                    = workflow.LLMTextOutputPointer
	LLMUserMessageTextPointer               = workflow.LLMUserMessageTextPointer
	LLMUserMessageTextPointerIndex1         = workflow.LLMUserMessageTextPointerIndex1
	RunEventEnvelopeVersionV0               = workflow.EventEnvelopeVersionV0
	ArtifactKeyNodeOutputV0                 = workflow.ArtifactKeyNodeOutputV0
	ArtifactKeyRunOutputsV0                 = workflow.ArtifactKeyRunOutputsV0
	RunEventRunCompiled                     = workflow.EventRunCompiled
	RunEventRunStarted                      = workflow.EventRunStarted
	RunEventRunCompleted                    = workflow.EventRunCompleted
	RunEventRunFailed                       = workflow.EventRunFailed
	RunEventRunCanceled                     = workflow.EventRunCanceled
	RunEventNodeLLMCall                     = workflow.EventNodeLLMCall
	RunEventNodeToolCall                    = workflow.EventNodeToolCall
	RunEventNodeToolResult                  = workflow.EventNodeToolResult
	RunEventNodeWaiting                     = workflow.EventNodeWaiting
	RunEventNodeStarted                     = workflow.EventNodeStarted
	RunEventNodeSucceeded                   = workflow.EventNodeSucceeded
	RunEventNodeFailed                      = workflow.EventNodeFailed
	RunEventNodeOutputDelta                 = workflow.EventNodeOutputDelta
	RunEventNodeOutput                      = workflow.EventNodeOutput
	RunStatusRunning                        = workflow.StatusRunning
	RunStatusWaiting                        = workflow.StatusWaiting
	RunStatusSucceeded                      = workflow.StatusSucceeded
	RunStatusFailed                         = workflow.StatusFailed
	RunStatusCanceled                       = workflow.StatusCanceled
	NodeStatusPending                       = workflow.NodeStatusPending
	NodeStatusRunning                       = workflow.NodeStatusRunning
	NodeStatusWaiting                       = workflow.NodeStatusWaiting
	NodeStatusSucceeded                     = workflow.NodeStatusSucceeded
	NodeStatusFailed                        = workflow.NodeStatusFailed
	NodeStatusCanceled                      = workflow.NodeStatusCanceled
	StreamEventKindMessageStart             = workflow.StreamEventKindMessageStart
	StreamEventKindMessageDelta             = workflow.StreamEventKindMessageDelta
	StreamEventKindMessageStop              = workflow.StreamEventKindMessageStop
	StreamEventKindToolUseStart             = workflow.StreamEventKindToolUseStart
	StreamEventKindToolUseDelta             = workflow.StreamEventKindToolUseDelta
	StreamEventKindToolUseStop              = workflow.StreamEventKindToolUseStop
)

// Re-export workflow functions.
var (
	NewNodeID                = workflow.NewNodeID
	NewOutputName            = workflow.NewOutputName
	NewJSONPointer           = workflow.NewJSONPointer
	NewPlaceholderName       = workflow.NewPlaceholderName
	NewRunID                 = workflow.NewRunID
	ParseRunID               = workflow.ParseRunID
	ParsePlanHash            = workflow.ParsePlanHash
	MapFanoutItemsFromLLM    = workflow.MapFanoutItemsFromLLM
	MapFanoutItemsFromInput  = workflow.MapFanoutItemsFromInput
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
