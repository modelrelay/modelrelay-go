// Package sdk provides a fluent workflow builder for ergonomic workflow construction.
//
// Example:
//
//	spec, err := sdk.NewWorkflow("tier_generation").
//		AddLLMNode("tier_generator", tierReq).Stream(true).
//		AddLLMNode("business_summary", summaryReq).
//			BindFrom("tier_generator", "/output/0/content/0/text").
//		Output("tiers", "tier_generator").
//		Output("summary", "business_summary").
//		Build()
package sdk

import (
	"encoding/json"
	"sort"
)

// Workflow is a fluent builder for constructing workflow specifications.
// Errors are accumulated and returned at Build() time.
type Workflow struct {
	name      string
	execution *WorkflowExecutionV0
	nodes     []WorkflowNodeV0
	edges     map[edgeKey]struct{} // deduplicated edges
	outputs   []WorkflowOutputRefV0
	errors    []error

	// pendingNode tracks the current node being configured
	pendingNode *pendingLLMNode
}

type edgeKey struct {
	from NodeID
	to   NodeID
}

type pendingLLMNode struct {
	id            NodeID
	req           ResponseRequest
	stream        *bool
	bindings      []LLMResponsesBindingV0
	toolExecution *ToolExecutionV0
	toolLimits    *LLMResponsesToolLimitsV0
}

// NewWorkflow creates a new workflow builder with the given name.
func NewWorkflow(name string) *Workflow {
	return &Workflow{
		name:  name,
		edges: make(map[edgeKey]struct{}),
	}
}

// Execution sets the workflow execution configuration.
func (w *Workflow) Execution(exec WorkflowExecutionV0) *Workflow {
	w.flushPendingNode()
	w.execution = &exec
	return w
}

// AddLLMNode adds an LLM responses node and returns a node builder for configuration.
// The node is finalized when another workflow method is called.
func (w *Workflow) AddLLMNode(id NodeID, req ResponseRequest) *LLMNode {
	w.flushPendingNode()
	w.pendingNode = &pendingLLMNode{
		id:  id,
		req: req,
	}
	return &LLMNode{workflow: w}
}

// AddJoinAllNode adds a join.all node that waits for all incoming edges.
func (w *Workflow) AddJoinAllNode(id NodeID) *Workflow {
	w.flushPendingNode()
	w.nodes = append(w.nodes, WorkflowNodeV0{
		ID:   id,
		Type: WorkflowNodeTypeJoinAll,
	})
	return w
}

// AddTransformJSONNode adds a transform.json node and returns a builder.
func (w *Workflow) AddTransformJSONNode(id NodeID) *TransformJSONNode {
	w.flushPendingNode()
	return &TransformJSONNode{
		workflow: w,
		id:       id,
	}
}

// Output adds an output reference extracting the full node output.
func (w *Workflow) Output(name OutputName, from NodeID) *Workflow {
	return w.OutputAt(name, from, "")
}

// OutputAt adds an output reference with a JSON pointer.
func (w *Workflow) OutputAt(name OutputName, from NodeID, pointer JSONPointer) *Workflow {
	w.flushPendingNode()
	w.outputs = append(w.outputs, WorkflowOutputRefV0{
		Name:    name,
		From:    from,
		Pointer: pointer,
	})
	return w
}

// Edge explicitly adds an edge between nodes.
// Note: edges are automatically inferred from bindings, so this is rarely needed.
func (w *Workflow) Edge(from, to NodeID) *Workflow {
	w.flushPendingNode()
	w.edges[edgeKey{from: from, to: to}] = struct{}{}
	return w
}

// Build finalizes the workflow and returns the specification.
// Returns an error if any errors occurred during construction.
func (w *Workflow) Build() (WorkflowSpecV0, error) {
	w.flushPendingNode()

	if len(w.errors) > 0 {
		return WorkflowSpecV0{}, w.errors[0]
	}

	// Convert edge map to sorted slice
	edges := make([]WorkflowEdgeV0, 0, len(w.edges))
	for key := range w.edges {
		edges = append(edges, WorkflowEdgeV0{From: key.from, To: key.to})
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})

	// Sort outputs for deterministic output
	outputs := append([]WorkflowOutputRefV0(nil), w.outputs...)
	sort.Slice(outputs, func(i, j int) bool {
		if outputs[i].Name != outputs[j].Name {
			return outputs[i].Name < outputs[j].Name
		}
		if outputs[i].From != outputs[j].From {
			return outputs[i].From < outputs[j].From
		}
		return outputs[i].Pointer < outputs[j].Pointer
	})

	spec := WorkflowSpecV0{
		Kind:    WorkflowKindV0,
		Name:    w.name,
		Nodes:   append([]WorkflowNodeV0(nil), w.nodes...),
		Edges:   edges,
		Outputs: outputs,
	}
	if w.execution != nil {
		spec.Execution = w.execution
	}

	return spec, nil
}

// flushPendingNode marshals and adds any pending LLM node.
func (w *Workflow) flushPendingNode() {
	if w.pendingNode == nil {
		return
	}

	pending := w.pendingNode
	w.pendingNode = nil

	payload := llmResponsesNodeInputV0{
		Request:       newResponseRequestPayload(pending.req),
		Stream:        pending.stream,
		ToolExecution: pending.toolExecution,
		ToolLimits:    pending.toolLimits,
		Bindings:      append([]LLMResponsesBindingV0{}, pending.bindings...),
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		w.errors = append(w.errors, err)
		return
	}

	w.nodes = append(w.nodes, WorkflowNodeV0{
		ID:    pending.id,
		Type:  WorkflowNodeTypeLLMResponses,
		Input: raw,
	})

	// Auto-infer edges from bindings
	for _, binding := range pending.bindings {
		w.edges[edgeKey{from: binding.From, to: pending.id}] = struct{}{}
	}
}

// LLMNode is a builder for configuring an LLM responses node.
type LLMNode struct {
	workflow *Workflow
}

// Stream enables or disables streaming for this node.
func (n *LLMNode) Stream(enabled bool) *LLMNode {
	if n.workflow.pendingNode != nil {
		n.workflow.pendingNode.stream = &enabled
	}
	return n
}

// BindFrom adds a binding from another node's output to this node's user message text.
// This is a convenience method that binds to /request/input/1/content/0/text with JSON string encoding.
// The edge from the source node is automatically inferred.
func (n *LLMNode) BindFrom(from NodeID, pointer JSONPointer) *LLMNode {
	return n.BindFromTo(from, pointer, "/request/input/1/content/0/text", LLMResponsesBindingEncodingJSONString)
}

// BindFromTo adds a full binding with explicit source/destination pointers and encoding.
// The edge from the source node is automatically inferred.
func (n *LLMNode) BindFromTo(from NodeID, fromPointer, toPointer JSONPointer, encoding LLMResponsesBindingEncodingV0) *LLMNode {
	if n.workflow.pendingNode != nil {
		n.workflow.pendingNode.bindings = append(n.workflow.pendingNode.bindings, LLMResponsesBindingV0{
			From:     from,
			Pointer:  fromPointer,
			To:       toPointer,
			Encoding: encoding,
		})
	}
	return n
}

// ToolExecution sets the tool execution mode (server or client).
func (n *LLMNode) ToolExecution(mode ToolExecutionModeV0) *LLMNode {
	if n.workflow.pendingNode != nil {
		n.workflow.pendingNode.toolExecution = &ToolExecutionV0{Mode: mode}
	}
	return n
}

// ToolLimits sets the tool execution limits.
func (n *LLMNode) ToolLimits(limits LLMResponsesToolLimitsV0) *LLMNode {
	if n.workflow.pendingNode != nil {
		n.workflow.pendingNode.toolLimits = &limits
	}
	return n
}

// AddLLMNode finishes configuring this node and adds another LLM node.
func (n *LLMNode) AddLLMNode(id NodeID, req ResponseRequest) *LLMNode {
	return n.workflow.AddLLMNode(id, req)
}

// AddJoinAllNode finishes configuring this node and adds a join.all node.
func (n *LLMNode) AddJoinAllNode(id NodeID) *Workflow {
	return n.workflow.AddJoinAllNode(id)
}

// AddTransformJSONNode finishes configuring this node and adds a transform.json node.
func (n *LLMNode) AddTransformJSONNode(id NodeID) *TransformJSONNode {
	return n.workflow.AddTransformJSONNode(id)
}

// Edge finishes configuring this node and adds an explicit edge.
func (n *LLMNode) Edge(from, to NodeID) *Workflow {
	return n.workflow.Edge(from, to)
}

// Output finishes configuring this node and adds an output reference.
func (n *LLMNode) Output(name OutputName, from NodeID) *Workflow {
	return n.workflow.Output(name, from)
}

// OutputAt finishes configuring this node and adds an output with pointer.
func (n *LLMNode) OutputAt(name OutputName, from NodeID, pointer JSONPointer) *Workflow {
	return n.workflow.OutputAt(name, from, pointer)
}

// Execution finishes configuring this node and sets execution config.
func (n *LLMNode) Execution(exec WorkflowExecutionV0) *Workflow {
	return n.workflow.Execution(exec)
}

// Build finishes configuring this node and builds the workflow.
func (n *LLMNode) Build() (WorkflowSpecV0, error) {
	return n.workflow.Build()
}

// TransformJSONNode is a builder for configuring a transform.json node.
type TransformJSONNode struct {
	workflow *Workflow
	id       NodeID
	input    TransformJSONNodeInputV0
}

// Object sets the object transformation with field mappings.
func (n *TransformJSONNode) Object(fields map[string]TransformJSONFieldRefV0) *TransformJSONNode {
	n.input.Object = fields
	return n
}

// Merge sets the merge transformation with source references.
func (n *TransformJSONNode) Merge(items []TransformJSONRefV0) *TransformJSONNode {
	n.input.Merge = items
	return n
}

// Done finalizes this node and returns to the workflow builder.
func (n *TransformJSONNode) Done() *Workflow {
	n.finalize()
	return n.workflow
}

// AddLLMNode finalizes this node and adds an LLM node.
func (n *TransformJSONNode) AddLLMNode(id NodeID, req ResponseRequest) *LLMNode {
	n.finalize()
	return n.workflow.AddLLMNode(id, req)
}

// AddJoinAllNode finalizes this node and adds a join.all node.
func (n *TransformJSONNode) AddJoinAllNode(id NodeID) *Workflow {
	n.finalize()
	return n.workflow.AddJoinAllNode(id)
}

// Output finalizes this node and adds an output reference.
func (n *TransformJSONNode) Output(name OutputName, from NodeID) *Workflow {
	n.finalize()
	return n.workflow.Output(name, from)
}

// OutputAt finalizes this node and adds an output with pointer.
func (n *TransformJSONNode) OutputAt(name OutputName, from NodeID, pointer JSONPointer) *Workflow {
	n.finalize()
	return n.workflow.OutputAt(name, from, pointer)
}

// Edge finalizes this node and adds an explicit edge.
func (n *TransformJSONNode) Edge(from, to NodeID) *Workflow {
	n.finalize()
	return n.workflow.Edge(from, to)
}

// Build finalizes this node and builds the workflow.
func (n *TransformJSONNode) Build() (WorkflowSpecV0, error) {
	n.finalize()
	return n.workflow.Build()
}

func (n *TransformJSONNode) finalize() {
	raw, err := json.Marshal(n.input)
	if err != nil {
		n.workflow.errors = append(n.workflow.errors, err)
		return
	}

	n.workflow.nodes = append(n.workflow.nodes, WorkflowNodeV0{
		ID:    n.id,
		Type:  WorkflowNodeTypeTransformJSON,
		Input: raw,
	})

	// Auto-infer edges from object field references
	for _, ref := range n.input.Object {
		n.workflow.edges[edgeKey{from: ref.From, to: n.id}] = struct{}{}
	}

	// Auto-infer edges from merge references
	for _, ref := range n.input.Merge {
		n.workflow.edges[edgeKey{from: ref.From, to: n.id}] = struct{}{}
	}
}
