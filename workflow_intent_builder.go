package sdk

import (
	"fmt"
	"strings"

	"github.com/modelrelay/modelrelay/sdk/go/llm"
	"github.com/modelrelay/modelrelay/sdk/go/workflowintent"
)

type workflowIntentEdge struct {
	From string
	To   string
}

// WorkflowIntentBuilder builds workflow specs.
type WorkflowIntentBuilder struct {
	name    string
	model   string
	nodes   []workflowintent.Node
	edges   []workflowIntentEdge
	outputs []workflowintent.OutputRef
}

// WorkflowIntent starts a workflow builder.
func WorkflowIntent() WorkflowIntentBuilder {
	return WorkflowIntentBuilder{}
}

func (b WorkflowIntentBuilder) Name(name string) WorkflowIntentBuilder {
	b.name = strings.TrimSpace(name)
	return b
}

func (b WorkflowIntentBuilder) Model(model string) WorkflowIntentBuilder {
	b.model = strings.TrimSpace(model)
	return b
}

func (b WorkflowIntentBuilder) Node(node workflowintent.Node) WorkflowIntentBuilder {
	next := make([]workflowintent.Node, len(b.nodes)+1)
	copy(next, b.nodes)
	next[len(b.nodes)] = node
	b.nodes = next
	return b
}

func (b WorkflowIntentBuilder) LLM(id string, configure func(LLMNodeBuilder) LLMNodeBuilder) WorkflowIntentBuilder {
	node := NewLLMNode(id)
	if configure != nil {
		node = configure(node)
	}
	return b.Node(node.Build())
}

func (b WorkflowIntentBuilder) JoinAll(id string) WorkflowIntentBuilder {
	return b.Node(workflowintent.Node{ID: strings.TrimSpace(id), Type: workflowintent.NodeTypeJoinAll})
}

func (b WorkflowIntentBuilder) JoinAny(id string) WorkflowIntentBuilder {
	return b.Node(workflowintent.Node{ID: strings.TrimSpace(id), Type: workflowintent.NodeTypeJoinAny})
}

func (b WorkflowIntentBuilder) JoinCollect(id string, limit *int64, timeoutMS *int64) WorkflowIntentBuilder {
	return b.Node(workflowintent.Node{
		ID:        strings.TrimSpace(id),
		Type:      workflowintent.NodeTypeJoinCollect,
		Limit:     limit,
		TimeoutMS: timeoutMS,
	})
}

func (b WorkflowIntentBuilder) TransformJSON(id string, object map[string]workflowintent.TransformValue) WorkflowIntentBuilder {
	return b.Node(workflowintent.Node{
		ID:     strings.TrimSpace(id),
		Type:   workflowintent.NodeTypeTransformJSON,
		Object: object,
	})
}

// MapFanout creates a map.fanout node that iterates over items from a source.
// When itemsFrom references an LLM node, the compiler automatically extracts from the response envelope.
// Use itemsPath to select the array from the output (e.g., "/tiers" for {"tiers": [...]}).
func (b WorkflowIntentBuilder) MapFanout(id string, itemsFrom string, itemsPath string, subnode workflowintent.Node) WorkflowIntentBuilder {
	return b.Node(workflowintent.Node{
		ID:        strings.TrimSpace(id),
		Type:      workflowintent.NodeTypeMapFanout,
		ItemsFrom: strings.TrimSpace(itemsFrom),
		ItemsPath: strings.TrimSpace(itemsPath),
		SubNode:   &subnode,
	})
}

// MapFanoutFromInput creates a map.fanout node that iterates over items from a workflow input.
// Use itemsPath to select the array from the input (e.g., "/items" if input is {"items": [...]}).
func (b WorkflowIntentBuilder) MapFanoutFromInput(id string, inputName string, itemsPath string, subnode workflowintent.Node) WorkflowIntentBuilder {
	return b.Node(workflowintent.Node{
		ID:             strings.TrimSpace(id),
		Type:           workflowintent.NodeTypeMapFanout,
		ItemsFromInput: strings.TrimSpace(inputName),
		ItemsPath:      strings.TrimSpace(itemsPath),
		SubNode:        &subnode,
	})
}

func (b WorkflowIntentBuilder) Edge(from string, to string) WorkflowIntentBuilder {
	b.edges = append(b.edges, workflowIntentEdge{
		From: strings.TrimSpace(from),
		To:   strings.TrimSpace(to),
	})
	return b
}

func (b WorkflowIntentBuilder) Output(name string, from string) WorkflowIntentBuilder {
	return b.OutputWithPointer(name, from, "")
}

func (b WorkflowIntentBuilder) OutputWithPointer(name string, from string, pointer string) WorkflowIntentBuilder {
	b.outputs = append(b.outputs, workflowintent.OutputRef{
		Name:    strings.TrimSpace(name),
		From:    strings.TrimSpace(from),
		Pointer: strings.TrimSpace(pointer),
	})
	return b
}

func (b WorkflowIntentBuilder) Build() (workflowintent.Spec, error) {
	spec := workflowintent.Spec{
		Kind:    workflowintent.KindWorkflow,
		Name:    strings.TrimSpace(b.name),
		Model:   strings.TrimSpace(b.model),
		Nodes:   append([]workflowintent.Node{}, b.nodes...),
		Outputs: append([]workflowintent.OutputRef{}, b.outputs...),
	}

	index := map[string]int{}
	for i := range spec.Nodes {
		id := strings.TrimSpace(spec.Nodes[i].ID)
		if id == "" {
			return workflowintent.Spec{}, fmt.Errorf("node id is required")
		}
		if _, exists := index[id]; exists {
			return workflowintent.Spec{}, fmt.Errorf("duplicate node id %q", id)
		}
		index[id] = i
	}

	for _, edge := range b.edges {
		if edge.To == "" || edge.From == "" {
			return workflowintent.Spec{}, fmt.Errorf("edge endpoints must be non-empty")
		}
		idx, ok := index[edge.To]
		if !ok {
			return workflowintent.Spec{}, fmt.Errorf("edge to unknown node %q", edge.To)
		}
		spec.Nodes[idx].DependsOn = appendUnique(spec.Nodes[idx].DependsOn, edge.From)
	}

	return spec, nil
}

func appendUnique(values []string, value string) []string {
	for _, v := range values {
		if v == value {
			return values
		}
	}
	return append(values, value)
}

// LLMNodeBuilder configures a workflow.lite LLM node.
type LLMNodeBuilder struct {
	node workflowintent.Node
}

// NewLLMNode starts an LLM node builder with the provided id.
func NewLLMNode(id string) LLMNodeBuilder {
	return LLMNodeBuilder{
		node: workflowintent.Node{
			ID:   strings.TrimSpace(id),
			Type: workflowintent.NodeTypeLLM,
		},
	}
}

func (b LLMNodeBuilder) Model(model string) LLMNodeBuilder {
	b.node.Model = strings.TrimSpace(model)
	return b
}

func (b LLMNodeBuilder) System(text string) LLMNodeBuilder {
	b.node.System = text
	return b
}

func (b LLMNodeBuilder) User(text string) LLMNodeBuilder {
	b.node.User = text
	return b
}

func (b LLMNodeBuilder) Input(items []llm.InputItem) LLMNodeBuilder {
	b.node.Input = append([]llm.InputItem{}, items...)
	return b
}

func (b LLMNodeBuilder) Stream(enabled bool) LLMNodeBuilder {
	b.node.Stream = &enabled
	return b
}

func (b LLMNodeBuilder) ToolExecution(mode workflowintent.ToolExecutionMode) LLMNodeBuilder {
	b.node.ToolExecution = &workflowintent.ToolExecution{Mode: mode}
	return b
}

func (b LLMNodeBuilder) Retry(maxAttempts int, retryableErrors []string, backoffMS int) LLMNodeBuilder {
	b.node.Retry = &workflowintent.RetryConfig{
		MaxAttempts:     maxAttempts,
		RetryableErrors: retryableErrors,
		BackoffMS:       backoffMS,
	}
	return b
}

func (b LLMNodeBuilder) MaxOutputTokens(tokens int64) LLMNodeBuilder {
	b.node.MaxOutputTokens = &tokens
	return b
}

func (b LLMNodeBuilder) OutputFormat(format llm.OutputFormat) LLMNodeBuilder {
	b.node.OutputFormat = &format
	return b
}

func (b LLMNodeBuilder) Stop(sequences ...string) LLMNodeBuilder {
	b.node.Stop = sequences
	return b
}

func (b LLMNodeBuilder) Tools(names ...string) LLMNodeBuilder {
	if len(names) == 0 {
		return b
	}
	refs := make([]workflowintent.ToolRef, 0, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			panic("empty tool name")
		}
		refs = append(refs, workflowintent.ToolRef{
			Tool: llm.Tool{
				Type: llm.ToolTypeFunction,
				Function: &llm.FunctionTool{
					Name: llm.ToolName(trimmed),
				},
			},
		})
	}
	b.node.Tools = append(b.node.Tools, refs...)
	return b
}

func (b LLMNodeBuilder) ToolDefs(tools ...llm.Tool) LLMNodeBuilder {
	if len(tools) == 0 {
		return b
	}
	refs := make([]workflowintent.ToolRef, 0, len(tools))
	for i := range tools {
		refs = append(refs, workflowintent.ToolRef{Tool: tools[i]})
	}
	b.node.Tools = append(b.node.Tools, refs...)
	return b
}

func (b LLMNodeBuilder) Build() workflowintent.Node {
	return b.node
}

// Workflow is an alias for WorkflowIntent with a cleaner name.
func Workflow() WorkflowIntentBuilder {
	return WorkflowIntentBuilder{}
}

// LLM creates a standalone LLM node for use with Chain and Parallel.
func LLM(id string, configure func(LLMNodeBuilder) LLMNodeBuilder) workflowintent.Node {
	node := NewLLMNode(id)
	if configure != nil {
		node = configure(node)
	}
	return node.Build()
}

// ChainOptions configures the Chain helper.
type ChainOptions struct {
	Name  string
	Model string
}

// Chain creates a sequential workflow where each step depends on the previous one.
// Edges are automatically wired based on order.
//
// Example:
//
//	spec, _ := sdk.Chain([]workflowintent.Node{
//	    sdk.LLM("summarize", func(n sdk.LLMNodeBuilder) sdk.LLMNodeBuilder {
//	        return n.System("Summarize.").User("{{task}}")
//	    }),
//	    sdk.LLM("translate", func(n sdk.LLMNodeBuilder) sdk.LLMNodeBuilder {
//	        return n.System("Translate to French.").User("{{summarize}}")
//	    }),
//	}, sdk.ChainOptions{Name: "summarize-translate"}).
//	    Output("result", "translate").
//	    Build()
func Chain(steps []workflowintent.Node, opts ChainOptions) WorkflowIntentBuilder {
	b := WorkflowIntentBuilder{}

	if opts.Name != "" {
		b = b.Name(opts.Name)
	}
	if opts.Model != "" {
		b = b.Model(opts.Model)
	}

	// Add all nodes
	for i := range steps {
		b = b.Node(steps[i])
	}

	// Wire edges sequentially: step[0] -> step[1] -> step[2] -> ...
	for i := 1; i < len(steps); i++ {
		b = b.Edge(steps[i-1].ID, steps[i].ID)
	}

	return b
}

// ParallelOptions configures the Parallel helper.
type ParallelOptions struct {
	Name   string
	Model  string
	JoinID string // defaults to "join"
}

// Parallel creates a parallel workflow where all steps run concurrently, then join.
// Edges are automatically wired to a join.all node.
//
// Example:
//
//	spec, _ := sdk.Parallel([]workflowintent.Node{
//	    sdk.LLM("agent_a", func(n sdk.LLMNodeBuilder) sdk.LLMNodeBuilder {
//	        return n.User("Write 3 ideas for {{task}}")
//	    }),
//	    sdk.LLM("agent_b", func(n sdk.LLMNodeBuilder) sdk.LLMNodeBuilder {
//	        return n.User("Write 3 objections for {{task}}")
//	    }),
//	}, sdk.ParallelOptions{Name: "multi-agent"}).
//	    LLM("aggregate", func(n sdk.LLMNodeBuilder) sdk.LLMNodeBuilder {
//	        return n.System("Synthesize.").User("{{join}}")
//	    }).
//	    Edge("join", "aggregate").
//	    Output("result", "aggregate").
//	    Build()
func Parallel(steps []workflowintent.Node, opts ParallelOptions) WorkflowIntentBuilder {
	b := WorkflowIntentBuilder{}
	joinID := opts.JoinID
	if joinID == "" {
		joinID = "join"
	}

	if opts.Name != "" {
		b = b.Name(opts.Name)
	}
	if opts.Model != "" {
		b = b.Model(opts.Model)
	}

	// Add all parallel nodes
	for i := range steps {
		b = b.Node(steps[i])
	}

	// Add join node
	b = b.JoinAll(joinID)

	// Wire all parallel nodes to the join
	for i := range steps {
		b = b.Edge(steps[i].ID, joinID)
	}

	return b
}
