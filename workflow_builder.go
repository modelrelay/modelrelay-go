package sdk

import (
	"encoding/json"
	"fmt"
	"reflect"
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

type llmResponsesNodeInputV1 struct {
	Request       responseRequestPayload    `json:"request"`
	Stream        *bool                     `json:"stream,omitempty"`
	ToolExecution *ToolExecutionV1          `json:"tool_execution,omitempty"`
	ToolLimits    *LLMResponsesToolLimitsV1 `json:"tool_limits,omitempty"`
	Bindings      []LLMResponsesBindingV1   `json:"bindings,omitempty"`
	Retry         *RetryConfigV1            `json:"retry,omitempty"`
}

type LLMResponsesNodeOptionsV1 struct {
	ToolExecution *ToolExecutionV1
	ToolLimits    *LLMResponsesToolLimitsV1
	Bindings      []LLMResponsesBindingV1
	Retry         *RetryConfigV1
}

type TransformJSONNodeInputV1 struct {
	Object map[string]TransformJSONFieldRefV1 `json:"object,omitempty"`
	Merge  []TransformJSONRefV1               `json:"merge,omitempty"`
}

type TransformJSONFieldRefV1 struct {
	From    NodeID      `json:"from"`
	Pointer JSONPointer `json:"pointer,omitempty"`
}

type TransformJSONRefV1 struct {
	From    NodeID      `json:"from"`
	Pointer JSONPointer `json:"pointer,omitempty"`
}

type MapFanoutInputError struct {
	NodeID  NodeID
	Message string
}

func (e MapFanoutInputError) Error() string {
	return fmt.Sprintf("node %q: %s", e.NodeID, e.Message)
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

type WorkflowBuilderV1 struct {
	name      string
	execution *WorkflowExecutionV1
	nodes     []WorkflowNodeV1
	edges     []WorkflowEdgeV1
	outputs   []WorkflowOutputRefV1
}

func WorkflowV1() WorkflowBuilderV1 {
	return WorkflowBuilderV1{}
}

func (b WorkflowBuilderV1) Name(name string) WorkflowBuilderV1 {
	b.name = strings.TrimSpace(name)
	return b
}

func (b WorkflowBuilderV1) Execution(exec WorkflowExecutionV1) WorkflowBuilderV1 {
	b.execution = &exec
	return b
}

func (b WorkflowBuilderV1) Node(node WorkflowNodeV1) WorkflowBuilderV1 {
	next := make([]WorkflowNodeV1, len(b.nodes)+1)
	copy(next, b.nodes)
	next[len(b.nodes)] = node
	b.nodes = next
	return b
}

func (b WorkflowBuilderV1) LLMResponsesNode(id NodeID, req ResponseRequest, stream *bool) (WorkflowBuilderV1, error) {
	return b.LLMResponsesNodeWithOptions(id, req, stream, LLMResponsesNodeOptionsV1{})
}

func (b WorkflowBuilderV1) LLMResponsesNodeWithBindings(id NodeID, req ResponseRequest, stream *bool, bindings []LLMResponsesBindingV1) (WorkflowBuilderV1, error) {
	return b.LLMResponsesNodeWithOptions(id, req, stream, LLMResponsesNodeOptionsV1{Bindings: bindings})
}

func (b WorkflowBuilderV1) LLMResponsesNodeWithOptions(id NodeID, req ResponseRequest, stream *bool, opts LLMResponsesNodeOptionsV1) (WorkflowBuilderV1, error) {
	if err := validateBindingTargetsV1(id, req, opts.Bindings); err != nil {
		return WorkflowBuilderV1{}, err
	}

	payload := llmResponsesNodeInputV1{
		Request:       newResponseRequestPayload(req),
		Stream:        stream,
		ToolExecution: opts.ToolExecution,
		ToolLimits:    opts.ToolLimits,
		Bindings:      append([]LLMResponsesBindingV1{}, opts.Bindings...),
		Retry:         opts.Retry,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return WorkflowBuilderV1{}, err
	}
	result := b.Node(WorkflowNodeV1{ID: id, Type: WorkflowNodeTypeV1LLMResponses, Input: raw})

	for _, binding := range opts.Bindings {
		if binding.From != "" {
			result = result.Edge(binding.From, id)
		}
	}

	return result, nil
}

func (b WorkflowBuilderV1) RouteSwitchNode(id NodeID, req ResponseRequest, stream *bool) (WorkflowBuilderV1, error) {
	return b.RouteSwitchNodeWithOptions(id, req, stream, LLMResponsesNodeOptionsV1{})
}

func (b WorkflowBuilderV1) RouteSwitchNodeWithBindings(id NodeID, req ResponseRequest, stream *bool, bindings []LLMResponsesBindingV1) (WorkflowBuilderV1, error) {
	return b.RouteSwitchNodeWithOptions(id, req, stream, LLMResponsesNodeOptionsV1{Bindings: bindings})
}

func (b WorkflowBuilderV1) RouteSwitchNodeWithOptions(id NodeID, req ResponseRequest, stream *bool, opts LLMResponsesNodeOptionsV1) (WorkflowBuilderV1, error) {
	if err := validateBindingTargetsV1(id, req, opts.Bindings); err != nil {
		return WorkflowBuilderV1{}, err
	}

	payload := llmResponsesNodeInputV1{
		Request:       newResponseRequestPayload(req),
		Stream:        stream,
		ToolExecution: opts.ToolExecution,
		ToolLimits:    opts.ToolLimits,
		Bindings:      append([]LLMResponsesBindingV1{}, opts.Bindings...),
		Retry:         opts.Retry,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return WorkflowBuilderV1{}, err
	}
	result := b.Node(WorkflowNodeV1{ID: id, Type: WorkflowNodeTypeV1RouteSwitch, Input: raw})

	for _, binding := range opts.Bindings {
		if binding.From != "" {
			result = result.Edge(binding.From, id)
		}
	}

	return result, nil
}

func (b WorkflowBuilderV1) JoinAllNode(id NodeID) WorkflowBuilderV1 {
	return b.Node(WorkflowNodeV1{ID: id, Type: WorkflowNodeTypeV1JoinAll})
}

func (b WorkflowBuilderV1) JoinAnyNode(id NodeID, input *JoinAnyNodeInputV1) (WorkflowBuilderV1, error) {
	node := WorkflowNodeV1{ID: id, Type: WorkflowNodeTypeV1JoinAny}
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return WorkflowBuilderV1{}, err
		}
		node.Input = raw
	}
	return b.Node(node), nil
}

func (b WorkflowBuilderV1) JoinCollectNode(id NodeID, input JoinCollectNodeInputV1) (WorkflowBuilderV1, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return WorkflowBuilderV1{}, err
	}
	return b.Node(WorkflowNodeV1{ID: id, Type: WorkflowNodeTypeV1JoinCollect, Input: raw}), nil
}

func (b WorkflowBuilderV1) TransformJSONNode(id NodeID, input TransformJSONNodeInputV1) (WorkflowBuilderV1, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return WorkflowBuilderV1{}, err
	}
	return b.Node(WorkflowNodeV1{ID: id, Type: WorkflowNodeTypeV1TransformJSON, Input: raw}), nil
}

func (b WorkflowBuilderV1) MapFanoutNode(id NodeID, input MapFanoutNodeInputV1) (WorkflowBuilderV1, error) {
	if err := validateMapFanoutInputV1(id, input); err != nil {
		return WorkflowBuilderV1{}, err
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return WorkflowBuilderV1{}, err
	}
	return b.Node(WorkflowNodeV1{ID: id, Type: WorkflowNodeTypeV1MapFanout, Input: raw}), nil
}

func (b WorkflowBuilderV1) Edge(from, to NodeID) WorkflowBuilderV1 {
	for _, e := range b.edges {
		if e.From == from && e.To == to && e.When == nil {
			return b
		}
	}
	next := make([]WorkflowEdgeV1, len(b.edges)+1)
	copy(next, b.edges)
	next[len(b.edges)] = WorkflowEdgeV1{From: from, To: to}
	b.edges = next
	return b
}

func (b WorkflowBuilderV1) EdgeWhen(from, to NodeID, when ConditionV1) WorkflowBuilderV1 {
	for _, e := range b.edges {
		if e.From == from && e.To == to && e.When != nil && reflect.DeepEqual(*e.When, when) {
			return b
		}
	}
	next := make([]WorkflowEdgeV1, len(b.edges)+1)
	copy(next, b.edges)
	next[len(b.edges)] = WorkflowEdgeV1{From: from, To: to, When: &when}
	b.edges = next
	return b
}

func (b WorkflowBuilderV1) Output(name OutputName, from NodeID, pointer JSONPointer) WorkflowBuilderV1 {
	next := make([]WorkflowOutputRefV1, len(b.outputs)+1)
	copy(next, b.outputs)
	next[len(b.outputs)] = WorkflowOutputRefV1{Name: name, From: from, Pointer: pointer}
	b.outputs = next
	return b
}

func (b WorkflowBuilderV1) Build() (WorkflowSpecV1, error) {
	spec := WorkflowSpecV1{
		Kind:    WorkflowKindV1,
		Name:    b.name,
		Nodes:   append([]WorkflowNodeV1(nil), b.nodes...),
		Edges:   append([]WorkflowEdgeV1(nil), b.edges...),
		Outputs: append([]WorkflowOutputRefV1(nil), b.outputs...),
	}
	if b.execution != nil {
		spec.Execution = b.execution
	}

	sort.Slice(spec.Edges, func(i, j int) bool {
		if spec.Edges[i].From != spec.Edges[j].From {
			return spec.Edges[i].From < spec.Edges[j].From
		}
		if spec.Edges[i].To != spec.Edges[j].To {
			return spec.Edges[i].To < spec.Edges[j].To
		}
		return edgeConditionKey(spec.Edges[i].When) < edgeConditionKey(spec.Edges[j].When)
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

func MapFanoutSubNodeLLMResponses(id NodeID, req ResponseRequest, stream *bool, opts LLMResponsesNodeOptionsV1) (MapFanoutSubNodeV1, error) {
	payload := llmResponsesNodeInputV1{
		Request:       newResponseRequestPayload(req),
		Stream:        stream,
		ToolExecution: opts.ToolExecution,
		ToolLimits:    opts.ToolLimits,
		Bindings:      append([]LLMResponsesBindingV1{}, opts.Bindings...),
		Retry:         opts.Retry,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return MapFanoutSubNodeV1{}, err
	}
	return MapFanoutSubNodeV1{ID: id, Type: WorkflowNodeTypeV1LLMResponses, Input: raw}, nil
}

func MapFanoutSubNodeRouteSwitch(id NodeID, req ResponseRequest, stream *bool, opts LLMResponsesNodeOptionsV1) (MapFanoutSubNodeV1, error) {
	payload := llmResponsesNodeInputV1{
		Request:       newResponseRequestPayload(req),
		Stream:        stream,
		ToolExecution: opts.ToolExecution,
		ToolLimits:    opts.ToolLimits,
		Bindings:      append([]LLMResponsesBindingV1{}, opts.Bindings...),
		Retry:         opts.Retry,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return MapFanoutSubNodeV1{}, err
	}
	return MapFanoutSubNodeV1{ID: id, Type: WorkflowNodeTypeV1RouteSwitch, Input: raw}, nil
}

func MapFanoutSubNodeTransformJSON(id NodeID, input TransformJSONNodeInputV1) (MapFanoutSubNodeV1, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return MapFanoutSubNodeV1{}, err
	}
	return MapFanoutSubNodeV1{ID: id, Type: WorkflowNodeTypeV1TransformJSON, Input: raw}, nil
}

func edgeConditionKey(cond *ConditionV1) string {
	if cond == nil {
		return ""
	}
	raw, err := json.Marshal(cond)
	if err != nil {
		return ""
	}
	return string(raw)
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

// validateBindingTargetsV1 checks that binding targets exist in the request.
func validateBindingTargetsV1(nodeID NodeID, req ResponseRequest, bindings []LLMResponsesBindingV1) error {
	input := req.Input()
	for i, binding := range bindings {
		if binding.To == "" {
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

func validateMapFanoutInputV1(nodeID NodeID, input MapFanoutNodeInputV1) error {
	switch input.SubNode.Type {
	case WorkflowNodeTypeV1LLMResponses, WorkflowNodeTypeV1RouteSwitch:
		if len(input.SubNode.Input) == 0 {
			return MapFanoutInputError{NodeID: nodeID, Message: "map.fanout subnode input is required"}
		}
		var in llmResponsesNodeInputV1
		if err := json.Unmarshal(input.SubNode.Input, &in); err != nil {
			return MapFanoutInputError{NodeID: nodeID, Message: "map.fanout subnode input must be valid JSON"}
		}
		if len(in.Bindings) > 0 {
			return MapFanoutInputError{NodeID: nodeID, Message: "map.fanout subnode bindings are not allowed"}
		}
	case WorkflowNodeTypeV1TransformJSON:
		if len(input.SubNode.Input) == 0 {
			return MapFanoutInputError{NodeID: nodeID, Message: "map.fanout subnode input is required"}
		}
		if len(input.ItemBindings) > 0 {
			return MapFanoutInputError{NodeID: nodeID, Message: "map.fanout transform.json cannot use item_bindings"}
		}
		var in TransformJSONNodeInputV1
		if err := json.Unmarshal(input.SubNode.Input, &in); err != nil {
			return MapFanoutInputError{NodeID: nodeID, Message: "map.fanout transform.json input must be valid JSON"}
		}
		hasObject := len(in.Object) > 0
		hasMerge := len(in.Merge) > 0
		if hasObject == hasMerge {
			return MapFanoutInputError{NodeID: nodeID, Message: "map.fanout transform.json must provide exactly one of object or merge"}
		}
		if hasObject {
			for key, value := range in.Object {
				if strings.TrimSpace(key) == "" {
					continue
				}
				if value.From.String() != "item" {
					return MapFanoutInputError{NodeID: nodeID, Message: fmt.Sprintf("map.fanout transform.json object.%s.from must be \"item\"", key)}
				}
			}
		}
		if hasMerge {
			for i, value := range in.Merge {
				if value.From.String() != "item" {
					return MapFanoutInputError{NodeID: nodeID, Message: fmt.Sprintf("map.fanout transform.json merge[%d].from must be \"item\"", i)}
				}
			}
		}
	default:
		return MapFanoutInputError{NodeID: nodeID, Message: "unsupported map.fanout subnode type"}
	}

	return nil
}
