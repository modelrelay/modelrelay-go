package workflow

import (
	"encoding/json"
	"strings"
)

type NodeTypeV1 string

const (
	NodeTypeV1LLMResponses  NodeTypeV1 = "llm.responses"
	NodeTypeV1RouteSwitch   NodeTypeV1 = "route.switch"
	NodeTypeV1JoinAll       NodeTypeV1 = "join.all"
	NodeTypeV1JoinAny       NodeTypeV1 = "join.any"
	NodeTypeV1JoinCollect   NodeTypeV1 = "join.collect"
	NodeTypeV1TransformJSON NodeTypeV1 = "transform.json"
	NodeTypeV1MapFanout     NodeTypeV1 = "map.fanout"
)

func (t NodeTypeV1) Valid() bool {
	switch t {
	case NodeTypeV1LLMResponses,
		NodeTypeV1RouteSwitch,
		NodeTypeV1JoinAll,
		NodeTypeV1JoinAny,
		NodeTypeV1JoinCollect,
		NodeTypeV1TransformJSON,
		NodeTypeV1MapFanout:
		return true
	default:
		return false
	}
}

type JSONPath string

func (p JSONPath) String() string { return string(p) }
func (p JSONPath) Valid() bool {
	if p == "" {
		return true
	}
	return strings.HasPrefix(string(p), "$")
}

type ConditionSourceV1 string

const (
	ConditionSourceNodeOutput ConditionSourceV1 = "node_output"
	ConditionSourceNodeStatus ConditionSourceV1 = "node_status"
)

func (s ConditionSourceV1) Valid() bool {
	switch s {
	case ConditionSourceNodeOutput, ConditionSourceNodeStatus:
		return true
	default:
		return false
	}
}

type ConditionOpV1 string

const (
	ConditionOpEquals  ConditionOpV1 = "equals"
	ConditionOpMatches ConditionOpV1 = "matches"
	ConditionOpExists  ConditionOpV1 = "exists"
)

func (o ConditionOpV1) Valid() bool {
	switch o {
	case ConditionOpEquals, ConditionOpMatches, ConditionOpExists:
		return true
	default:
		return false
	}
}

type ConditionV1 struct {
	Source ConditionSourceV1 `json:"source"`
	Op     ConditionOpV1     `json:"op"`
	Path   JSONPath          `json:"path,omitempty"`
	Value  json.RawMessage   `json:"value,omitempty"`
}

type EdgeV1 struct {
	From NodeID       `json:"from"`
	To   NodeID       `json:"to"`
	When *ConditionV1 `json:"when,omitempty"`
}

type SpecV1 struct {
	Kind      Kind          `json:"kind"`
	Name      string        `json:"name,omitempty"`
	Execution *ExecutionV1  `json:"execution,omitempty"`
	Nodes     []NodeV1      `json:"nodes"`
	Edges     []EdgeV1      `json:"edges,omitempty"`
	Outputs   []OutputRefV1 `json:"outputs"`
}

type ExecutionV1 struct {
	MaxParallelism *int64 `json:"max_parallelism,omitempty"`
	NodeTimeoutMS  *int64 `json:"node_timeout_ms,omitempty"`
	RunTimeoutMS   *int64 `json:"run_timeout_ms,omitempty"`
}

type NodeV1 struct {
	ID    NodeID          `json:"id"`
	Type  NodeTypeV1      `json:"type"`
	Input json.RawMessage `json:"input,omitempty"`
}

type OutputRefV1 struct {
	Name    OutputName  `json:"name"`
	From    NodeID      `json:"from"`
	Pointer JSONPointer `json:"pointer,omitempty"`
}

type ToolExecutionModeV1 string

const (
	ToolExecutionModeServerV1 ToolExecutionModeV1 = "server"
	ToolExecutionModeClientV1 ToolExecutionModeV1 = "client"
)

func (m ToolExecutionModeV1) Valid() bool {
	switch m {
	case "", ToolExecutionModeServerV1, ToolExecutionModeClientV1:
		return true
	default:
		return false
	}
}

type ToolExecutionV1 struct {
	Mode ToolExecutionModeV1 `json:"mode"`
}

type LLMResponsesToolLimitsV1 struct {
	MaxLLMCallsPerNode  *int64 `json:"max_llm_calls,omitempty"`
	MaxToolCallsPerStep *int64 `json:"max_tool_calls_per_step,omitempty"`
	WaitTTLMS           *int64 `json:"wait_ttl_ms,omitempty"`
}

// RetryConfigV1 configures retry behavior for LLM nodes.
type RetryConfigV1 struct {
	// MaxAttempts is the maximum number of attempts (including the first).
	// Default is 1 (no retries). Set to 2+ to enable retries.
	MaxAttempts int `json:"max_attempts,omitempty"`

	// RetryableErrors specifies which error types should trigger a retry.
	// Supported values: "invalid_json", "truncated", "rate_limit", "timeout".
	// If empty, defaults to ["invalid_json", "truncated"].
	RetryableErrors []string `json:"retryable_errors,omitempty"`

	// BackoffMS is the initial backoff delay in milliseconds.
	// Default is 1000ms. Doubles after each retry (exponential backoff).
	BackoffMS int `json:"backoff_ms,omitempty"`
}

type LLMResponsesBindingEncodingV1 string

const (
	LLMResponsesBindingEncodingJSONV1       LLMResponsesBindingEncodingV1 = "json"
	LLMResponsesBindingEncodingJSONStringV1 LLMResponsesBindingEncodingV1 = "json_string"
)

func (e LLMResponsesBindingEncodingV1) Valid() bool {
	switch e {
	case "", LLMResponsesBindingEncodingJSONV1, LLMResponsesBindingEncodingJSONStringV1:
		return true
	default:
		return false
	}
}

// LLMResponsesBindingV1 defines how to bind data to an LLM request.
// Exactly one of From or FromInput must be set.
type LLMResponsesBindingV1 struct {
	From          NodeID                        `json:"from,omitempty"`       // Reference to an upstream node's output
	FromInput     InputName                     `json:"from_input,omitempty"` // Reference to a workflow input (mutually exclusive with From)
	Pointer       JSONPointer                   `json:"pointer,omitempty"`
	To            JSONPointer                   `json:"to,omitempty"`
	ToPlaceholder PlaceholderName               `json:"to_placeholder,omitempty"`
	Encoding      LLMResponsesBindingEncodingV1 `json:"encoding,omitempty"`
}

// InputName identifies an input to the workflow. Inputs are provided at runtime.
type InputName string

func (n InputName) Valid() bool {
	return strings.TrimSpace(string(n)) != ""
}

// MapFanoutItemsV1 specifies where to get the items array for a map.fanout node.
// Exactly one of From or FromInput must be set.
type MapFanoutItemsV1 struct {
	From      NodeID      `json:"from,omitempty"`       // Reference to an upstream node's output
	FromInput InputName   `json:"from_input,omitempty"` // Reference to a workflow input (mutually exclusive with From)
	Pointer   JSONPointer `json:"pointer,omitempty"`    // RFC 6901 pointer to extract before parsing; if value is string, parsed as JSON
	Path      JSONPointer `json:"path,omitempty"`       // RFC 6901 pointer to select items array from the document (empty = root)
}

// LLMTextOutputPointer is the JSON pointer to extract text content from an LLM response.
// Use this when the source node uses structured output and you need to parse the JSON text.
const LLMTextOutputPointer JSONPointer = "/output/0/content/0/text"

// MapFanoutItemsFromLLM creates a MapFanoutItemsV1 that extracts items from an LLM node's
// structured output. It automatically handles the response envelope by extracting the text
// from /output/0/content/0/text and parsing it as JSON before applying the path.
//
// Example:
//
//	Items: workflow.MapFanoutItemsFromLLM(nodeID, "/items")
func MapFanoutItemsFromLLM(from NodeID, path JSONPointer) MapFanoutItemsV1 {
	return MapFanoutItemsV1{
		From:    from,
		Pointer: LLMTextOutputPointer,
		Path:    path,
	}
}

// MapFanoutItemsFromInput creates a MapFanoutItemsV1 that extracts items from a workflow input.
// Use this when you want to fan out over an array provided at runtime rather than from a node output.
//
// Example:
//
//	Items: workflow.MapFanoutItemsFromInput("models", "")
//
// This expects the workflow to be called with an input named "models" containing a JSON array.
func MapFanoutItemsFromInput(inputName InputName, path JSONPointer) MapFanoutItemsV1 {
	return MapFanoutItemsV1{
		FromInput: inputName,
		Path:      path,
	}
}

type MapFanoutItemBindingV1 struct {
	Path          JSONPointer                   `json:"path,omitempty"` // RFC 6901 pointer to select from item (empty = whole item)
	To            JSONPointer                   `json:"to,omitempty"`
	ToPlaceholder PlaceholderName               `json:"to_placeholder,omitempty"`
	Encoding      LLMResponsesBindingEncodingV1 `json:"encoding,omitempty"`
}

type MapFanoutSubNodeV1 struct {
	ID    NodeID          `json:"id"`
	Type  NodeTypeV1      `json:"type"`
	Input json.RawMessage `json:"input,omitempty"`
}

type MapFanoutNodeInputV1 struct {
	Items          MapFanoutItemsV1         `json:"items"`
	ItemBindings   []MapFanoutItemBindingV1 `json:"item_bindings,omitempty"`
	SubNode        MapFanoutSubNodeV1       `json:"subnode"`
	MaxParallelism *int64                   `json:"max_parallelism,omitempty"`
}

type JoinAnyNodeInputV1 struct {
	Predicate *ConditionV1 `json:"predicate,omitempty"`
}

type JoinCollectNodeInputV1 struct {
	Predicate *ConditionV1 `json:"predicate,omitempty"`
	Limit     *int64       `json:"limit,omitempty"`
	TimeoutMS *int64       `json:"timeout_ms,omitempty"`
}
