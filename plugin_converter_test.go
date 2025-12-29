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

		spec := WorkflowSpecV0{
			Kind: WorkflowKindV0,
			Name: "converted",
			Nodes: []WorkflowNodeV0{
				{
					ID:   "fs_tools",
					Type: WorkflowNodeTypeLLMResponses,
					Input: mustJSON(llmResponsesNodeInputV0{
						Request: responseRequestPayload{
							Model: "x",
							Input: []llm.InputItem{llm.NewSystemText("x"), llm.NewUserText("x")},
							Tools: []llm.Tool{
								{Type: llm.ToolTypeFunction, Function: &llm.FunctionTool{Name: "fs.search"}},
							},
						},
					}),
				},
				{
					ID:   "bash_tools",
					Type: WorkflowNodeTypeLLMResponses,
					Input: mustJSON(llmResponsesNodeInputV0{
						Request: responseRequestPayload{
							Model: "x",
							Input: []llm.InputItem{llm.NewSystemText("x"), llm.NewUserText("x")},
							Tools: []llm.Tool{
								{Type: llm.ToolTypeFunction, Function: &llm.FunctionTool{Name: "bash"}},
							},
						},
					}),
				},
			},
			Edges:   []WorkflowEdgeV0{{From: "fs_tools", To: "bash_tools"}},
			Outputs: []WorkflowOutputRefV0{{Name: "result", From: "bash_tools"}},
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
	if out == nil || out.Kind != WorkflowKindV0 {
		t.Fatalf("unexpected spec: %#v", out)
	}

	if strings.TrimSpace(gotReq.Model) != "claude-3-5-haiku-latest" {
		t.Fatalf("unexpected converter model: %q", gotReq.Model)
	}
	if gotReq.OutputFormat == nil || gotReq.OutputFormat.Type != llm.OutputFormatTypeJSONSchema || gotReq.OutputFormat.JSONSchema == nil || gotReq.OutputFormat.JSONSchema.Name != "workflow_v0" {
		t.Fatalf("expected json_schema output format, got: %#v", gotReq.OutputFormat)
	}
	if len(gotReq.Input) < 2 || gotReq.Input[0].Role != llm.RoleSystem || gotReq.Input[1].Role != llm.RoleUser {
		t.Fatalf("unexpected request input: %#v", gotReq.Input)
	}
	sys := gotReq.Input[0].Content[0].Text
	if !strings.Contains(sys, "workflow.v0") {
		t.Fatalf("expected system prompt, got: %q", gotReq.Input[0].Content[0].Text)
	}
	if !strings.Contains(sys, "tools.v0") || !strings.Contains(sys, "docs/reference/tools-v0.md") {
		t.Fatalf("expected tools.v0 contract reference in system prompt, got: %q", sys)
	}
	for _, want := range []string{"fs.read_file", "fs.list_files", "fs.search", "bash", "write_file"} {
		if !strings.Contains(sys, want) {
			t.Fatalf("expected system prompt to mention %q, got: %q", want, sys)
		}
	}
	if !strings.Contains(sys, "Do NOT invent") {
		t.Fatalf("expected system prompt to forbid ad-hoc tool names, got: %q", sys)
	}

	var n0 llmResponsesNodeInputV0
	if err := json.Unmarshal(out.Nodes[0].Input, &n0); err != nil {
		t.Fatalf("unmarshal node input: %v", err)
	}
	if n0.ToolExecution == nil || n0.ToolExecution.Mode != ToolExecutionModeClient {
		t.Fatalf("expected client mode, got: %#v", n0.ToolExecution)
	}
	var n1 llmResponsesNodeInputV0
	if err := json.Unmarshal(out.Nodes[1].Input, &n1); err != nil {
		t.Fatalf("unmarshal node input: %v", err)
	}
	if n1.ToolExecution == nil || n1.ToolExecution.Mode != ToolExecutionModeClient {
		t.Fatalf("expected client mode, got: %#v", n1.ToolExecution)
	}
}

func TestPluginConverter_ToWorkflow_AllowsMixingFSAndBashTools(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != routes.Responses {
			http.NotFound(w, r)
			return
		}

		spec := WorkflowSpecV0{
			Kind: WorkflowKindV0,
			Nodes: []WorkflowNodeV0{
				{
					ID:   "mixed",
					Type: WorkflowNodeTypeLLMResponses,
					Input: mustJSON(llmResponsesNodeInputV0{
						Request: responseRequestPayload{
							Model: "x",
							Input: []llm.InputItem{llm.NewSystemText("x"), llm.NewUserText("x")},
							Tools: []llm.Tool{
								{Type: llm.ToolTypeFunction, Function: &llm.FunctionTool{Name: "bash"}},
								{Type: llm.ToolTypeFunction, Function: &llm.FunctionTool{Name: "fs.search"}},
							},
						},
					}),
				},
			},
			Outputs: []WorkflowOutputRefV0{{Name: "result", From: "mixed"}},
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
	var n0 llmResponsesNodeInputV0
	if err := json.Unmarshal(out.Nodes[0].Input, &n0); err != nil {
		t.Fatalf("unmarshal node input: %v", err)
	}
	if n0.ToolExecution == nil || n0.ToolExecution.Mode != ToolExecutionModeClient {
		t.Fatalf("expected client mode, got: %#v", n0.ToolExecution)
	}
}

func TestPluginConverter_ToWorkflow_RejectsUnknownToolName(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != routes.Responses {
			http.NotFound(w, r)
			return
		}

		spec := WorkflowSpecV0{
			Kind: WorkflowKindV0,
			Nodes: []WorkflowNodeV0{
				{
					ID:   "bad_tool",
					Type: WorkflowNodeTypeLLMResponses,
					Input: mustJSON(llmResponsesNodeInputV0{
						Request: responseRequestPayload{
							Model: "x",
							Input: []llm.InputItem{llm.NewSystemText("x"), llm.NewUserText("x")},
							Tools: []llm.Tool{
								{Type: llm.ToolTypeFunction, Function: &llm.FunctionTool{Name: "repo.search"}},
							},
						},
					}),
				},
			},
			Outputs: []WorkflowOutputRefV0{{Name: "result", From: "bad_tool"}},
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
	if !strings.Contains(err.Error(), "unsupported tool") || !strings.Contains(err.Error(), "repo.search") {
		t.Fatalf("expected unsupported tool error, got: %v", err)
	}
}

func TestPluginConverter_ToWorkflow_RejectsNonFunctionTools(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != routes.Responses {
			http.NotFound(w, r)
			return
		}

		spec := WorkflowSpecV0{
			Kind: WorkflowKindV0,
			Nodes: []WorkflowNodeV0{
				{
					ID:   "bad_type",
					Type: WorkflowNodeTypeLLMResponses,
					Input: mustJSON(llmResponsesNodeInputV0{
						Request: responseRequestPayload{
							Model: "x",
							Input: []llm.InputItem{llm.NewSystemText("x"), llm.NewUserText("x")},
							Tools: []llm.Tool{
								{Type: llm.ToolTypeWeb, Web: &llm.WebToolConfig{Intent: llm.WebIntentAuto}},
							},
						},
					}),
				},
			},
			Outputs: []WorkflowOutputRefV0{{Name: "result", From: "bad_type"}},
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

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}
