package sdk

import (
	"encoding/json"
	"testing"
)

func TestChain_TwoSteps(t *testing.T) {
	req1, _, err := (ResponseBuilder{}).
		Model(NewModelID("claude-sonnet-4-20250514")).
		User("Step 1").
		Build()
	if err != nil {
		t.Fatal(err)
	}

	req2, _, err := (ResponseBuilder{}).
		Model(NewModelID("claude-sonnet-4-20250514")).
		User("Step 2").
		Build()
	if err != nil {
		t.Fatal(err)
	}

	spec, err := Chain("two_step_chain",
		LLMStep("step1", req1),
		LLMStep("step2", req2),
	).
		OutputLast("result").
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if spec.Name != "two_step_chain" {
		t.Errorf("expected name 'two_step_chain', got %q", spec.Name)
	}

	if len(spec.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(spec.Nodes))
	}

	if spec.Nodes[0].ID != "step1" {
		t.Errorf("expected first node ID 'step1', got %q", spec.Nodes[0].ID)
	}

	if spec.Nodes[1].ID != "step2" {
		t.Errorf("expected second node ID 'step2', got %q", spec.Nodes[1].ID)
	}

	// Verify edge from step1 to step2
	if len(spec.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(spec.Edges))
	}

	if spec.Edges[0].From != "step1" || spec.Edges[0].To != "step2" {
		t.Errorf("expected edge step1->step2, got %s->%s", spec.Edges[0].From, spec.Edges[0].To)
	}

	// Verify output
	if len(spec.Outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(spec.Outputs))
	}

	if spec.Outputs[0].Name != "result" || spec.Outputs[0].From != "step2" {
		t.Errorf("expected output 'result' from 'step2', got %q from %q",
			spec.Outputs[0].Name, spec.Outputs[0].From)
	}

	// Verify binding on step2
	var input2 llmResponsesNodeInputV0
	if err := json.Unmarshal(spec.Nodes[1].Input, &input2); err != nil {
		t.Fatalf("failed to unmarshal step2 input: %v", err)
	}

	if len(input2.Bindings) != 1 {
		t.Fatalf("expected 1 binding on step2, got %d", len(input2.Bindings))
	}

	if input2.Bindings[0].From != "step1" {
		t.Errorf("expected binding from 'step1', got %q", input2.Bindings[0].From)
	}
}

func TestChain_ThreeStepsWithStream(t *testing.T) {
	req1, _, _ := (ResponseBuilder{}).Model(NewModelID("model")).User("1").Build()
	req2, _, _ := (ResponseBuilder{}).Model(NewModelID("model")).User("2").Build()
	req3, _, _ := (ResponseBuilder{}).Model(NewModelID("model")).User("3").Build()

	spec, err := Chain("three_step_chain",
		LLMStep("a", req1),
		LLMStep("b", req2).WithStream(),
		LLMStep("c", req3).WithStream(),
	).
		Output("final", "c").
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(spec.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(spec.Nodes))
	}

	if len(spec.Edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(spec.Edges))
	}

	// Verify streaming is set on nodes b and c
	var inputB, inputC llmResponsesNodeInputV0
	if err := json.Unmarshal(spec.Nodes[1].Input, &inputB); err != nil {
		t.Fatalf("failed to unmarshal node b input: %v", err)
	}
	if err := json.Unmarshal(spec.Nodes[2].Input, &inputC); err != nil {
		t.Fatalf("failed to unmarshal node c input: %v", err)
	}

	if inputB.Stream == nil || !*inputB.Stream {
		t.Error("expected streaming enabled on node b")
	}

	if inputC.Stream == nil || !*inputC.Stream {
		t.Error("expected streaming enabled on node c")
	}
}

func TestChain_Empty(t *testing.T) {
	_, err := Chain("empty").Build()
	if err == nil {
		t.Error("expected error for empty chain")
	}
}

func TestChain_WithExecution(t *testing.T) {
	req1, _, _ := (ResponseBuilder{}).Model(NewModelID("model")).User("1").Build()

	maxPar := int64(5)
	exec := WorkflowExecutionV0{MaxParallelism: &maxPar}

	spec, err := Chain("with_exec",
		LLMStep("step1", req1),
	).
		Execution(exec).
		OutputLast("out").
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if spec.Execution == nil {
		t.Fatal("expected execution config")
	}

	if spec.Execution.MaxParallelism == nil || *spec.Execution.MaxParallelism != 5 {
		t.Error("expected max parallelism of 5")
	}
}

func TestParallel_ThreeNodes(t *testing.T) {
	reqA, _, _ := (ResponseBuilder{}).Model(NewModelID("model")).User("A").Build()
	reqB, _, _ := (ResponseBuilder{}).Model(NewModelID("model")).User("B").Build()
	reqC, _, _ := (ResponseBuilder{}).Model(NewModelID("model")).User("C").Build()

	spec, err := Parallel("three_parallel",
		LLMStep("a", reqA),
		LLMStep("b", reqB),
		LLMStep("c", reqC),
	).
		Output("out_a", "a").
		Output("out_b", "b").
		Output("out_c", "c").
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(spec.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(spec.Nodes))
	}

	// No edges for parallel-only (no aggregation)
	if len(spec.Edges) != 0 {
		t.Fatalf("expected 0 edges (no aggregation), got %d", len(spec.Edges))
	}

	if len(spec.Outputs) != 3 {
		t.Fatalf("expected 3 outputs, got %d", len(spec.Outputs))
	}
}

func TestParallel_WithAggregate(t *testing.T) {
	reqA, _, _ := (ResponseBuilder{}).Model(NewModelID("model")).User("A").Build()
	reqB, _, _ := (ResponseBuilder{}).Model(NewModelID("model")).User("B").Build()
	reqC, _, _ := (ResponseBuilder{}).Model(NewModelID("model")).User("C").Build()
	reqAgg, _, _ := (ResponseBuilder{}).Model(NewModelID("model")).User("Aggregate").Build()

	spec, err := Parallel("parallel_aggregate",
		LLMStep("a", reqA),
		LLMStep("b", reqB),
		LLMStep("c", reqC),
	).
		Aggregate("agg", reqAgg).
		Output("result", "agg").
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// 3 parallel nodes + 1 join node + 1 aggregator = 5 nodes
	if len(spec.Nodes) != 5 {
		t.Fatalf("expected 5 nodes, got %d", len(spec.Nodes))
	}

	// Check node types
	nodeTypes := make(map[NodeID]WorkflowNodeType)
	for _, n := range spec.Nodes {
		nodeTypes[n.ID] = n.Type
	}

	if nodeTypes["a"] != WorkflowNodeTypeLLMResponses {
		t.Errorf("expected node 'a' to be LLM responses, got %q", nodeTypes["a"])
	}

	if nodeTypes["agg_join"] != WorkflowNodeTypeJoinAll {
		t.Errorf("expected node 'agg_join' to be join.all, got %q", nodeTypes["agg_join"])
	}

	if nodeTypes["agg"] != WorkflowNodeTypeLLMResponses {
		t.Errorf("expected node 'agg' to be LLM responses, got %q", nodeTypes["agg"])
	}

	// Edges: a->join, b->join, c->join, join->agg = 4 edges
	if len(spec.Edges) != 4 {
		t.Fatalf("expected 4 edges, got %d", len(spec.Edges))
	}

	// Verify all parallel nodes connect to join
	edgeSet := make(map[string]bool)
	for _, e := range spec.Edges {
		edgeSet[string(e.From)+"->"+string(e.To)] = true
	}

	if !edgeSet["a->agg_join"] {
		t.Error("missing edge a->agg_join")
	}
	if !edgeSet["b->agg_join"] {
		t.Error("missing edge b->agg_join")
	}
	if !edgeSet["c->agg_join"] {
		t.Error("missing edge c->agg_join")
	}
	if !edgeSet["agg_join->agg"] {
		t.Error("missing edge agg_join->agg")
	}

	// Verify aggregator binding
	var aggInput llmResponsesNodeInputV0
	for _, n := range spec.Nodes {
		if n.ID == "agg" {
			if err := json.Unmarshal(n.Input, &aggInput); err != nil {
				t.Fatalf("failed to unmarshal agg input: %v", err)
			}
			break
		}
	}

	if len(aggInput.Bindings) != 1 {
		t.Fatalf("expected 1 binding on agg, got %d", len(aggInput.Bindings))
	}

	if aggInput.Bindings[0].From != "agg_join" {
		t.Errorf("expected binding from 'agg_join', got %q", aggInput.Bindings[0].From)
	}
}

func TestParallel_AggregateWithStream(t *testing.T) {
	reqA, _, _ := (ResponseBuilder{}).Model(NewModelID("model")).User("A").Build()
	reqAgg, _, _ := (ResponseBuilder{}).Model(NewModelID("model")).User("Aggregate").Build()

	spec, err := Parallel("stream_agg",
		LLMStep("a", reqA),
	).
		AggregateWithStream("agg", reqAgg).
		Output("result", "agg").
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Find aggregator and check streaming
	for _, n := range spec.Nodes {
		if n.ID == "agg" {
			var input llmResponsesNodeInputV0
			if err := json.Unmarshal(n.Input, &input); err != nil {
				t.Fatalf("failed to unmarshal agg input: %v", err)
			}

			if input.Stream == nil || !*input.Stream {
				t.Error("expected streaming enabled on aggregator")
			}
			return
		}
	}

	t.Error("aggregator node not found")
}

func TestParallel_Empty(t *testing.T) {
	_, err := Parallel("empty").Build()
	if err == nil {
		t.Error("expected error for empty parallel")
	}
}

func TestParallel_WithExecution(t *testing.T) {
	req1, _, _ := (ResponseBuilder{}).Model(NewModelID("model")).User("1").Build()

	maxPar := int64(10)
	exec := WorkflowExecutionV0{MaxParallelism: &maxPar}

	spec, err := Parallel("with_exec",
		LLMStep("step1", req1),
	).
		Execution(exec).
		Output("out", "step1").
		Build()

	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if spec.Execution == nil {
		t.Fatal("expected execution config")
	}

	if spec.Execution.MaxParallelism == nil || *spec.Execution.MaxParallelism != 10 {
		t.Error("expected max parallelism of 10")
	}
}

func TestLLMStep_WithStream(t *testing.T) {
	req, _, _ := (ResponseBuilder{}).Model(NewModelID("model")).User("test").Build()

	step := LLMStep("test", req)
	if step.Stream {
		t.Error("expected stream to be false by default")
	}

	streamStep := step.WithStream()
	if !streamStep.Stream {
		t.Error("expected stream to be true after WithStream()")
	}

	// Verify original is unchanged (value copy)
	if step.Stream {
		t.Error("original step should be unchanged")
	}
}
