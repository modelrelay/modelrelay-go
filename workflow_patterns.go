package sdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// LLMStepConfig configures an LLM step in a workflow pattern.
type LLMStepConfig struct {
	ID     NodeID
	Req    ResponseRequest
	Stream bool
}

// LLMStep creates a step configuration for Chain or Parallel patterns.
func LLMStep(id NodeID, req ResponseRequest) LLMStepConfig {
	return LLMStepConfig{ID: id, Req: req}
}

// WithStream returns a copy with streaming enabled.
func (s LLMStepConfig) WithStream() LLMStepConfig {
	s.Stream = true
	return s
}

// ChainBuilder builds a sequential workflow where each step's output
// feeds into the next step's input.
type ChainBuilder struct {
	name      string
	execution *WorkflowExecutionV0
	steps     []LLMStepConfig
	outputs   []WorkflowOutputRefV0
}

// Chain creates a workflow builder for sequential LLM steps.
// Each step after the first automatically binds its input from
// the previous step's text output.
func Chain(name string, steps ...LLMStepConfig) *ChainBuilder {
	return &ChainBuilder{
		name:  name,
		steps: steps,
	}
}

// Execution sets the workflow execution configuration.
func (c *ChainBuilder) Execution(exec WorkflowExecutionV0) *ChainBuilder {
	c.execution = &exec
	return c
}

// Output adds an output reference from a specific step.
func (c *ChainBuilder) Output(name OutputName, from NodeID) *ChainBuilder {
	c.outputs = append(c.outputs, WorkflowOutputRefV0{
		Name:    name,
		From:    from,
		Pointer: LLMTextOutput,
	})
	return c
}

// OutputLast adds an output reference from the last step.
func (c *ChainBuilder) OutputLast(name OutputName) *ChainBuilder {
	if len(c.steps) == 0 {
		return c
	}
	return c.Output(name, c.steps[len(c.steps)-1].ID)
}

// Build returns the compiled workflow spec.
func (c *ChainBuilder) Build() (WorkflowSpecV0, error) {
	if len(c.steps) == 0 {
		return WorkflowSpecV0{}, errors.New("chain requires at least one step")
	}

	var nodes []WorkflowNodeV0
	var edges []WorkflowEdgeV0

	for i := range c.steps {
		step := &c.steps[i]
		var bindings []LLMResponsesBindingV0

		// Bind from previous step (except for the first step)
		if i > 0 {
			prevID := c.steps[i-1].ID
			bindings = append(bindings, LLMResponsesBindingV0{
				From:     prevID,
				Pointer:  LLMTextOutput,
				To:       LLMUserMessageText,
				Encoding: LLMResponsesBindingEncodingJSONString,
			})
			edges = append(edges, WorkflowEdgeV0{From: prevID, To: step.ID})
		}

		var stream *bool
		if step.Stream {
			stream = BoolPtr(true)
		}

		payload := llmResponsesNodeInputV0{
			Request:  newResponseRequestPayload(step.Req),
			Stream:   stream,
			Bindings: bindings,
		}

		raw, err := json.Marshal(payload)
		if err != nil {
			return WorkflowSpecV0{}, err
		}

		nodes = append(nodes, WorkflowNodeV0{
			ID:    step.ID,
			Type:  WorkflowNodeTypeLLMResponses,
			Input: raw,
		})
	}

	// Sort outputs for deterministic output
	outputs := append([]WorkflowOutputRefV0(nil), c.outputs...)
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
		Name:    c.name,
		Nodes:   nodes,
		Edges:   edges,
		Outputs: outputs,
	}
	if c.execution != nil {
		spec.Execution = c.execution
	}

	return spec, nil
}

// ParallelBuilder builds a workflow where multiple LLM steps
// execute in parallel, with optional aggregation.
type ParallelBuilder struct {
	name      string
	execution *WorkflowExecutionV0
	steps     []LLMStepConfig
	aggregate *aggregateConfig
	outputs   []WorkflowOutputRefV0
}

type aggregateConfig struct {
	id     NodeID
	req    ResponseRequest
	stream bool
}

// Parallel creates a workflow builder for parallel LLM steps.
// All steps execute concurrently with no dependencies between them.
func Parallel(name string, steps ...LLMStepConfig) *ParallelBuilder {
	return &ParallelBuilder{
		name:  name,
		steps: steps,
	}
}

// Execution sets the workflow execution configuration.
func (p *ParallelBuilder) Execution(exec WorkflowExecutionV0) *ParallelBuilder {
	p.execution = &exec
	return p
}

// Aggregate adds a join node that waits for all parallel steps,
// followed by an aggregator LLM node that receives the combined output.
// The join node ID is automatically generated as "<id>_join".
func (p *ParallelBuilder) Aggregate(id NodeID, req ResponseRequest) *ParallelBuilder {
	p.aggregate = &aggregateConfig{id: id, req: req}
	return p
}

// AggregateWithStream is like Aggregate but enables streaming on the aggregator node.
func (p *ParallelBuilder) AggregateWithStream(id NodeID, req ResponseRequest) *ParallelBuilder {
	p.aggregate = &aggregateConfig{id: id, req: req, stream: true}
	return p
}

// Output adds an output reference from a specific step.
func (p *ParallelBuilder) Output(name OutputName, from NodeID) *ParallelBuilder {
	p.outputs = append(p.outputs, WorkflowOutputRefV0{
		Name:    name,
		From:    from,
		Pointer: LLMTextOutput,
	})
	return p
}

// Build returns the compiled workflow spec.
func (p *ParallelBuilder) Build() (WorkflowSpecV0, error) {
	if len(p.steps) == 0 {
		return WorkflowSpecV0{}, errors.New("parallel requires at least one step")
	}

	var nodes []WorkflowNodeV0
	var edges []WorkflowEdgeV0

	// Add all parallel nodes
	for i := range p.steps {
		step := &p.steps[i]
		var stream *bool
		if step.Stream {
			stream = BoolPtr(true)
		}

		payload := llmResponsesNodeInputV0{
			Request: newResponseRequestPayload(step.Req),
			Stream:  stream,
		}

		raw, err := json.Marshal(payload)
		if err != nil {
			return WorkflowSpecV0{}, err
		}

		nodes = append(nodes, WorkflowNodeV0{
			ID:    step.ID,
			Type:  WorkflowNodeTypeLLMResponses,
			Input: raw,
		})
	}

	// Add join and aggregator if configured
	if p.aggregate != nil {
		joinID := NodeID(string(p.aggregate.id) + "_join")

		// Add join.all node
		nodes = append(nodes, WorkflowNodeV0{
			ID:   joinID,
			Type: WorkflowNodeTypeJoinAll,
		})

		// Add edges from all parallel nodes to join
		for i := range p.steps {
			edges = append(edges, WorkflowEdgeV0{From: p.steps[i].ID, To: joinID})
		}

		// Add aggregator node with binding from join
		var stream *bool
		if p.aggregate.stream {
			stream = BoolPtr(true)
		}

		payload := llmResponsesNodeInputV0{
			Request: newResponseRequestPayload(p.aggregate.req),
			Stream:  stream,
			Bindings: []LLMResponsesBindingV0{{
				From:     joinID,
				Pointer:  "", // Empty pointer = full join output
				To:       LLMUserMessageText,
				Encoding: LLMResponsesBindingEncodingJSONString,
			}},
		}

		raw, err := json.Marshal(payload)
		if err != nil {
			return WorkflowSpecV0{}, err
		}

		nodes = append(nodes, WorkflowNodeV0{
			ID:    p.aggregate.id,
			Type:  WorkflowNodeTypeLLMResponses,
			Input: raw,
		})

		// Add edge from join to aggregator
		edges = append(edges, WorkflowEdgeV0{From: joinID, To: p.aggregate.id})
	}

	// Sort edges for deterministic output
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})

	// Sort outputs for deterministic output
	outputs := append([]WorkflowOutputRefV0(nil), p.outputs...)
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
		Name:    p.name,
		Nodes:   nodes,
		Edges:   edges,
		Outputs: outputs,
	}
	if p.execution != nil {
		spec.Execution = p.execution
	}

	return spec, nil
}

// MapItem represents an item to be processed by a mapper in MapReduce.
// Each item becomes a separate mapper node that runs in parallel.
type MapItem struct {
	// ID is the unique identifier for this item.
	// It becomes part of the mapper node ID: "map_<ID>".
	// Must be unique within the MapReduce workflow.
	ID string

	// Req is the complete LLM request for processing this item.
	// The user builds this request with the item content embedded.
	Req ResponseRequest

	// Stream enables streaming for this mapper node.
	Stream bool
}

// WithStream returns a copy with streaming enabled.
func (m MapItem) WithStream() MapItem {
	m.Stream = true
	return m
}

// MapReduceBuilder builds a workflow where items are processed in parallel
// by mapper nodes, then combined by a reducer node.
//
// The pattern creates:
//   - N mapper nodes (one per item), running in parallel
//   - A join.all node to collect all mapper outputs
//   - A reducer LLM node that receives the combined outputs
//
// Note: Items must be known at workflow build time. For dynamic array
// processing at runtime, server-side support for dynamic node instantiation
// would be required.
type MapReduceBuilder struct {
	name      string
	execution *WorkflowExecutionV0
	items     []MapItem
	reducer   *reducerConfig
	outputs   []WorkflowOutputRefV0
}

type reducerConfig struct {
	id     NodeID
	req    ResponseRequest
	stream bool
}

// MapReduce creates a workflow builder for parallel map-reduce processing.
// Each item is processed by a separate mapper node, and results are combined
// by a reducer node.
//
// Example:
//
//	// Build items with their individual prompts
//	items := make([]sdk.MapItem, len(documents))
//	for i, doc := range documents {
//	    req, _, _ := sdk.NewResponseBuilder().
//	        Model(model).
//	        User(fmt.Sprintf("Summarize: %s", doc)).
//	        Build()
//	    items[i] = sdk.MapItem{ID: fmt.Sprintf("doc_%d", i), Req: req}
//	}
//
//	// Build the MapReduce workflow
//	spec, _ := sdk.MapReduce("summarize-docs", items...).
//	    Reduce("combine", combineReq).
//	    Output("result", "combine").
//	    Build()
func MapReduce(name string, items ...MapItem) *MapReduceBuilder {
	return &MapReduceBuilder{
		name:  name,
		items: items,
	}
}

// Execution sets the workflow execution configuration.
func (m *MapReduceBuilder) Execution(exec WorkflowExecutionV0) *MapReduceBuilder {
	m.execution = &exec
	return m
}

// Reduce adds a reducer node that receives all mapper outputs.
// The reducer receives a JSON object mapping each mapper ID to its text output.
// The join node ID is automatically generated as "<id>_join".
func (m *MapReduceBuilder) Reduce(id NodeID, req ResponseRequest) *MapReduceBuilder {
	m.reducer = &reducerConfig{id: id, req: req}
	return m
}

// ReduceWithStream is like Reduce but enables streaming on the reducer node.
func (m *MapReduceBuilder) ReduceWithStream(id NodeID, req ResponseRequest) *MapReduceBuilder {
	m.reducer = &reducerConfig{id: id, req: req, stream: true}
	return m
}

// Output adds an output reference from a specific node.
// Typically used to output from the reducer node.
func (m *MapReduceBuilder) Output(name OutputName, from NodeID) *MapReduceBuilder {
	m.outputs = append(m.outputs, WorkflowOutputRefV0{
		Name:    name,
		From:    from,
		Pointer: LLMTextOutput,
	})
	return m
}

// Build returns the compiled workflow spec.
func (m *MapReduceBuilder) Build() (WorkflowSpecV0, error) {
	if len(m.items) == 0 {
		return WorkflowSpecV0{}, errors.New("map-reduce requires at least one item")
	}

	if m.reducer == nil {
		return WorkflowSpecV0{}, errors.New("map-reduce requires a reducer (call Reduce)")
	}

	// Check for duplicate item IDs
	seenIDs := make(map[string]struct{}, len(m.items))
	for i := range m.items {
		if _, ok := seenIDs[m.items[i].ID]; ok {
			return WorkflowSpecV0{}, fmt.Errorf("duplicate item ID: %q", m.items[i].ID)
		}
		if m.items[i].ID == "" {
			return WorkflowSpecV0{}, errors.New("item ID cannot be empty")
		}
		seenIDs[m.items[i].ID] = struct{}{}
	}

	var nodes []WorkflowNodeV0
	var edges []WorkflowEdgeV0

	joinID := NodeID(string(m.reducer.id) + "_join")

	// Add mapper nodes
	for i := range m.items {
		item := &m.items[i]
		mapperID := NodeID(fmt.Sprintf("map_%s", item.ID))

		var stream *bool
		if item.Stream {
			stream = BoolPtr(true)
		}

		payload := llmResponsesNodeInputV0{
			Request: newResponseRequestPayload(item.Req),
			Stream:  stream,
		}

		raw, err := json.Marshal(payload)
		if err != nil {
			return WorkflowSpecV0{}, err
		}

		nodes = append(nodes, WorkflowNodeV0{
			ID:    mapperID,
			Type:  WorkflowNodeTypeLLMResponses,
			Input: raw,
		})

		// Edge from mapper to join
		edges = append(edges, WorkflowEdgeV0{From: mapperID, To: joinID})
	}

	// Add join.all node
	nodes = append(nodes, WorkflowNodeV0{
		ID:   joinID,
		Type: WorkflowNodeTypeJoinAll,
	})

	// Add reducer node with binding from join
	var stream *bool
	if m.reducer.stream {
		stream = BoolPtr(true)
	}

	payload := llmResponsesNodeInputV0{
		Request: newResponseRequestPayload(m.reducer.req),
		Stream:  stream,
		Bindings: []LLMResponsesBindingV0{{
			From:     joinID,
			Pointer:  "", // Empty pointer = full join output
			To:       LLMUserMessageText,
			Encoding: LLMResponsesBindingEncodingJSONString,
		}},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return WorkflowSpecV0{}, err
	}

	nodes = append(nodes, WorkflowNodeV0{
		ID:    m.reducer.id,
		Type:  WorkflowNodeTypeLLMResponses,
		Input: raw,
	})

	// Edge from join to reducer
	edges = append(edges, WorkflowEdgeV0{From: joinID, To: m.reducer.id})

	// Sort edges for deterministic output
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})

	// Sort outputs for deterministic output
	outputs := append([]WorkflowOutputRefV0(nil), m.outputs...)
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
		Name:    m.name,
		Nodes:   nodes,
		Edges:   edges,
		Outputs: outputs,
	}
	if m.execution != nil {
		spec.Execution = m.execution
	}

	return spec, nil
}
