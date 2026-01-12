package workflowintent

import (
	"encoding/json"
	"strings"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

// Kind identifies the workflow intent spec version.
type Kind string

const (
	KindWorkflow Kind = "workflow"
)

func (k Kind) Valid() bool {
	return k == KindWorkflow
}

// NodeType defines the supported node types in workflow.
type NodeType string

const (
	NodeTypeLLM           NodeType = "llm"
	NodeTypeJoinAll       NodeType = "join.all"
	NodeTypeJoinCollect   NodeType = "join.collect"
	NodeTypeJoinAny       NodeType = "join.any"
	NodeTypeTransformJSON NodeType = "transform.json"
	NodeTypeMapFanout     NodeType = "map.fanout"
)

func (t NodeType) Valid() bool {
	switch t {
	case NodeTypeLLM, NodeTypeJoinAll, NodeTypeJoinCollect, NodeTypeJoinAny, NodeTypeTransformJSON, NodeTypeMapFanout:
		return true
	default:
		return false
	}
}

// Spec defines the workflow schema.
type Spec struct {
	Kind           Kind        `json:"kind"`
	Name           string      `json:"name,omitempty"`
	Model          string      `json:"model,omitempty"`
	MaxParallelism *int64      `json:"max_parallelism,omitempty"`
	Inputs         []InputDecl `json:"inputs,omitempty"`
	Nodes          []Node      `json:"nodes"`
	Outputs        []OutputRef `json:"outputs"`
}

// InputDecl declares a workflow input for validation and documentation.
type InputDecl struct {
	Name        string          `json:"name"`
	Type        string          `json:"type,omitempty"` // "string", "number", "object", "array", "boolean", "null", "json"
	Required    bool            `json:"required,omitempty"`
	Description string          `json:"description,omitempty"`
	Default     json.RawMessage `json:"default,omitempty"`
}

// Node defines a workflow node.
type Node struct {
	ID        string   `json:"id"`
	Type      NodeType `json:"type"`
	DependsOn []string `json:"depends_on,omitempty"`

	// LLM node fields.
	Model           string            `json:"model,omitempty"`
	System          string            `json:"system,omitempty"`
	User            string            `json:"user,omitempty"`
	Input           []llm.InputItem   `json:"input,omitempty"`
	Stream          *bool             `json:"stream,omitempty"`
	Tools           []ToolRef         `json:"tools,omitempty"`
	ToolExecution   *ToolExecution    `json:"tool_execution,omitempty"`
	Retry           *RetryConfig      `json:"retry,omitempty"`
	MaxOutputTokens *int64            `json:"max_output_tokens,omitempty"`
	OutputFormat    *llm.OutputFormat `json:"output_format,omitempty"`
	Stop            []string          `json:"stop,omitempty"`

	// join.collect fields.
	Limit     *int64     `json:"limit,omitempty"`
	TimeoutMS *int64     `json:"timeout_ms,omitempty"`
	Predicate *Condition `json:"predicate,omitempty"`

	// map.fanout fields.
	ItemsFrom      string `json:"items_from,omitempty"`
	ItemsFromInput string `json:"items_from_input,omitempty"`
	ItemsPointer   string `json:"items_pointer,omitempty"`
	ItemsPath      string `json:"items_path,omitempty"`
	SubNode        *Node  `json:"subnode,omitempty"`
	MaxParallelism *int64 `json:"max_parallelism,omitempty"`

	// transform.json fields.
	Object map[string]TransformValue `json:"object,omitempty"`
	Merge  []TransformValue          `json:"merge,omitempty"`
}

// OutputRef defines a workflow output.
type OutputRef struct {
	Name    string `json:"name"`
	From    string `json:"from"`
	Pointer string `json:"pointer,omitempty"`
}

// ConditionSource defines valid condition sources.
type ConditionSource string

const (
	ConditionSourceNodeOutput ConditionSource = "node_output"
	ConditionSourceNodeStatus ConditionSource = "node_status"
)

func (s ConditionSource) Valid() bool {
	switch s {
	case ConditionSourceNodeOutput, ConditionSourceNodeStatus:
		return true
	default:
		return false
	}
}

// ConditionOp defines valid condition operators.
type ConditionOp string

const (
	ConditionOpEquals  ConditionOp = "equals"
	ConditionOpMatches ConditionOp = "matches"
	ConditionOpExists  ConditionOp = "exists"
)

func (o ConditionOp) Valid() bool {
	switch o {
	case ConditionOpEquals, ConditionOpMatches, ConditionOpExists:
		return true
	default:
		return false
	}
}

// Condition mirrors workflow.ConditionV1 for intent specs.
type Condition struct {
	Source ConditionSource `json:"source"`
	Op     ConditionOp     `json:"op"`
	Path   string          `json:"path,omitempty"`
	Value  json.RawMessage `json:"value,omitempty"`
}

// TransformValue defines a transform.json value mapping.
type TransformValue struct {
	From    string `json:"from"`
	Pointer string `json:"pointer,omitempty"`
}

// ToolExecutionMode defines valid tool execution modes.
type ToolExecutionMode string

const (
	ToolExecutionModeServer  ToolExecutionMode = "server"
	ToolExecutionModeClient  ToolExecutionMode = "client"
	ToolExecutionModeAgentic ToolExecutionMode = "agentic"
)

func (m ToolExecutionMode) Valid() bool {
	switch m {
	case ToolExecutionModeServer, ToolExecutionModeClient, ToolExecutionModeAgentic:
		return true
	default:
		return false
	}
}

// ToolExecution configures tool execution behavior for intent LLM nodes.
type ToolExecution struct {
	Mode ToolExecutionMode `json:"mode"`
}

// RetryConfig configures retry behavior for LLM nodes.
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (including the first).
	// Default is 1 (no retries). Set to 2+ to enable retries.
	MaxAttempts int `json:"max_attempts,omitempty"`

	// RetryableErrors specifies which error types should trigger a retry.
	// Supported values: "invalid_json", "truncated", "rate_limit", "timeout".
	// If empty, defaults to ["invalid_json", "truncated"].
	RetryableErrors []string `json:"retryable_errors,omitempty"`

	// BackoffMS is the initial backoff delay in milliseconds.
	// Default is 1000. Uses exponential backoff with jitter.
	BackoffMS int `json:"backoff_ms,omitempty"`
}

// ToolRef accepts either a tool name string or a full tool definition.
type ToolRef struct {
	Tool llm.Tool
	raw  json.RawMessage
}

func (t *ToolRef) UnmarshalJSON(data []byte) error {
	t.raw = append(t.raw[:0], data...)
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil
		}
		t.Tool = llm.Tool{
			Type: llm.ToolTypeFunction,
			Function: &llm.FunctionTool{
				Name: llm.ToolName(name),
			},
		}
		return nil
	}
	var tool llm.Tool
	if err := json.Unmarshal(data, &tool); err != nil {
		return err
	}
	t.Tool = tool
	return nil
}

func (t ToolRef) MarshalJSON() ([]byte, error) {
	if len(t.raw) > 0 {
		return t.raw, nil
	}
	return json.Marshal(t.Tool)
}

func (t ToolRef) Name() string {
	if t.Tool.Function == nil {
		return ""
	}
	return strings.TrimSpace(string(t.Tool.Function.Name))
}
