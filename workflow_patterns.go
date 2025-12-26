package sdk

import (
	"encoding/json"
	"errors"
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
