package sdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/modelrelay/modelrelay/sdk/go/workflow"
)

// =============================================================================
// Router Pattern
// =============================================================================

// RouterRouteV1 defines a single route in the router pattern.
type RouterRouteV1 struct {
	// Value is the value to match in the router output (at RoutePath)
	Value string
	// ID is the handler node ID (auto-generated if empty)
	ID NodeID
	// Req is the LLM request for this route's handler
	Req ResponseRequest
	// Stream enables streaming for this handler
	Stream bool
	// Bindings are optional bindings for the handler node
	Bindings []LLMResponsesBindingV1
}

// RouterAggregatorV1 defines the optional aggregator node configuration.
type RouterAggregatorV1 struct {
	// ID is the aggregator node ID (defaults to "aggregate")
	ID NodeID
	// Req is the LLM request for aggregation
	Req ResponseRequest
	// Stream enables streaming for the aggregator
	Stream bool
	// Placeholder is the name for injecting the routed result (defaults to "route_output")
	Placeholder PlaceholderName
}

// RouterBuilderV1 builds the Router pattern for workflow.v1.
//
// The router pattern classifies input and routes to specialized handlers
// based on the classification result. A join.any node collects the first
// successful handler response.
//
// Topology:
//
//	classifier --[when=billing]--> billing_handler --\
//	           --[when=support]--> support_handler --> join.any --> [aggregator]
//	           --[when=sales]--> sales_handler ----/
//
// Example:
//
//	spec, err := RouterV1("customer_support", classifierReq).
//	    Route("billing", billingReq).
//	    Route("support", supportReq).
//	    Aggregate(aggregatorReq, "route_output").
//	    Build()
type RouterBuilderV1 struct {
	name         string
	classifierID NodeID
	classifierReq ResponseRequest
	classifierStream bool
	routePath    workflow.JSONPath
	routes       []RouterRouteV1
	aggregator   *RouterAggregatorV1
	outputName   OutputName
}

// RouterV1 creates a new RouterBuilderV1 with the given name and classifier request.
func RouterV1(name string, classifierReq ResponseRequest) *RouterBuilderV1 {
	return &RouterBuilderV1{
		name:         name,
		classifierID: "router",
		classifierReq: classifierReq,
		routePath:    "$.route",
		outputName:   "final",
	}
}

// ClassifierID sets a custom ID for the classifier node.
func (r *RouterBuilderV1) ClassifierID(id NodeID) *RouterBuilderV1 {
	r.classifierID = id
	return r
}

// ClassifierStream enables streaming for the classifier node.
func (r *RouterBuilderV1) ClassifierStream() *RouterBuilderV1 {
	r.classifierStream = true
	return r
}

// RoutePath sets the JSONPath to extract the route value from classifier output.
// Defaults to "$.route".
func (r *RouterBuilderV1) RoutePath(path string) *RouterBuilderV1 {
	r.routePath = workflow.JSONPath(path)
	return r
}

// Route adds a route that matches the given value.
func (r *RouterBuilderV1) Route(value string, req ResponseRequest) *RouterBuilderV1 {
	r.routes = append(r.routes, RouterRouteV1{
		Value: value,
		Req:   req,
	})
	return r
}

// RouteWithID adds a route with a specific handler node ID.
func (r *RouterBuilderV1) RouteWithID(id NodeID, value string, req ResponseRequest) *RouterBuilderV1 {
	r.routes = append(r.routes, RouterRouteV1{
		ID:    id,
		Value: value,
		Req:   req,
	})
	return r
}

// RouteWithBindings adds a route with custom bindings.
func (r *RouterBuilderV1) RouteWithBindings(value string, req ResponseRequest, bindings []LLMResponsesBindingV1) *RouterBuilderV1 {
	r.routes = append(r.routes, RouterRouteV1{
		Value:    value,
		Req:      req,
		Bindings: bindings,
	})
	return r
}

// Aggregate adds an aggregator node that combines route results.
func (r *RouterBuilderV1) Aggregate(req ResponseRequest, placeholder PlaceholderName) *RouterBuilderV1 {
	r.aggregator = &RouterAggregatorV1{
		ID:          "aggregate",
		Req:         req,
		Placeholder: placeholder,
	}
	return r
}

// AggregateWithID adds an aggregator node with a specific ID.
func (r *RouterBuilderV1) AggregateWithID(id NodeID, req ResponseRequest, placeholder PlaceholderName) *RouterBuilderV1 {
	r.aggregator = &RouterAggregatorV1{
		ID:          id,
		Req:         req,
		Placeholder: placeholder,
	}
	return r
}

// OutputName sets the workflow output name. Defaults to "final".
func (r *RouterBuilderV1) OutputName(name OutputName) *RouterBuilderV1 {
	r.outputName = name
	return r
}

// Build constructs the workflow specification.
func (r *RouterBuilderV1) Build() (WorkflowSpecV1, error) {
	if len(r.routes) == 0 {
		return WorkflowSpecV1{}, errors.New("router requires at least one route")
	}

	var nodes []WorkflowNodeV1
	var edges []WorkflowEdgeV1

	// Add classifier node
	var classifierStream *bool
	if r.classifierStream {
		classifierStream = BoolPtr(true)
	}
	classifierPayload := llmResponsesNodeInputV1{
		Request: newResponseRequestPayload(r.classifierReq),
		Stream:  classifierStream,
	}
	classifierRaw, err := json.Marshal(classifierPayload)
	if err != nil {
		return WorkflowSpecV1{}, fmt.Errorf("classifier marshal: %w", err)
	}
	nodes = append(nodes, WorkflowNodeV1{
		ID:    r.classifierID,
		Type:  WorkflowNodeTypeV1RouteSwitch,
		Input: classifierRaw,
	})

	// Add join.any node
	joinID := NodeID("__router_join")
	nodes = append(nodes, WorkflowNodeV1{
		ID:   joinID,
		Type: WorkflowNodeTypeV1JoinAny,
	})

	// Add route handlers
	for i := range r.routes {
		route := &r.routes[i]
		handlerID := route.ID
		if handlerID == "" {
			handlerID = NodeID(fmt.Sprintf("handler_%d", i))
		}

		var stream *bool
		if route.Stream {
			stream = BoolPtr(true)
		}

		handlerPayload := llmResponsesNodeInputV1{
			Request:  newResponseRequestPayload(route.Req),
			Stream:   stream,
			Bindings: route.Bindings,
		}
		handlerRaw, err := json.Marshal(handlerPayload)
		if err != nil {
			return WorkflowSpecV1{}, fmt.Errorf("handler %q marshal: %w", handlerID, err)
		}

		nodes = append(nodes, WorkflowNodeV1{
			ID:    handlerID,
			Type:  WorkflowNodeTypeV1LLMResponses,
			Input: handlerRaw,
		})

		// Conditional edge from classifier to handler, plus edge from handler to join
		condition := WhenOutputEquals(string(r.routePath), route.Value)
		edges = append(edges,
			WorkflowEdgeV1{From: r.classifierID, To: handlerID, When: &condition},
			WorkflowEdgeV1{From: handlerID, To: joinID},
		)
	}

	// Add aggregator if configured
	outputFrom := joinID
	if r.aggregator != nil {
		aggID := r.aggregator.ID
		if aggID == "" {
			aggID = "aggregate"
		}
		placeholder := r.aggregator.Placeholder
		if placeholder == "" {
			placeholder = "route_output"
		}

		var stream *bool
		if r.aggregator.Stream {
			stream = BoolPtr(true)
		}

		aggPayload := llmResponsesNodeInputV1{
			Request:  newResponseRequestPayload(r.aggregator.Req),
			Stream:   stream,
			Bindings: []LLMResponsesBindingV1{BindToPlaceholderV1(joinID, placeholder)},
		}
		aggRaw, err := json.Marshal(aggPayload)
		if err != nil {
			return WorkflowSpecV1{}, fmt.Errorf("aggregator marshal: %w", err)
		}

		nodes = append(nodes, WorkflowNodeV1{
			ID:    aggID,
			Type:  WorkflowNodeTypeV1LLMResponses,
			Input: aggRaw,
		})
		edges = append(edges, WorkflowEdgeV1{From: joinID, To: aggID})
		outputFrom = aggID
	}

	// Sort edges for deterministic output
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		// Sort by when condition if both have one
		if edges[i].When != nil && edges[j].When != nil {
			wi, _ := json.Marshal(edges[i].When)
			wj, _ := json.Marshal(edges[j].When)
			return string(wi) < string(wj)
		}
		return edges[i].When == nil && edges[j].When != nil
	})

	outputs := []WorkflowOutputRefV1{{Name: r.outputName, From: outputFrom}}

	return WorkflowSpecV1{
		Kind:    WorkflowKindV1,
		Name:    r.name,
		Nodes:   nodes,
		Edges:   edges,
		Outputs: outputs,
	}, nil
}

// =============================================================================
// FanoutReduce Pattern
// =============================================================================

// FanoutReduceBuilderV1 builds the FanoutReduce pattern for workflow.v1.
//
// The fanout/reduce pattern generates a list of items, processes each item
// in parallel using a mapper node, then aggregates all results.
//
// Topology:
//
//	generator --> map.fanout(mapper) --> reducer
//
// Example:
//
//	spec, err := FanoutReduceV1("decompose_qa", generatorReq, mapperReq, reducerReq).
//	    ItemsPath("$.questions").
//	    MapperPlaceholder("question").
//	    ReducerPlaceholder("results").
//	    MaxParallelism(4).
//	    Build()
type FanoutReduceBuilderV1 struct {
	name               string
	generatorID        NodeID
	generatorReq       ResponseRequest
	generatorStream    bool
	itemsPath          workflow.JSONPath
	mapperReq          ResponseRequest
	mapperPlaceholder  PlaceholderName
	maxParallelism     *int64
	reducerID          NodeID
	reducerReq         ResponseRequest
	reducerStream      bool
	reducerPointer     JSONPointer
	reducerPlaceholder PlaceholderName
	reducerTo          JSONPointer
	outputName         OutputName
}

// FanoutReduceV1 creates a new FanoutReduceBuilderV1 with the given configuration.
func FanoutReduceV1(name string, generatorReq, mapperReq, reducerReq ResponseRequest) *FanoutReduceBuilderV1 {
	return &FanoutReduceBuilderV1{
		name:              name,
		generatorID:       "generator",
		generatorReq:      generatorReq,
		itemsPath:         "$.items",
		mapperReq:         mapperReq,
		mapperPlaceholder: "item",
		reducerID:         "reducer",
		reducerReq:        reducerReq,
		reducerPointer:    "/results",
		reducerPlaceholder: "results",
		outputName:        "final",
	}
}

// GeneratorID sets a custom ID for the generator node.
func (f *FanoutReduceBuilderV1) GeneratorID(id NodeID) *FanoutReduceBuilderV1 {
	f.generatorID = id
	return f
}

// GeneratorStream enables streaming for the generator node.
func (f *FanoutReduceBuilderV1) GeneratorStream() *FanoutReduceBuilderV1 {
	f.generatorStream = true
	return f
}

// ItemsPath sets the JSONPath to extract items from generator output.
// Defaults to "$.items".
func (f *FanoutReduceBuilderV1) ItemsPath(path string) *FanoutReduceBuilderV1 {
	f.itemsPath = workflow.JSONPath(path)
	return f
}

// MapperPlaceholder sets the placeholder name for item injection in the mapper.
// Defaults to "item".
func (f *FanoutReduceBuilderV1) MapperPlaceholder(placeholder PlaceholderName) *FanoutReduceBuilderV1 {
	f.mapperPlaceholder = placeholder
	return f
}

// MaxParallelism sets the maximum concurrent mapper executions.
func (f *FanoutReduceBuilderV1) MaxParallelism(n int64) *FanoutReduceBuilderV1 {
	f.maxParallelism = &n
	return f
}

// ReducerID sets a custom ID for the reducer node.
func (f *FanoutReduceBuilderV1) ReducerID(id NodeID) *FanoutReduceBuilderV1 {
	f.reducerID = id
	return f
}

// ReducerStream enables streaming for the reducer node.
func (f *FanoutReduceBuilderV1) ReducerStream() *FanoutReduceBuilderV1 {
	f.reducerStream = true
	return f
}

// ReducerPlaceholder sets the placeholder name for injecting fanout results.
// Defaults to "results".
func (f *FanoutReduceBuilderV1) ReducerPlaceholder(placeholder PlaceholderName) *FanoutReduceBuilderV1 {
	f.reducerPlaceholder = placeholder
	return f
}

// ReducerPointer sets the JSON pointer to extract from fanout output.
// Defaults to "/results".
func (f *FanoutReduceBuilderV1) ReducerPointer(pointer JSONPointer) *FanoutReduceBuilderV1 {
	f.reducerPointer = pointer
	return f
}

// ReducerTo sets a destination JSON pointer instead of using placeholder injection.
func (f *FanoutReduceBuilderV1) ReducerTo(to JSONPointer) *FanoutReduceBuilderV1 {
	f.reducerTo = to
	f.reducerPlaceholder = ""
	return f
}

// OutputName sets the workflow output name. Defaults to "final".
func (f *FanoutReduceBuilderV1) OutputName(name OutputName) *FanoutReduceBuilderV1 {
	f.outputName = name
	return f
}

// Build constructs the workflow specification.
func (f *FanoutReduceBuilderV1) Build() (WorkflowSpecV1, error) {
	var nodes []WorkflowNodeV1
	var edges []WorkflowEdgeV1

	// Add generator node
	var generatorStream *bool
	if f.generatorStream {
		generatorStream = BoolPtr(true)
	}
	generatorPayload := llmResponsesNodeInputV1{
		Request: newResponseRequestPayload(f.generatorReq),
		Stream:  generatorStream,
	}
	generatorRaw, err := json.Marshal(generatorPayload)
	if err != nil {
		return WorkflowSpecV1{}, fmt.Errorf("generator marshal: %w", err)
	}
	nodes = append(nodes, WorkflowNodeV1{
		ID:    f.generatorID,
		Type:  WorkflowNodeTypeV1LLMResponses,
		Input: generatorRaw,
	})

	// Add fanout node
	fanoutID := NodeID("__fanout")

	// Prepare mapper subnode
	mapperPayload := llmResponsesNodeInputV1{
		Request: newResponseRequestPayload(f.mapperReq),
	}
	mapperRaw, err := json.Marshal(mapperPayload)
	if err != nil {
		return WorkflowSpecV1{}, fmt.Errorf("mapper marshal: %w", err)
	}

	fanoutInput := MapFanoutNodeInputV1{
		Items: MapFanoutItemsV1{
			From: f.generatorID,
			Path: f.itemsPath,
		},
		ItemBindings: []MapFanoutItemBindingV1{{
			Path:          "$",
			ToPlaceholder: f.mapperPlaceholder,
			Encoding:      LLMResponsesBindingEncodingJSONStringV1,
		}},
		SubNode: MapFanoutSubNodeV1{
			ID:    "__mapper",
			Type:  WorkflowNodeTypeV1LLMResponses,
			Input: mapperRaw,
		},
		MaxParallelism: f.maxParallelism,
	}
	fanoutRaw, err := json.Marshal(fanoutInput)
	if err != nil {
		return WorkflowSpecV1{}, fmt.Errorf("fanout marshal: %w", err)
	}
	nodes = append(nodes, WorkflowNodeV1{
		ID:    fanoutID,
		Type:  WorkflowNodeTypeV1MapFanout,
		Input: fanoutRaw,
	})
	edges = append(edges, WorkflowEdgeV1{From: f.generatorID, To: fanoutID})

	// Add reducer node
	var reducerStream *bool
	if f.reducerStream {
		reducerStream = BoolPtr(true)
	}

	var reducerBinding LLMResponsesBindingV1
	if f.reducerPlaceholder != "" {
		reducerBinding = BindToPlaceholderWithPointerV1(fanoutID, f.reducerPointer, f.reducerPlaceholder)
	} else {
		reducerBinding = BindToPointerWithSourceV1(fanoutID, f.reducerPointer, f.reducerTo)
	}

	reducerPayload := llmResponsesNodeInputV1{
		Request:  newResponseRequestPayload(f.reducerReq),
		Stream:   reducerStream,
		Bindings: []LLMResponsesBindingV1{reducerBinding},
	}
	reducerRaw, err := json.Marshal(reducerPayload)
	if err != nil {
		return WorkflowSpecV1{}, fmt.Errorf("reducer marshal: %w", err)
	}
	nodes = append(nodes, WorkflowNodeV1{
		ID:    f.reducerID,
		Type:  WorkflowNodeTypeV1LLMResponses,
		Input: reducerRaw,
	})
	edges = append(edges, WorkflowEdgeV1{From: fanoutID, To: f.reducerID})

	// Sort edges for deterministic output
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})

	outputs := []WorkflowOutputRefV1{{Name: f.outputName, From: f.reducerID}}

	return WorkflowSpecV1{
		Kind:    WorkflowKindV1,
		Name:    f.name,
		Nodes:   nodes,
		Edges:   edges,
		Outputs: outputs,
	}, nil
}
