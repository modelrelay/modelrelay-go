package sdk

import (
	"errors"
	"strings"

	"github.com/modelrelay/modelrelay/sdk/go/workflow"
	"github.com/modelrelay/modelrelay/sdk/go/workflowintent"
)

// Re-export workflow types for internal SDK use.
// External users should prefer importing the workflowintent package directly:
//
//	import "github.com/modelrelay/modelrelay/sdk/go/workflowintent"
//	spec := workflowintent.Spec{...}
type (
	WorkflowKind                    = workflowintent.Kind
	WorkflowNodeType                = workflowintent.NodeType
	NodeID                          = workflow.NodeID
	OutputName                      = workflow.OutputName
	JSONPointer                     = workflow.JSONPointer
	JSONPath                        = workflow.JSONPath
	PlaceholderName                 = workflow.PlaceholderName
	InputName                       = workflow.InputName
	WorkflowIntentNode              = workflowintent.Node
	WorkflowIntentOutputRef         = workflowintent.OutputRef
	WorkflowSpec              = workflowintent.Spec
	WorkflowIntentCondition         = workflowintent.Condition
	WorkflowIntentConditionSource   = workflowintent.ConditionSource
	WorkflowIntentConditionOp       = workflowintent.ConditionOp
	WorkflowIntentTransformValue    = workflowintent.TransformValue
	WorkflowIntentToolExecution     = workflowintent.ToolExecution
	WorkflowIntentToolExecutionMode = workflowintent.ToolExecutionMode
	WorkflowIssue                 = workflow.Issue
	WorkflowValidationError       = workflow.ValidationError
	ConditionSourceV1             = workflow.ConditionSourceV1
	ConditionOpV1                 = workflow.ConditionOpV1
	ConditionV1                   = workflow.ConditionV1
	RunID                         = workflow.RunID
	PlanHash                      = workflow.PlanHash
	RunEventType                  = workflow.EventTypeV0
	RunStatus                     = workflow.StatusV0
	NodeStatus                    = workflow.NodeStatus
	NodeError                     = workflow.NodeError
	NodeResult                    = workflow.NodeResult
	RunEventEnvelope              = workflow.EventV0Envelope
	PayloadInfo                   = workflow.PayloadInfo
	PayloadArtifact               = workflow.PayloadArtifact
	TokenUsage                    = workflow.TokenUsage
	NodeOutputDelta               = workflow.NodeOutputDelta
	NodeLLMCall                   = workflow.NodeLLMCall
	ToolCall                      = workflow.ToolCall
	ToolCallWithArguments         = workflow.ToolCallWithArguments
	NodeToolCall                  = workflow.NodeToolCall
	NodeToolResult                = workflow.NodeToolResult
	PendingToolCall               = workflow.PendingToolCall
	NodeWaiting                   = workflow.NodeWaiting
	RunCostSummary                = workflow.CostSummaryV0
	RunCostLineItem               = workflow.CostLineItemV0
	StreamEventKind               = workflow.StreamEventKind
)

// Re-export workflow constants.
const (
	WorkflowKindIntent                = workflowintent.KindWorkflow
	WorkflowNodeTypeLLM               = workflowintent.NodeTypeLLM
	WorkflowNodeTypeJoinAll           = workflowintent.NodeTypeJoinAll
	WorkflowNodeTypeJoinAny           = workflowintent.NodeTypeJoinAny
	WorkflowNodeTypeJoinCollect       = workflowintent.NodeTypeJoinCollect
	WorkflowNodeTypeTransformJSON     = workflowintent.NodeTypeTransformJSON
	WorkflowNodeTypeMapFanout         = workflowintent.NodeTypeMapFanout
	ConditionSourceNodeOutput         = workflow.ConditionSourceNodeOutput
	ConditionSourceNodeStatus         = workflow.ConditionSourceNodeStatus
	ConditionOpEquals                 = workflow.ConditionOpEquals
	ConditionOpMatches                = workflow.ConditionOpMatches
	ConditionOpExists                 = workflow.ConditionOpExists
	LLMTextOutputPointer              = workflow.LLMTextOutputPointer
	LLMUserMessageTextPointer         = workflow.LLMUserMessageTextPointer
	LLMUserMessageTextPointerIndex1   = workflow.LLMUserMessageTextPointerIndex1
	RunEventEnvelopeVersion           = workflow.EventEnvelopeVersionV0
	ArtifactKeyNodeOutputV0           = workflow.ArtifactKeyNodeOutputV0
	ArtifactKeyRunOutputsV0           = workflow.ArtifactKeyRunOutputsV0
	RunEventRunCompiled               = workflow.EventRunCompiled
	RunEventRunStarted                = workflow.EventRunStarted
	RunEventRunCompleted              = workflow.EventRunCompleted
	RunEventRunFailed                 = workflow.EventRunFailed
	RunEventRunCanceled               = workflow.EventRunCanceled
	RunEventNodeLLMCall               = workflow.EventNodeLLMCall
	RunEventNodeToolCall              = workflow.EventNodeToolCall
	RunEventNodeToolResult            = workflow.EventNodeToolResult
	RunEventNodeWaiting               = workflow.EventNodeWaiting
	RunEventNodeStarted               = workflow.EventNodeStarted
	RunEventNodeSucceeded             = workflow.EventNodeSucceeded
	RunEventNodeFailed                = workflow.EventNodeFailed
	RunEventNodeOutputDelta           = workflow.EventNodeOutputDelta
	RunEventNodeOutput                = workflow.EventNodeOutput
	RunStatusRunning                  = workflow.StatusRunning
	RunStatusWaiting                  = workflow.StatusWaiting
	RunStatusSucceeded                = workflow.StatusSucceeded
	RunStatusFailed                   = workflow.StatusFailed
	RunStatusCanceled                 = workflow.StatusCanceled
	NodeStatusPending                 = workflow.NodeStatusPending
	NodeStatusRunning                 = workflow.NodeStatusRunning
	NodeStatusWaiting                 = workflow.NodeStatusWaiting
	NodeStatusSucceeded               = workflow.NodeStatusSucceeded
	NodeStatusFailed                  = workflow.NodeStatusFailed
	NodeStatusCanceled                = workflow.NodeStatusCanceled
	StreamEventKindMessageStart       = workflow.StreamEventKindMessageStart
	StreamEventKindMessageDelta       = workflow.StreamEventKindMessageDelta
	StreamEventKindMessageStop        = workflow.StreamEventKindMessageStop
	StreamEventKindToolUseStart       = workflow.StreamEventKindToolUseStart
	StreamEventKindToolUseDelta       = workflow.StreamEventKindToolUseDelta
	StreamEventKindToolUseStop        = workflow.StreamEventKindToolUseStop
)

// Re-export workflow functions.
var (
	NewNodeID               = workflow.NewNodeID
	NewOutputName           = workflow.NewOutputName
	NewJSONPointer          = workflow.NewJSONPointer
	NewPlaceholderName      = workflow.NewPlaceholderName
	NewRunID                = workflow.NewRunID
	ParseRunID              = workflow.ParseRunID
	ParsePlanHash           = workflow.ParsePlanHash
	MapFanoutItemsFromLLM   = workflow.MapFanoutItemsFromLLM
	MapFanoutItemsFromInput = workflow.MapFanoutItemsFromInput
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
