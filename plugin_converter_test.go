package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
	"github.com/modelrelay/modelrelay/sdk/go/workflowintent"
)

func TestPluginConverter_ToWorkflow_AssignsToolModes(t *testing.T) {
	t.Parallel()

	var gotReq responseRequestPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != routes.Responses {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		spec := WorkflowSpec{
			Kind: WorkflowKindIntent,
			Name: "converted",
			Nodes: []WorkflowIntentNode{
				{
					ID:   "fs_tools",
					Type: WorkflowNodeTypeLLM,
					User: "x",
					Tools: []workflowintent.ToolRef{
						toolRef("fs_search"),
					},
				},
				{
					ID:   "bash_tools",
					Type: WorkflowNodeTypeLLM,
					User: "x",
					Tools: []workflowintent.ToolRef{
						toolRef("bash"),
					},
					DependsOn: []string{"fs_tools"},
				},
			},
			Outputs: []WorkflowIntentOutputRef{{Name: "result", From: "bash_tools"}},
		}

		rawSpec, _ := json.Marshal(spec)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "id":"resp_1",
  "model":"claude-3-5-haiku-latest",
  "usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2},
  "output":[{"type":"message","role":"assistant","content":[{"type":"text","text":` + jsonString(string(rawSpec)) + `}]}]
}`))
	}))
	t.Cleanup(srv.Close)

	client, err := NewClientWithToken("tok", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("NewClientWithToken: %v", err)
	}

	plugin := Plugin{
		ID:  PluginID("octo/repo/plugins/my"),
		URL: PluginURL("github.com/octo/repo@main/plugins/my"),
		Commands: map[PluginCommandName]PluginCommand{
			PluginCommandName("analyze"): {Name: PluginCommandName("analyze"), Prompt: "# analyze"},
		},
		Agents:   map[PluginAgentName]PluginAgent{},
		RawFiles: map[PluginRepoPath]string{},
		Manifest: PluginManifest{Name: "x"},
		Ref:      PluginGitHubRef{Owner: GitHubOwner("octo"), Repo: GitHubRepo("repo"), Ref: GitHubRef("main"), Path: GitHubPath("plugins/my")},
	}

	converter := NewPluginConverter(client)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	out, err := converter.ToWorkflow(ctx, &plugin, "analyze", "do the thing")
	if err != nil {
		t.Fatalf("ToWorkflow() error: %v", err)
	}
	if out == nil || out.Kind != WorkflowKindIntent {
		t.Fatalf("unexpected spec: %#v", out)
	}

	if strings.TrimSpace(gotReq.Model) != "claude-3-5-haiku-latest" {
		t.Fatalf("unexpected converter model: %q", gotReq.Model)
	}
	if gotReq.OutputFormat == nil || gotReq.OutputFormat.Type != llm.OutputFormatTypeJSONSchema || gotReq.OutputFormat.JSONSchema == nil || gotReq.OutputFormat.JSONSchema.Name != "workflow" {
		t.Fatalf("expected json_schema output format, got: %#v", gotReq.OutputFormat)
	}
	if len(gotReq.Input) < 2 || gotReq.Input[0].Role != llm.RoleSystem || gotReq.Input[1].Role != llm.RoleUser {
		t.Fatalf("unexpected request input: %#v", gotReq.Input)
	}
	sys := gotReq.Input[0].Content[0].Text
	if !strings.Contains(sys, "workflow") {
		t.Fatalf("expected system prompt, got: %q", gotReq.Input[0].Content[0].Text)
	}
	if !strings.Contains(sys, "tools.v0") || !strings.Contains(sys, "docs/reference/tools.md") {
		t.Fatalf("expected tools.v0 contract reference in system prompt, got: %q", sys)
	}
	for _, want := range []string{"fs_read_file", "fs_list_files", "fs_search", "fs_edit", "bash", "write_file"} {
		if !strings.Contains(sys, want) {
			t.Fatalf("expected system prompt to mention %q, got: %q", want, sys)
		}
	}
	if !strings.Contains(sys, "Do NOT invent") {
		t.Fatalf("expected system prompt to forbid ad-hoc tool names, got: %q", sys)
	}

	if out.Nodes[0].ToolExecution == nil || out.Nodes[0].ToolExecution.Mode != "client" {
		t.Fatalf("expected client mode, got: %#v", out.Nodes[0].ToolExecution)
	}
	if out.Nodes[1].ToolExecution == nil || out.Nodes[1].ToolExecution.Mode != "client" {
		t.Fatalf("expected client mode, got: %#v", out.Nodes[1].ToolExecution)
	}
}

func TestPluginConverter_ToWorkflow_AllowsMixingFSAndBashTools(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != routes.Responses {
			http.NotFound(w, r)
			return
		}

		spec := WorkflowSpec{
			Kind: WorkflowKindIntent,
			Nodes: []WorkflowIntentNode{
				{
					ID:   "mixed",
					Type: WorkflowNodeTypeLLM,
					User: "x",
					Tools: []workflowintent.ToolRef{
						toolRef("bash"),
						toolRef("fs_search"),
					},
				},
			},
			Outputs: []WorkflowIntentOutputRef{{Name: "result", From: "mixed"}},
		}
		rawSpec, _ := json.Marshal(spec)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "id":"resp_1",
  "model":"claude-3-5-haiku-latest",
  "usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2},
  "output":[{"type":"message","role":"assistant","content":[{"type":"text","text":` + jsonString(string(rawSpec)) + `}]}]
}`))
	}))
	t.Cleanup(srv.Close)

	client, err := NewClientWithToken("tok", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("NewClientWithToken: %v", err)
	}

	plugin := Plugin{
		ID:  PluginID("octo/repo/plugins/my"),
		URL: PluginURL("github.com/octo/repo@main/plugins/my"),
		Commands: map[PluginCommandName]PluginCommand{
			PluginCommandName("analyze"): {Name: PluginCommandName("analyze"), Prompt: "# analyze"},
		},
		Agents:   map[PluginAgentName]PluginAgent{},
		RawFiles: map[PluginRepoPath]string{},
		Manifest: PluginManifest{Name: "x"},
		Ref:      PluginGitHubRef{Owner: GitHubOwner("octo"), Repo: GitHubRepo("repo"), Ref: GitHubRef("main"), Path: GitHubPath("plugins/my")},
	}

	converter := NewPluginConverter(client)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	out, err := converter.ToWorkflow(ctx, &plugin, "analyze", "do the thing")
	if err != nil {
		t.Fatalf("ToWorkflow() error: %v", err)
	}
	if out.Nodes[0].ToolExecution == nil || out.Nodes[0].ToolExecution.Mode != "client" {
		t.Fatalf("expected client mode, got: %#v", out.Nodes[0].ToolExecution)
	}
}

func TestPluginConverter_ToWorkflow_RejectsUnknownToolName(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != routes.Responses {
			http.NotFound(w, r)
			return
		}

		spec := WorkflowSpec{
			Kind: WorkflowKindIntent,
			Nodes: []WorkflowIntentNode{
				{
					ID:   "bad_tool",
					Type: WorkflowNodeTypeLLM,
					User: "x",
					Tools: []workflowintent.ToolRef{
						toolRef("repo.search"),
					},
				},
			},
			Outputs: []WorkflowIntentOutputRef{{Name: "result", From: "bad_tool"}},
		}
		rawSpec, _ := json.Marshal(spec)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "id":"resp_1",
  "model":"claude-3-5-haiku-latest",
  "usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2},
  "output":[{"type":"message","role":"assistant","content":[{"type":"text","text":` + jsonString(string(rawSpec)) + `}]}]
}`))
	}))
	t.Cleanup(srv.Close)

	client, err := NewClientWithToken("tok", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("NewClientWithToken: %v", err)
	}

	plugin := Plugin{
		ID:  PluginID("octo/repo/plugins/my"),
		URL: PluginURL("github.com/octo/repo@main/plugins/my"),
		Commands: map[PluginCommandName]PluginCommand{
			PluginCommandName("analyze"): {Name: PluginCommandName("analyze"), Prompt: "# analyze"},
		},
		Agents:   map[PluginAgentName]PluginAgent{},
		RawFiles: map[PluginRepoPath]string{},
		Manifest: PluginManifest{Name: "x"},
		Ref:      PluginGitHubRef{Owner: GitHubOwner("octo"), Repo: GitHubRepo("repo"), Ref: GitHubRef("main"), Path: GitHubPath("plugins/my")},
	}

	converter := NewPluginConverter(client)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	_, err = converter.ToWorkflow(ctx, &plugin, "analyze", "do the thing")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "unknown tool") || !strings.Contains(err.Error(), "repo.search") {
		t.Fatalf("expected unknown tool error, got: %v", err)
	}
}

func TestPluginConverter_ToWorkflow_RejectsNonFunctionTools(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != routes.Responses {
			http.NotFound(w, r)
			return
		}

		spec := WorkflowSpec{
			Kind: WorkflowKindIntent,
			Nodes: []WorkflowIntentNode{
				{
					ID:   "bad_type",
					Type: WorkflowNodeTypeLLM,
					User: "x",
					ToolExecution: &workflowintent.ToolExecution{
						Mode: "client",
					},
					Tools: []workflowintent.ToolRef{
						{Tool: llm.Tool{Type: llm.ToolTypeXSearch}},
					},
				},
			},
			Outputs: []WorkflowIntentOutputRef{{Name: "result", From: "bad_type"}},
		}
		rawSpec, _ := json.Marshal(spec)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "id":"resp_1",
  "model":"claude-3-5-haiku-latest",
  "usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2},
  "output":[{"type":"message","role":"assistant","content":[{"type":"text","text":` + jsonString(string(rawSpec)) + `}]}]
}`))
	}))
	t.Cleanup(srv.Close)

	client, err := NewClientWithToken("tok", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("NewClientWithToken: %v", err)
	}

	plugin := Plugin{
		ID:  PluginID("octo/repo/plugins/my"),
		URL: PluginURL("github.com/octo/repo@main/plugins/my"),
		Commands: map[PluginCommandName]PluginCommand{
			PluginCommandName("analyze"): {Name: PluginCommandName("analyze"), Prompt: "# analyze"},
		},
		Agents:   map[PluginAgentName]PluginAgent{},
		RawFiles: map[PluginRepoPath]string{},
		Manifest: PluginManifest{Name: "x"},
		Ref:      PluginGitHubRef{Owner: GitHubOwner("octo"), Repo: GitHubRepo("repo"), Ref: GitHubRef("main"), Path: GitHubPath("plugins/my")},
	}

	converter := NewPluginConverter(client)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	_, err = converter.ToWorkflow(ctx, &plugin, "analyze", "do the thing")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "only supports tools.v0 function tools") {
		t.Fatalf("expected non-function tool rejection, got: %v", err)
	}
}

func TestPluginConverter_ToWorkflowDynamic_BuildsWorkflowFromPlan(t *testing.T) {
	t.Parallel()

	var gotReq responseRequestPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == routes.Responses:
			if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}

			plan := OrchestrationPlanV1{
				Kind: orchestrationPlanKindV1,
				Steps: []OrchestrationPlanStepV1{
					{Agents: []OrchestrationPlanAgentV1{
						{ID: "reviewer", Reason: "Find bugs"},
						{ID: "tester", Reason: "Check tests"},
					}},
				},
			}

			rawPlan, _ := json.Marshal(plan)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
  "id":"resp_1",
  "model":"claude-3-5-haiku-latest",
  "usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2},
  "output":[{"type":"message","role":"assistant","content":[{"type":"text","text":` + jsonString(string(rawPlan)) + `}]}]
}`))
			return
		case r.Method == http.MethodGet && r.URL.Path == routes.Models:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
  "models": [
    {
      "model_id": "claude-3-5-haiku-latest",
      "provider": "anthropic",
      "display_name": "Claude",
      "description": "",
      "context_window": 1,
      "max_output_tokens": 1,
      "deprecated": false,
      "deprecation_message": "",
      "input_cost_per_million_cents": 1,
      "output_cost_per_million_cents": 1,
      "training_cutoff": "2025-01",
      "capabilities": ["tools"]
    }
  ]
}`))
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	t.Cleanup(srv.Close)

	client, err := NewClientWithToken("tok", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("NewClientWithToken: %v", err)
	}

	plugin := Plugin{
		ID:  PluginID("octo/repo/plugins/my"),
		URL: PluginURL("github.com/octo/repo@main/plugins/my"),
		Commands: map[PluginCommandName]PluginCommand{
			PluginCommandName("analyze"): {
				Name:      PluginCommandName("analyze"),
				Prompt:    "# analyze",
				AgentRefs: []PluginAgentName{"reviewer", "tester"},
			},
		},
		Agents: map[PluginAgentName]PluginAgent{
			"reviewer": {Name: "reviewer", SystemPrompt: "You review code.", Description: "Expert code reviewer."},
			"tester":   {Name: "tester", SystemPrompt: "You run tests.", Description: "Expert test runner."},
		},
		RawFiles: map[PluginRepoPath]string{},
		Manifest: PluginManifest{Name: "x"},
		Ref:      PluginGitHubRef{Owner: GitHubOwner("octo"), Repo: GitHubRepo("repo"), Ref: GitHubRef("main"), Path: GitHubPath("plugins/my")},
	}

	converter := NewPluginConverter(client)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	out, err := converter.ToWorkflowDynamic(ctx, &plugin, "analyze", "do the thing")
	if err != nil {
		t.Fatalf("ToWorkflowDynamic() error: %v", err)
	}
	if out == nil || out.Kind != WorkflowKindIntent {
		t.Fatalf("unexpected spec: %#v", out)
	}
	if strings.TrimSpace(out.Model) != "claude-3-5-haiku-latest" {
		t.Fatalf("unexpected model: %q", out.Model)
	}
	if gotReq.OutputFormat == nil || gotReq.OutputFormat.JSONSchema == nil || gotReq.OutputFormat.JSONSchema.Name != "orchestration_plan" {
		t.Fatalf("expected orchestration plan output format, got: %#v", gotReq.OutputFormat)
	}
	userPrompt := gotReq.Input[1].Content[0].Text
	if !strings.Contains(userPrompt, "Expert code reviewer.") || !strings.Contains(userPrompt, "Expert test runner.") {
		t.Fatalf("expected agent descriptions in orchestration prompt, got: %q", userPrompt)
	}

	ids := map[string]WorkflowIntentNode{}
	for _, node := range out.Nodes {
		ids[node.ID] = node
	}
	if _, ok := ids["agent_reviewer"]; !ok {
		t.Fatalf("expected reviewer node")
	}
	if node, ok := ids["agent_reviewer"]; ok {
		if node.ToolExecution == nil || node.ToolExecution.Mode != workflowintent.ToolExecutionModeClient {
			t.Fatalf("expected reviewer tool_execution client, got: %#v", node.ToolExecution)
		}
		if len(node.Tools) == 0 {
			t.Fatalf("expected reviewer tools")
		}
		found := map[string]struct{}{}
		for _, ref := range node.Tools {
			found[ref.Name()] = struct{}{}
		}
		for _, want := range []string{"fs_read_file", "fs_list_files", "fs_search"} {
			if _, ok := found[want]; !ok {
				t.Fatalf("expected tool %q in default tools", want)
			}
		}
		for _, ref := range node.Tools {
			if ref.Name() == "bash" || ref.Name() == "write_file" {
				t.Fatalf("did not expect bash/write_file in default tools")
			}
		}
	}
	if _, ok := ids["agent_tester"]; !ok {
		t.Fatalf("expected tester node")
	}
	if _, ok := ids["step_1_join"]; !ok {
		t.Fatalf("expected join node")
	}
	if node, ok := ids["orchestrator_synthesize"]; !ok {
		t.Fatalf("expected synth node")
	} else if len(node.DependsOn) != 1 || node.DependsOn[0] != "step_1_join" {
		t.Fatalf("unexpected synth dependencies: %#v", node.DependsOn)
	}
	if len(out.Outputs) != 1 || out.Outputs[0].From != "orchestrator_synthesize" {
		t.Fatalf("unexpected outputs: %#v", out.Outputs)
	}
}

func TestPluginConverter_ToWorkflowDynamic_RequiresAgentDescriptions(t *testing.T) {
	t.Parallel()

	client, err := NewClientWithToken("tok", WithBaseURL("http://example.invalid"))
	if err != nil {
		t.Fatalf("NewClientWithToken: %v", err)
	}

	plugin := Plugin{
		ID:  PluginID("octo/repo/plugins/my"),
		URL: PluginURL("github.com/octo/repo@main/plugins/my"),
		Commands: map[PluginCommandName]PluginCommand{
			PluginCommandName("analyze"): {Name: PluginCommandName("analyze"), Prompt: "# analyze"},
		},
		Agents: map[PluginAgentName]PluginAgent{
			"reviewer": {Name: "reviewer", SystemPrompt: "You review code.", Description: ""},
		},
		RawFiles: map[PluginRepoPath]string{},
		Manifest: PluginManifest{Name: "x"},
		Ref:      PluginGitHubRef{Owner: GitHubOwner("octo"), Repo: GitHubRepo("repo"), Ref: GitHubRef("main"), Path: GitHubPath("plugins/my")},
	}

	converter := NewPluginConverter(client)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	_, err = converter.ToWorkflowDynamic(ctx, &plugin, "analyze", "do the thing")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "missing description") {
		t.Fatalf("expected missing description error, got: %v", err)
	}
}

func TestPluginConverter_ToWorkflowDynamic_RejectsUnknownPlanAgents(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != routes.Responses {
			http.NotFound(w, r)
			return
		}

		plan := OrchestrationPlanV1{
			Kind: orchestrationPlanKindV1,
			Steps: []OrchestrationPlanStepV1{
				{Agents: []OrchestrationPlanAgentV1{
					{ID: "tester", Reason: "Not allowed"},
				}},
			},
		}

		rawPlan, _ := json.Marshal(plan)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "id":"resp_1",
  "model":"claude-3-5-haiku-latest",
  "usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2},
  "output":[{"type":"message","role":"assistant","content":[{"type":"text","text":` + jsonString(string(rawPlan)) + `}]}]
}`))
	}))
	t.Cleanup(srv.Close)

	client, err := NewClientWithToken("tok", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("NewClientWithToken: %v", err)
	}

	plugin := Plugin{
		ID:  PluginID("octo/repo/plugins/my"),
		URL: PluginURL("github.com/octo/repo@main/plugins/my"),
		Commands: map[PluginCommandName]PluginCommand{
			PluginCommandName("analyze"): {
				Name:      PluginCommandName("analyze"),
				Prompt:    "# analyze",
				AgentRefs: []PluginAgentName{"reviewer"},
			},
		},
		Agents: map[PluginAgentName]PluginAgent{
			"reviewer": {Name: "reviewer", SystemPrompt: "You review code.", Description: "Expert code reviewer."},
			"tester":   {Name: "tester", SystemPrompt: "You run tests.", Description: "Expert test runner."},
		},
		RawFiles: map[PluginRepoPath]string{},
		Manifest: PluginManifest{Name: "x"},
		Ref:      PluginGitHubRef{Owner: GitHubOwner("octo"), Repo: GitHubRepo("repo"), Ref: GitHubRef("main"), Path: GitHubPath("plugins/my")},
	}

	converter := NewPluginConverter(client)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	_, err = converter.ToWorkflowDynamic(ctx, &plugin, "analyze", "do the thing")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("expected unknown agent error, got: %v", err)
	}
}

func toolRef(name string) workflowintent.ToolRef {
	return workflowintent.ToolRef{
		Tool: llm.Tool{
			Type: llm.ToolTypeFunction,
			Function: &llm.FunctionTool{
				Name: llm.ToolName(name),
			},
		},
	}
}

func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
