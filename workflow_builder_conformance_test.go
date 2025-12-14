package sdk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/modelrelay/modelrelay/platform/workflow"
)

func repoRootForTest(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// file: .../sdk/go/workflow_builder_conformance_test.go
	// root: .../ (two levels up)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readFixtureBytes(t *testing.T, rel string) []byte {
	t.Helper()
	root := repoRootForTest(t)
	path := filepath.Join(root, rel)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
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
	fixture := readFixtureBytes(t, "platform/workflow/testdata/workflow_v0_parallel_agents.json")
	want := canonicalJSON(t, fixture)

	exec := workflow.ExecutionV0{
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

func TestValidateWorkflowSpecV0_ConformanceFixtures(t *testing.T) {
	cases := []struct {
		specRel   string
		issuesRel string
	}{
		{
			specRel:   "platform/workflow/testdata/workflow_v0_invalid_duplicate_node_id.json",
			issuesRel: "platform/workflow/testdata/workflow_v0_invalid_duplicate_node_id.issues.json",
		},
		{
			specRel:   "platform/workflow/testdata/workflow_v0_invalid_edge_unknown_node.json",
			issuesRel: "platform/workflow/testdata/workflow_v0_invalid_edge_unknown_node.issues.json",
		},
		{
			specRel:   "platform/workflow/testdata/workflow_v0_invalid_output_unknown_node.json",
			issuesRel: "platform/workflow/testdata/workflow_v0_invalid_output_unknown_node.issues.json",
		},
	}

	for _, tc := range cases {
		specBytes := readFixtureBytes(t, tc.specRel)
		var spec workflow.SpecV0
		if err := json.Unmarshal(specBytes, &spec); err != nil {
			t.Fatalf("unmarshal %s: %v", tc.specRel, err)
		}
		gotIssues := ValidateWorkflowSpecV0(spec)
		gotCodes := make([]string, 0, len(gotIssues))
		for _, iss := range gotIssues {
			gotCodes = append(gotCodes, string(iss.Code))
		}
		sort.Strings(gotCodes)

		wantBytes := readFixtureBytes(t, tc.issuesRel)
		var wantCodes []string
		if err := json.Unmarshal(wantBytes, &wantCodes); err != nil {
			t.Fatalf("unmarshal %s: %v", tc.issuesRel, err)
		}
		sort.Strings(wantCodes)

		if len(gotCodes) != len(wantCodes) {
			t.Fatalf("%s: codes mismatch\nwant: %v\ngot:  %v", tc.specRel, wantCodes, gotCodes)
		}
		for i := range gotCodes {
			if gotCodes[i] != wantCodes[i] {
				t.Fatalf("%s: codes mismatch\nwant: %v\ngot:  %v", tc.specRel, wantCodes, gotCodes)
			}
		}
	}
}
