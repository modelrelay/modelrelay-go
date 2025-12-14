package sdk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func conformanceWorkflowsV0DirForTest(t *testing.T) (string, bool) {
	t.Helper()

	if root := os.Getenv("MODELRELAY_CONFORMANCE_DIR"); root != "" {
		return filepath.Join(root, "workflows", "v0"), true
	}

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// sdk/go/workflow_builder_conformance_test.go -> repo root
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	internal := filepath.Join(repoRoot, "platform", "workflow", "testdata", "conformance", "workflows", "v0")
	if _, err := os.Stat(internal); err == nil {
		return internal, true
	}
	return "", false
}

func readConformanceWorkflowV0FixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	base, ok := conformanceWorkflowsV0DirForTest(t)
	if !ok {
		t.Skip("conformance fixtures not available (set MODELRELAY_CONFORMANCE_DIR)")
	}
	path := filepath.Join(base, name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read conformance fixture %s: %v", path, err)
	}
	return b
}

func canonicalJSON(t *testing.T, b []byte) []byte {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return out
}

func TestWorkflowBuilderV0_ConformanceParallelAgents(t *testing.T) {
	fixture := readConformanceWorkflowV0FixtureBytes(t, "workflow_v0_parallel_agents.json")
	want := canonicalJSON(t, fixture)

	exec := WorkflowExecutionV0{
		MaxParallelism: Int64Ptr(3),
		NodeTimeoutMS:  Int64Ptr(60_000),
		RunTimeoutMS:   Int64Ptr(180_000),
	}

	reqA, _, err := (ResponseBuilder{}).
		Model(NewModelID("echo-1")).
		MaxOutputTokens(64).
		System("You are Agent A.").
		User("Analyze the question.").
		Build()
	if err != nil {
		t.Fatalf("build agent_a request: %v", err)
	}

	reqB, _, err := (ResponseBuilder{}).
		Model(NewModelID("echo-1")).
		MaxOutputTokens(64).
		System("You are Agent B.").
		User("Find edge cases.").
		Build()
	if err != nil {
		t.Fatalf("build agent_b request: %v", err)
	}

	reqC, _, err := (ResponseBuilder{}).
		Model(NewModelID("echo-1")).
		MaxOutputTokens(64).
		System("You are Agent C.").
		User("Propose a solution.").
		Build()
	if err != nil {
		t.Fatalf("build agent_c request: %v", err)
	}

	reqAgg, _, err := (ResponseBuilder{}).
		Model(NewModelID("echo-1")).
		MaxOutputTokens(256).
		System("Synthesize the best answer.").
		Build()
	if err != nil {
		t.Fatalf("build aggregate request: %v", err)
	}

	b := WorkflowV0().
		Name("parallel_agents_aggregate").
		Execution(exec)

	b, err = b.LLMResponsesNode("agent_a", reqA, BoolPtr(false))
	if err != nil {
		t.Fatalf("add node agent_a: %v", err)
	}
	b, err = b.LLMResponsesNode("agent_b", reqB, nil)
	if err != nil {
		t.Fatalf("add node agent_b: %v", err)
	}
	b, err = b.LLMResponsesNode("agent_c", reqC, nil)
	if err != nil {
		t.Fatalf("add node agent_c: %v", err)
	}
	b = b.JoinAllNode("join")
	b, err = b.LLMResponsesNode("aggregate", reqAgg, nil)
	if err != nil {
		t.Fatalf("add node aggregate: %v", err)
	}

	b = b.
		Edge("agent_a", "join").
		Edge("agent_b", "join").
		Edge("agent_c", "join").
		Edge("join", "aggregate").
		Output("final", "aggregate", "")

	spec, err := b.Build()
	if err != nil {
		t.Fatalf("build workflow: %v", err)
	}

	gotBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	got := canonicalJSON(t, gotBytes)

	if string(got) != string(want) {
		t.Fatalf("spec mismatch\nwant: %s\ngot:  %s", want, got)
	}
}

func TestWorkflowBuilderV0_ConformanceBindingsJoinIntoAggregate(t *testing.T) {
	fixture := readConformanceWorkflowV0FixtureBytes(t, "workflow_v0_bindings_join_into_aggregate.json")
	want := canonicalJSON(t, fixture)

	reqA, _, err := (ResponseBuilder{}).
		Model(NewModelID("echo-1")).
		User("hello a").
		Build()
	if err != nil {
		t.Fatalf("build agent_a request: %v", err)
	}

	reqB, _, err := (ResponseBuilder{}).
		Model(NewModelID("echo-1")).
		User("hello b").
		Build()
	if err != nil {
		t.Fatalf("build agent_b request: %v", err)
	}

	reqAgg, _, err := (ResponseBuilder{}).
		Model(NewModelID("echo-1")).
		User("").
		Build()
	if err != nil {
		t.Fatalf("build aggregate request: %v", err)
	}

	b := WorkflowV0().Name("bindings_join_into_aggregate")

	b, err = b.LLMResponsesNode("agent_a", reqA, nil)
	if err != nil {
		t.Fatalf("add node agent_a: %v", err)
	}
	b, err = b.LLMResponsesNode("agent_b", reqB, nil)
	if err != nil {
		t.Fatalf("add node agent_b: %v", err)
	}
	b = b.JoinAllNode("join")
	b, err = b.LLMResponsesNodeWithBindings("aggregate", reqAgg, nil, []LLMResponsesBindingV0{
		{
			From:     "join",
			To:       "/input/0/content/0/text",
			Encoding: LLMResponsesBindingEncodingJSONString,
		},
	})
	if err != nil {
		t.Fatalf("add node aggregate: %v", err)
	}

	b = b.
		Edge("agent_a", "join").
		Edge("agent_b", "join").
		Edge("join", "aggregate").
		Output("final", "aggregate", "/output/0/content/0/text")

	spec, err := b.Build()
	if err != nil {
		t.Fatalf("build workflow: %v", err)
	}

	gotBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	got := canonicalJSON(t, gotBytes)

	if string(got) != string(want) {
		t.Fatalf("spec mismatch\nwant: %s\ngot:  %s", want, got)
	}
}
