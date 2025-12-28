package sdk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func conformanceWorkflowsV1DirForTest(t *testing.T) (string, bool) {
	t.Helper()

	if root := os.Getenv("MODELRELAY_CONFORMANCE_DIR"); root != "" {
		return filepath.Join(root, "workflows", "v1"), true
	}

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// sdk/go/workflow_builder_conformance_v1_test.go -> repo root
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	internal := filepath.Join(repoRoot, "platform", "workflow", "testdata", "conformance", "workflows", "v1")
	if _, err := os.Stat(internal); err == nil {
		return internal, true
	}
	return "", false
}

func readConformanceWorkflowV1FixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	base, ok := conformanceWorkflowsV1DirForTest(t)
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

func TestWorkflowBuilderV1_ConformanceRouter(t *testing.T) {
	fixture := readConformanceWorkflowV1FixtureBytes(t, "workflow_v1_router.json")
	want := canonicalJSON(t, fixture)

	routerReq, _, err := (ResponseBuilder{}).
		Model(NewModelID("echo-1")).
		MaxOutputTokens(32).
		System("Return JSON with a single 'route' field.").
		User("Classify the request into billing or support.").
		Build()
	if err != nil {
		t.Fatalf("build router request: %v", err)
	}

	billingReq, _, err := (ResponseBuilder{}).
		Model(NewModelID("echo-1")).
		MaxOutputTokens(128).
		System("You are a billing specialist.").
		User("Handle the billing request.").
		Build()
	if err != nil {
		t.Fatalf("build billing request: %v", err)
	}

	supportReq, _, err := (ResponseBuilder{}).
		Model(NewModelID("echo-1")).
		MaxOutputTokens(128).
		System("You are a support specialist.").
		User("Handle the support request.").
		Build()
	if err != nil {
		t.Fatalf("build support request: %v", err)
	}

	aggReq, _, err := (ResponseBuilder{}).
		Model(NewModelID("echo-1")).
		MaxOutputTokens(256).
		System("Summarize the specialist output: {{route_output}}").
		Build()
	if err != nil {
		t.Fatalf("build aggregate request: %v", err)
	}

	condBilling, err := json.Marshal("billing")
	if err != nil {
		t.Fatalf("marshal billing condition: %v", err)
	}
	condSupport, err := json.Marshal("support")
	if err != nil {
		t.Fatalf("marshal support condition: %v", err)
	}

	b := WorkflowV1().
		Name("router_specialists").
		Execution(WorkflowExecutionV1{
			MaxParallelism: Int64Ptr(4),
			NodeTimeoutMS:  Int64Ptr(60_000),
			RunTimeoutMS:   Int64Ptr(180_000),
		})

	b, err = b.RouteSwitchNode("router", routerReq, nil)
	if err != nil {
		t.Fatalf("add router: %v", err)
	}
	b, err = b.LLMResponsesNode("billing_agent", billingReq, nil)
	if err != nil {
		t.Fatalf("add billing agent: %v", err)
	}
	b, err = b.LLMResponsesNode("support_agent", supportReq, nil)
	if err != nil {
		t.Fatalf("add support agent: %v", err)
	}
	b, err = b.JoinAnyNode("join", nil)
	if err != nil {
		t.Fatalf("add join: %v", err)
	}
	b, err = b.LLMResponsesNodeWithBindings("aggregate", aggReq, nil, []LLMResponsesBindingV1{
		{
			From:          "join",
			Pointer:       "",
			ToPlaceholder: "route_output",
			Encoding:      LLMResponsesBindingEncodingJSONStringV1,
		},
	})
	if err != nil {
		t.Fatalf("add aggregate: %v", err)
	}

	b = b.
		EdgeWhen("router", "billing_agent", ConditionV1{
			Source: ConditionSourceNodeOutput,
			Op:     ConditionOpEquals,
			Path:   "$.route",
			Value:  condBilling,
		}).
		EdgeWhen("router", "support_agent", ConditionV1{
			Source: ConditionSourceNodeOutput,
			Op:     ConditionOpEquals,
			Path:   "$.route",
			Value:  condSupport,
		}).
		Edge("billing_agent", "join").
		Edge("support_agent", "join").
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

func TestWorkflowBuilderV1_ConformanceFanout(t *testing.T) {
	fixture := readConformanceWorkflowV1FixtureBytes(t, "workflow_v1_fanout.json")
	want := canonicalJSON(t, fixture)

	generatorReq, _, err := (ResponseBuilder{}).
		Model(NewModelID("echo-1")).
		MaxOutputTokens(128).
		System("Return JSON with a 'questions' array.").
		User("Generate 3 subquestions.").
		Build()
	if err != nil {
		t.Fatalf("build generator request: %v", err)
	}

	mapperReq, _, err := (ResponseBuilder{}).
		Model(NewModelID("echo-1")).
		MaxOutputTokens(128).
		System("Answer the question: {{question}}").
		Build()
	if err != nil {
		t.Fatalf("build mapper request: %v", err)
	}

	aggReq, _, err := (ResponseBuilder{}).
		Model(NewModelID("echo-1")).
		MaxOutputTokens(256).
		System("Combine the answers: ").
		Build()
	if err != nil {
		t.Fatalf("build aggregate request: %v", err)
	}

	subNode, err := MapFanoutSubNodeLLMResponses("mapper", mapperReq, nil, LLMResponsesNodeOptionsV1{})
	if err != nil {
		t.Fatalf("map fanout subnode: %v", err)
	}

	fanoutInput := MapFanoutNodeInputV1{
		Items: MapFanoutItemsV1{From: "question_generator", Path: "/questions"},
		ItemBindings: []MapFanoutItemBindingV1{
			{
				ToPlaceholder: "question",
				Encoding:      LLMResponsesBindingEncodingJSONStringV1,
			},
		},
		SubNode:        subNode,
		MaxParallelism: Int64Ptr(4),
	}

	b := WorkflowV1().Name("fanout_questions")

	b, err = b.LLMResponsesNode("question_generator", generatorReq, nil)
	if err != nil {
		t.Fatalf("add generator: %v", err)
	}
	b, err = b.MapFanoutNode("fanout", fanoutInput)
	if err != nil {
		t.Fatalf("add fanout: %v", err)
	}
	b, err = b.LLMResponsesNodeWithBindings("aggregate", aggReq, nil, []LLMResponsesBindingV1{
		{
			From:     "fanout",
			Pointer:  "/results",
			To:       "/input/0/content/0/text",
			Encoding: LLMResponsesBindingEncodingJSONStringV1,
		},
	})
	if err != nil {
		t.Fatalf("add aggregate: %v", err)
	}

	b = b.
		Edge("question_generator", "fanout").
		Edge("fanout", "aggregate").
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
