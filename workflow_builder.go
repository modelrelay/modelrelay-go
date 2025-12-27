package sdk

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

type llmResponsesNodeInputV0 struct {
	Request       responseRequestPayload    `json:"request"`
	Stream        *bool                     `json:"stream,omitempty"`
	ToolExecution *ToolExecutionV0          `json:"tool_execution,omitempty"`
	ToolLimits    *LLMResponsesToolLimitsV0 `json:"tool_limits,omitempty"`
	Bindings      []LLMResponsesBindingV0   `json:"bindings,omitempty"`
}

type LLMResponsesNodeOptionsV0 struct {
	ToolExecution *ToolExecutionV0
	ToolLimits    *LLMResponsesToolLimitsV0
	Bindings      []LLMResponsesBindingV0
}

type TransformJSONNodeInputV0 struct {
	Object map[string]TransformJSONFieldRefV0 `json:"object,omitempty"`
	Merge  []TransformJSONRefV0               `json:"merge,omitempty"`
}

type TransformJSONFieldRefV0 struct {
	From    NodeID      `json:"from"`
	Pointer JSONPointer `json:"pointer,omitempty"`
}

type TransformJSONRefV0 struct {
	From    NodeID      `json:"from"`
	Pointer JSONPointer `json:"pointer,omitempty"`
}

type WorkflowBuilderV0 struct {
	name      string
	execution *WorkflowExecutionV0
	nodes     []WorkflowNodeV0
	edges     []WorkflowEdgeV0
	outputs   []WorkflowOutputRefV0
}

func WorkflowV0() WorkflowBuilderV0 {
	return WorkflowBuilderV0{}
}

func (b WorkflowBuilderV0) Name(name string) WorkflowBuilderV0 {
	b.name = strings.TrimSpace(name)
	return b
}

func (b WorkflowBuilderV0) Execution(exec WorkflowExecutionV0) WorkflowBuilderV0 {
	b.execution = &exec
	return b
}

func (b WorkflowBuilderV0) Node(node WorkflowNodeV0) WorkflowBuilderV0 {
	next := make([]WorkflowNodeV0, len(b.nodes)+1)
	copy(next, b.nodes)
	next[len(b.nodes)] = node
	b.nodes = next
	return b
}

func (b WorkflowBuilderV0) LLMResponsesNode(id NodeID, req ResponseRequest, stream *bool) (WorkflowBuilderV0, error) {
	return b.LLMResponsesNodeWithBindings(id, req, stream, nil)
}

func (b WorkflowBuilderV0) LLMResponsesNodeWithBindings(id NodeID, req ResponseRequest, stream *bool, bindings []LLMResponsesBindingV0) (WorkflowBuilderV0, error) {
	return b.LLMResponsesNodeWithOptions(id, req, stream, LLMResponsesNodeOptionsV0{Bindings: bindings})
}

func (b WorkflowBuilderV0) LLMResponsesNodeWithOptions(id NodeID, req ResponseRequest, stream *bool, opts LLMResponsesNodeOptionsV0) (WorkflowBuilderV0, error) {
	// Validate binding targets before building the node
	if err := validateBindingTargets(id, req, opts.Bindings); err != nil {
		return WorkflowBuilderV0{}, err
	}

	payload := llmResponsesNodeInputV0{
		Request:       newResponseRequestPayload(req),
		Stream:        stream,
		ToolExecution: opts.ToolExecution,
		ToolLimits:    opts.ToolLimits,
		Bindings:      append([]LLMResponsesBindingV0{}, opts.Bindings...),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return WorkflowBuilderV0{}, err
	}
	result := b.Node(WorkflowNodeV0{ID: id, Type: WorkflowNodeTypeLLMResponses, Input: raw})

	// Bindings imply edges: a binding from node A means this node depends on A.
	// Automatically add edges for each binding source to avoid "must be an incoming dependency" errors.
	for _, binding := range opts.Bindings {
		if binding.From != "" {
			result = result.Edge(binding.From, id)
		}
	}

	return result, nil
}

func (b WorkflowBuilderV0) JoinAllNode(id NodeID) WorkflowBuilderV0 {
	return b.Node(WorkflowNodeV0{ID: id, Type: WorkflowNodeTypeJoinAll})
}

func (b WorkflowBuilderV0) TransformJSONNode(id NodeID, input TransformJSONNodeInputV0) (WorkflowBuilderV0, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return WorkflowBuilderV0{}, err
	}
	return b.Node(WorkflowNodeV0{ID: id, Type: WorkflowNodeTypeTransformJSON, Input: raw}), nil
}

func (b WorkflowBuilderV0) Edge(from, to NodeID) WorkflowBuilderV0 {
	// Check if edge already exists (dedup for bindings that imply edges)
	for _, e := range b.edges {
		if e.From == from && e.To == to {
			return b // Edge already exists, no-op
		}
	}
	next := make([]WorkflowEdgeV0, len(b.edges)+1)
	copy(next, b.edges)
	next[len(b.edges)] = WorkflowEdgeV0{From: from, To: to}
	b.edges = next
	return b
}

func (b WorkflowBuilderV0) Output(name OutputName, from NodeID, pointer JSONPointer) WorkflowBuilderV0 {
	next := make([]WorkflowOutputRefV0, len(b.outputs)+1)
	copy(next, b.outputs)
	next[len(b.outputs)] = WorkflowOutputRefV0{Name: name, From: from, Pointer: pointer}
	b.outputs = next
	return b
}

func (b WorkflowBuilderV0) Build() (WorkflowSpecV0, error) {
	spec := WorkflowSpecV0{
		Kind:    WorkflowKindV0,
		Name:    b.name,
		Nodes:   append([]WorkflowNodeV0(nil), b.nodes...),
		Edges:   append([]WorkflowEdgeV0(nil), b.edges...),
		Outputs: append([]WorkflowOutputRefV0(nil), b.outputs...),
	}
	if b.execution != nil {
		spec.Execution = b.execution
	}

	sort.Slice(spec.Edges, func(i, j int) bool {
		if spec.Edges[i].From != spec.Edges[j].From {
			return spec.Edges[i].From < spec.Edges[j].From
		}
		return spec.Edges[i].To < spec.Edges[j].To
	})

	sort.Slice(spec.Outputs, func(i, j int) bool {
		if spec.Outputs[i].Name != spec.Outputs[j].Name {
			return spec.Outputs[i].Name < spec.Outputs[j].Name
		}
		if spec.Outputs[i].From != spec.Outputs[j].From {
			return spec.Outputs[i].From < spec.Outputs[j].From
		}
		return spec.Outputs[i].Pointer < spec.Outputs[j].Pointer
	})

	return spec, nil
}

// inputPointerPattern matches /input/{index}/... paths
var inputPointerPattern = regexp.MustCompile(`^/input/(\d+)(?:/content/(\d+))?`)

// BindingTargetError describes a binding that targets a non-existent path.
type BindingTargetError struct {
	NodeID       NodeID
	BindingIndex int
	Pointer      JSONPointer
	Message      string
}

func (e BindingTargetError) Error() string {
	return fmt.Sprintf("node %q binding %d: %s", e.NodeID, e.BindingIndex, e.Message)
}

// validateBindingTargets checks that binding targets exist in the request.
// Returns nil if all bindings are valid, or an error describing the first invalid binding.
func validateBindingTargets(nodeID NodeID, req ResponseRequest, bindings []LLMResponsesBindingV0) error {
	input := req.Input()
	for i, binding := range bindings {
		if binding.To == "" {
			// ToPlaceholder bindings don't target a specific path
			continue
		}
		if err := validateInputPointer(binding.To, input); err != nil {
			return BindingTargetError{
				NodeID:       nodeID,
				BindingIndex: i,
				Pointer:      binding.To,
				Message:      err.Error(),
			}
		}
	}
	return nil
}

// validateInputPointer checks that a pointer targeting /input/... exists.
func validateInputPointer(pointer JSONPointer, input []llm.InputItem) error {
	p := string(pointer)
	if !strings.HasPrefix(p, "/input/") {
		// Not an input pointer (e.g., /output/...), skip validation
		return nil
	}

	matches := inputPointerPattern.FindStringSubmatch(p)
	if matches == nil {
		// Doesn't match /input/{index}/... pattern, skip validation
		return nil
	}

	// Parse message index
	msgIndex, err := strconv.Atoi(matches[1])
	if err != nil {
		return fmt.Errorf("invalid message index in %s", pointer)
	}

	if msgIndex >= len(input) {
		return fmt.Errorf("targets %s but request only has %d messages (indices 0-%d); add placeholder messages or adjust binding target",
			pointer, len(input), len(input)-1)
	}

	// Optionally validate content block index
	if matches[2] != "" {
		contentIndex, err := strconv.Atoi(matches[2])
		if err != nil {
			return fmt.Errorf("invalid content index in %s", pointer)
		}
		msg := input[msgIndex]
		if contentIndex >= len(msg.Content) {
			return fmt.Errorf("targets %s but message %d only has %d content blocks (indices 0-%d)",
				pointer, msgIndex, len(msg.Content), len(msg.Content)-1)
		}
	}

	return nil
}
