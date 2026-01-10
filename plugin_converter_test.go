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
						toolRef("fs.search"),
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
	for _, want := range []string{"fs.read_file", "fs.list_files", "fs.search", "fs.edit", "bash", "write_file"} {
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
						toolRef("fs.search"),
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
						{Tool: llm.Tool{Type: llm.ToolTypeWeb, Web: &llm.WebToolConfig{Intent: llm.WebIntentAuto}}},
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
