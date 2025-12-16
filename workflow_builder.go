package sdk

import (
	"encoding/json"
	"sort"
	"strings"
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
	return b.Node(WorkflowNodeV0{ID: id, Type: WorkflowNodeTypeLLMResponses, Input: raw}), nil
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
