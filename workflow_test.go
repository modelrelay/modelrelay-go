package sdk

import (
	"encoding/json"
	"testing"
)

func TestNewWorkflow_Basic(t *testing.T) {
	req, _, err := (ResponseBuilder{}).
		Model(NewModelID("test-model")).
		System("You are a helpful assistant.").
		User("Hello").
		Build()
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	spec, err := NewWorkflow("test_workflow").
		AddLLMNode("node1", req).Stream(true).
		Output("result", "node1").
		Build()
	if err != nil {
		t.Fatalf("build workflow: %v", err)
	}

	if spec.Name != "test_workflow" {
		t.Errorf("name = %q, want %q", spec.Name, "test_workflow")
	}
	if spec.Kind != WorkflowKindV0 {
		t.Errorf("kind = %q, want %q", spec.Kind, WorkflowKindV0)
	}
	if len(spec.Nodes) != 1 {
		t.Fatalf("nodes = %d, want 1", len(spec.Nodes))
	}
	if spec.Nodes[0].ID != "node1" {
		t.Errorf("node id = %q, want %q", spec.Nodes[0].ID, "node1")
	}
	if spec.Nodes[0].Type != WorkflowNodeTypeLLMResponses {
		t.Errorf("node type = %q, want %q", spec.Nodes[0].Type, WorkflowNodeTypeLLMResponses)
	}
	if len(spec.Outputs) != 1 {
		t.Fatalf("outputs = %d, want 1", len(spec.Outputs))
	}
	if spec.Outputs[0].Name != "result" {
		t.Errorf("output name = %q, want %q", spec.Outputs[0].Name, "result")
	}
}

func TestNewWorkflow_AutoEdgeFromBinding(t *testing.T) {
	reqA, _, err := (ResponseBuilder{}).
		Model(NewModelID("test-model")).
		User("Hello A").
		Build()
	if err != nil {
		t.Fatalf("build request A: %v", err)
	}

	reqB, _, err := (ResponseBuilder{}).
		Model(NewModelID("test-model")).
		User("Placeholder").
		Build()
	if err != nil {
		t.Fatalf("build request B: %v", err)
	}

	spec, err := NewWorkflow("binding_test").
		AddLLMNode("node_a", reqA).
		AddLLMNode("node_b", reqB).
			BindFrom("node_a", "/output/0/content/0/text").
		Output("result", "node_b").
		Build()
	if err != nil {
		t.Fatalf("build workflow: %v", err)
	}

	// Should have auto-inferred edge from binding
	if len(spec.Edges) != 1 {
		t.Fatalf("edges = %d, want 1 (auto-inferred)", len(spec.Edges))
	}
	if spec.Edges[0].From != "node_a" || spec.Edges[0].To != "node_b" {
		t.Errorf("edge = %v -> %v, want node_a -> node_b", spec.Edges[0].From, spec.Edges[0].To)
	}
}

func TestNewWorkflow_MultipleBindings(t *testing.T) {
	reqA, _, _ := (ResponseBuilder{}).Model(NewModelID("m")).User("a").Build()
	reqB, _, _ := (ResponseBuilder{}).Model(NewModelID("m")).User("b").Build()
	reqC, _, _ := (ResponseBuilder{}).Model(NewModelID("m")).User("c").Build()

	spec, err := NewWorkflow("multi_bind").
		AddLLMNode("agent_a", reqA).
		AddLLMNode("agent_b", reqB).
		AddLLMNode("aggregate", reqC).
			BindFromTo("agent_a", "/output/0", "/request/input/1/content/0/text", LLMResponsesBindingEncodingJSONString).
			BindFromTo("agent_b", "/output/0", "/request/input/2/content/0/text", LLMResponsesBindingEncodingJSONString).
		Output("final", "aggregate").
		Build()
	if err != nil {
		t.Fatalf("build workflow: %v", err)
	}

	// Should have 2 edges auto-inferred from bindings
	if len(spec.Edges) != 2 {
		t.Fatalf("edges = %d, want 2", len(spec.Edges))
	}

	edgeSet := make(map[string]bool)
	for _, e := range spec.Edges {
		edgeSet[string(e.From)+"->"+string(e.To)] = true
	}
	if !edgeSet["agent_a->aggregate"] {
		t.Error("missing edge agent_a -> aggregate")
	}
	if !edgeSet["agent_b->aggregate"] {
		t.Error("missing edge agent_b -> aggregate")
	}
}

func TestNewWorkflow_JoinAllNode(t *testing.T) {
	reqA, _, _ := (ResponseBuilder{}).Model(NewModelID("m")).User("a").Build()
	reqB, _, _ := (ResponseBuilder{}).Model(NewModelID("m")).User("b").Build()
	reqAgg, _, _ := (ResponseBuilder{}).Model(NewModelID("m")).User("agg").Build()

	spec, err := NewWorkflow("with_join").
		AddLLMNode("a", reqA).
		AddLLMNode("b", reqB).
		AddJoinAllNode("join").
		Edge("a", "join").
		Edge("b", "join").
		AddLLMNode("agg", reqAgg).
			BindFrom("join", "").
		Output("out", "agg").
		Build()
	if err != nil {
		t.Fatalf("build workflow: %v", err)
	}

	// Check nodes
	nodeTypes := make(map[string]WorkflowNodeType)
	for _, n := range spec.Nodes {
		nodeTypes[string(n.ID)] = n.Type
	}
	if nodeTypes["join"] != WorkflowNodeTypeJoinAll {
		t.Errorf("join node type = %q, want %q", nodeTypes["join"], WorkflowNodeTypeJoinAll)
	}

	// Check edges (3 explicit + 1 auto-inferred from binding)
	if len(spec.Edges) != 3 {
		t.Errorf("edges = %d, want 3", len(spec.Edges))
	}
}

func TestNewWorkflow_ExecutionConfig(t *testing.T) {
	req, _, _ := (ResponseBuilder{}).Model(NewModelID("m")).User("u").Build()

	maxPar := int64(5)
	nodeTimeout := int64(30000)
	runTimeout := int64(60000)

	spec, err := NewWorkflow("with_exec").
		Execution(WorkflowExecutionV0{
			MaxParallelism: &maxPar,
			NodeTimeoutMS:  &nodeTimeout,
			RunTimeoutMS:   &runTimeout,
		}).
		AddLLMNode("n", req).
		Output("o", "n").
		Build()
	if err != nil {
		t.Fatalf("build workflow: %v", err)
	}

	if spec.Execution == nil {
		t.Fatal("execution is nil")
	}
	if *spec.Execution.MaxParallelism != 5 {
		t.Errorf("max_parallelism = %d, want 5", *spec.Execution.MaxParallelism)
	}
	if *spec.Execution.NodeTimeoutMS != 30000 {
		t.Errorf("node_timeout_ms = %d, want 30000", *spec.Execution.NodeTimeoutMS)
	}
	if *spec.Execution.RunTimeoutMS != 60000 {
		t.Errorf("run_timeout_ms = %d, want 60000", *spec.Execution.RunTimeoutMS)
	}
}

func TestNewWorkflow_TransformJSONNode(t *testing.T) {
	reqA, _, _ := (ResponseBuilder{}).Model(NewModelID("m")).User("a").Build()
	reqB, _, _ := (ResponseBuilder{}).Model(NewModelID("m")).User("b").Build()

	spec, err := NewWorkflow("with_transform").
		AddLLMNode("a", reqA).
		AddLLMNode("b", reqB).
		AddTransformJSONNode("transform").
			Object(map[string]TransformJSONFieldRefV0{
				"result_a": {From: "a", Pointer: "/output/0/content/0/text"},
				"result_b": {From: "b", Pointer: "/output/0/content/0/text"},
			}).
		Output("combined", "transform").
		Build()
	if err != nil {
		t.Fatalf("build workflow: %v", err)
	}

	// Check nodes
	var transformNode *WorkflowNodeV0
	for i := range spec.Nodes {
		if spec.Nodes[i].ID == "transform" {
			transformNode = &spec.Nodes[i]
			break
		}
	}
	if transformNode == nil {
		t.Fatal("transform node not found")
	}
	if transformNode.Type != WorkflowNodeTypeTransformJSON {
		t.Errorf("transform node type = %q, want %q", transformNode.Type, WorkflowNodeTypeTransformJSON)
	}

	// Check auto-inferred edges from transform references
	if len(spec.Edges) != 2 {
		t.Errorf("edges = %d, want 2 (auto-inferred from transform)", len(spec.Edges))
	}
}

func TestNewWorkflow_ToolExecution(t *testing.T) {
	req, _, _ := (ResponseBuilder{}).Model(NewModelID("m")).User("u").Build()

	spec, err := NewWorkflow("with_tools").
		AddLLMNode("n", req).
			ToolExecution(ToolExecutionModeServer).
			ToolLimits(LLMResponsesToolLimitsV0{MaxLLMCallsPerNode: Int64Ptr(5)}).
		Output("o", "n").
		Build()
	if err != nil {
		t.Fatalf("build workflow: %v", err)
	}

	// Parse the node input to verify tool settings
	var input llmResponsesNodeInputV0
	if err := json.Unmarshal(spec.Nodes[0].Input, &input); err != nil {
		t.Fatalf("unmarshal node input: %v", err)
	}

	if input.ToolExecution == nil || input.ToolExecution.Mode != ToolExecutionModeServer {
		t.Error("tool_execution mode not set correctly")
	}
	if input.ToolLimits == nil || input.ToolLimits.MaxLLMCallsPerNode == nil || *input.ToolLimits.MaxLLMCallsPerNode != 5 {
		t.Error("tool_limits not set correctly")
	}
}

func TestNewWorkflow_OutputWithPointer(t *testing.T) {
	req, _, _ := (ResponseBuilder{}).Model(NewModelID("m")).User("u").Build()

	spec, err := NewWorkflow("with_pointer").
		AddLLMNode("n", req).
		OutputAt("text", "n", "/output/0/content/0/text").
		Build()
	if err != nil {
		t.Fatalf("build workflow: %v", err)
	}

	if len(spec.Outputs) != 1 {
		t.Fatalf("outputs = %d, want 1", len(spec.Outputs))
	}
	if spec.Outputs[0].Pointer != "/output/0/content/0/text" {
		t.Errorf("pointer = %q, want %q", spec.Outputs[0].Pointer, "/output/0/content/0/text")
	}
}

func TestNewWorkflow_EdgeDeduplication(t *testing.T) {
	req, _, _ := (ResponseBuilder{}).Model(NewModelID("m")).User("u").Build()

	spec, err := NewWorkflow("dedup").
		AddLLMNode("a", req).
		AddLLMNode("b", req).
			BindFrom("a", "/output").
		Edge("a", "b"). // Explicit edge that duplicates the auto-inferred one
		Output("o", "b").
		Build()
	if err != nil {
		t.Fatalf("build workflow: %v", err)
	}

	// Should have only 1 edge, not 2
	if len(spec.Edges) != 1 {
		t.Errorf("edges = %d, want 1 (deduplicated)", len(spec.Edges))
	}
}

func TestNewWorkflow_FluentChaining(t *testing.T) {
	reqA, _, _ := (ResponseBuilder{}).Model(NewModelID("m")).User("a").Build()
	reqB, _, _ := (ResponseBuilder{}).Model(NewModelID("m")).User("b").Build()
	reqC, _, _ := (ResponseBuilder{}).Model(NewModelID("m")).User("c").Build()

	// Test that all node builder methods return to workflow correctly
	spec, err := NewWorkflow("chaining").
		AddLLMNode("a", reqA).Stream(true).
		AddLLMNode("b", reqB).Stream(false).BindFrom("a", "/out").
		AddLLMNode("c", reqC).
			Stream(true).
			BindFromTo("b", "/x", "/y", LLMResponsesBindingEncodingJSONString).
			ToolExecution(ToolExecutionModeClient).
		Output("final", "c").
		Build()
	if err != nil {
		t.Fatalf("build workflow: %v", err)
	}

	if len(spec.Nodes) != 3 {
		t.Errorf("nodes = %d, want 3", len(spec.Nodes))
	}
	if len(spec.Edges) != 2 {
		t.Errorf("edges = %d, want 2", len(spec.Edges))
	}
}

// TestNewWorkflow_MatchesOldBuilder verifies the new API produces identical output to the old builder.
func TestNewWorkflow_MatchesOldBuilder(t *testing.T) {
	reqA, _, _ := (ResponseBuilder{}).Model(NewModelID("echo-1")).MaxOutputTokens(64).System("A").User("Hello A").Build()
	reqB, _, _ := (ResponseBuilder{}).Model(NewModelID("echo-1")).User("Hello B").Build()
	reqAgg, _, _ := (ResponseBuilder{}).Model(NewModelID("echo-1")).User("").Build()

	// Build with old API
	oldBuilder := WorkflowV0().Name("compare_test")
	oldBuilder, _ = oldBuilder.LLMResponsesNode("a", reqA, BoolPtr(false))
	oldBuilder, _ = oldBuilder.LLMResponsesNode("b", reqB, nil)
	oldBuilder = oldBuilder.JoinAllNode("join")
	oldBuilder, _ = oldBuilder.LLMResponsesNodeWithBindings("agg", reqAgg, nil, []LLMResponsesBindingV0{
		{From: "join", To: "/input/0/content/0/text", Encoding: LLMResponsesBindingEncodingJSONString},
	})
	oldBuilder = oldBuilder.
		Edge("a", "join").
		Edge("b", "join").
		Edge("join", "agg").
		Output("final", "agg", "/output/0/content/0/text")
	oldSpec, err := oldBuilder.Build()
	if err != nil {
		t.Fatalf("old builder: %v", err)
	}

	// Build with new API
	newSpec, err := NewWorkflow("compare_test").
		AddLLMNode("a", reqA).Stream(false).
		AddLLMNode("b", reqB).
		AddJoinAllNode("join").
		Edge("a", "join").
		Edge("b", "join").
		AddLLMNode("agg", reqAgg).
			BindFromTo("join", "", "/input/0/content/0/text", LLMResponsesBindingEncodingJSONString).
		OutputAt("final", "agg", "/output/0/content/0/text").
		Build()
	if err != nil {
		t.Fatalf("new builder: %v", err)
	}

	// Compare JSON representations
	oldJSON, _ := json.Marshal(oldSpec)
	newJSON, _ := json.Marshal(newSpec)

	if string(canonicalJSON(t, oldJSON)) != string(canonicalJSON(t, newJSON)) {
		t.Errorf("specs don't match\nold: %s\nnew: %s", oldJSON, newJSON)
	}
}
