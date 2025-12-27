// Package workflow provides workflow specification types for workflow.v0.
//
// Types in this package use concise names without the "Workflow" prefix.
// For example, use workflow.SpecV0 instead of sdk.WorkflowSpecV0.
//
// Example:
//
//	import "github.com/modelrelay/modelrelay/sdk/go/workflow"
//
//	spec := workflow.SpecV0{
//		Kind:  workflow.KindV0,
//		Nodes: []workflow.NodeV0{{ID: "my_node", Type: workflow.NodeTypeLLMResponses}},
//	}
package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Kind identifies the versioned workflow spec kind.
type Kind string

const (
	KindV0 Kind = "workflow.v0"
)

// NodeType identifies the type of a workflow node.
type NodeType string

const (
	NodeTypeLLMResponses  NodeType = "llm.responses"
	NodeTypeJoinAll       NodeType = "join.all"
	NodeTypeTransformJSON NodeType = "transform.json"
)

// NodeID is a unique identifier for a node within a workflow.
type NodeID string

func NewNodeID(val string) NodeID { return NodeID(strings.TrimSpace(val)) }
func (id NodeID) String() string  { return string(id) }
func (id NodeID) Valid() bool     { return strings.TrimSpace(string(id)) != "" }

// OutputName is the name of a workflow output.
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

// ExecutionV0 configures workflow execution behavior.
type ExecutionV0 struct {
	MaxParallelism *int64 `json:"max_parallelism,omitempty"`
	NodeTimeoutMS  *int64 `json:"node_timeout_ms,omitempty"`
	RunTimeoutMS   *int64 `json:"run_timeout_ms,omitempty"`
}

// NodeV0 represents a node in the workflow DAG.
type NodeV0 struct {
	ID    NodeID          `json:"id"`
	Type  NodeType        `json:"type"`
	Input json.RawMessage `json:"input,omitempty"`
}

// EdgeV0 represents a directed edge between nodes.
type EdgeV0 struct {
	From NodeID `json:"from"`
	To   NodeID `json:"to"`
}

// OutputRefV0 defines an output extracted from a node.
type OutputRefV0 struct {
	Name    OutputName  `json:"name"`
	From    NodeID      `json:"from"`
	Pointer JSONPointer `json:"pointer,omitempty"`
}

// SpecV0 is the request payload shape for workflow.v0.
type SpecV0 struct {
	Kind      Kind          `json:"kind"`
	Name      string        `json:"name,omitempty"`
	Execution *ExecutionV0  `json:"execution,omitempty"`
	Nodes     []NodeV0      `json:"nodes"`
	Edges     []EdgeV0      `json:"edges,omitempty"`
	Outputs   []OutputRefV0 `json:"outputs"`
}

// Issue describes a validation issue with a workflow spec.
type Issue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationError is returned by workflow compilation/validation endpoints.
type ValidationError struct {
	Issues []Issue `json:"issues"`
}

func (e ValidationError) Error() string {
	if len(e.Issues) == 0 {
		return "workflow spec invalid"
	}
	if len(e.Issues) == 1 {
		return "workflow spec invalid: " + e.Issues[0].Message
	}
	return fmt.Sprintf("workflow spec invalid: %d issues", len(e.Issues))
}

// ToolExecutionModeV0 specifies how tool calls are executed.
type ToolExecutionModeV0 string

const (
	ToolExecutionModeServer ToolExecutionModeV0 = "server"
	ToolExecutionModeClient ToolExecutionModeV0 = "client"
)

// ToolExecutionV0 configures tool execution for a node.
type ToolExecutionV0 struct {
	Mode ToolExecutionModeV0 `json:"mode"`
}

// LLMResponsesToolLimitsV0 configures limits for tool execution loops.
type LLMResponsesToolLimitsV0 struct {
	MaxLLMCallsPerNode  *int64 `json:"max_llm_calls,omitempty"`
	MaxToolCallsPerStep *int64 `json:"max_tool_calls_per_step,omitempty"`
	WaitTTLMS           *int64 `json:"wait_ttl_ms,omitempty"`
}

// LLMResponsesBindingEncodingV0 specifies how bound values are encoded.
type LLMResponsesBindingEncodingV0 string

const (
	LLMResponsesBindingEncodingJSON       LLMResponsesBindingEncodingV0 = "json"
	LLMResponsesBindingEncodingJSONString LLMResponsesBindingEncodingV0 = "json_string"
)

// PlaceholderName is a named placeholder marker in a prompt (e.g., "tier_data" for {{tier_data}}).
type PlaceholderName string

func NewPlaceholderName(val string) PlaceholderName { return PlaceholderName(strings.TrimSpace(val)) }
func (n PlaceholderName) String() string            { return string(n) }
func (n PlaceholderName) Valid() bool               { return strings.TrimSpace(string(n)) != "" }

// LLMResponsesBindingV0 binds output from one node to input of another.
//
// Either To or ToPlaceholder must be specified (but not both):
//   - To: a JSON pointer path to the target location (e.g., "/input/2/content/0/text")
//   - ToPlaceholder: a named placeholder to find and replace (e.g., "tier_data" finds {{tier_data}})
type LLMResponsesBindingV0 struct {
	From          NodeID                        `json:"from"`
	Pointer       JSONPointer                   `json:"pointer,omitempty"`
	To            JSONPointer                   `json:"to,omitempty"`
	ToPlaceholder PlaceholderName               `json:"to_placeholder,omitempty"`
	Encoding      LLMResponsesBindingEncodingV0 `json:"encoding,omitempty"`
}
