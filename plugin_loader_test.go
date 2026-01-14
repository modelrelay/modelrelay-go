package sdk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestPluginServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var reqs atomic.Int32

	manifest := `---
name: My Plugin
description: Test plugin
version: 1.2.3
---

# My Plugin

This description should be ignored because front matter exists.
`
	command := `---
tools:
  - fs_read_file
  - fs_search
---

# analyze

Use agents/reviewer.md to review changes.
`
	agent := `---
description: Expert reviewer
tools:
  - fs_read_file
---

# reviewer

You are a reviewer.`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs.Add(1)

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/raw/octo/repo/main/plugins/my/PLUGIN.md":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(manifest))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/raw/octo/repo/main/plugins/my/commands/analyze.md":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(command))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/raw/octo/repo/main/plugins/my/agents/reviewer.md":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(agent))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/repos/octo/repo/contents/plugins/my/commands":
			if strings.TrimSpace(r.URL.Query().Get("ref")) != "main" {
				http.Error(w, "missing ref query", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
  {"type":"file","name":"analyze.md","path":"plugins/my/commands/analyze.md"}
]`))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/repos/octo/repo/contents/plugins/my/agents":
			if strings.TrimSpace(r.URL.Query().Get("ref")) != "main" {
				http.Error(w, "missing ref query", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
  {"type":"file","name":"reviewer.md","path":"plugins/my/agents/reviewer.md"}
]`))
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))

	return srv, &reqs
}

func newTestPluginServerSkillManifest(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var reqs atomic.Int32

	manifest := `---
name: My Plugin
description: Test plugin (skill)
version: 9.9.9
---`

	command := `# analyze

hello`
	agent := `# reviewer

You are a reviewer.`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs.Add(1)

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/raw/octo/repo/main/plugins/my/PLUGIN.md":
			http.NotFound(w, r)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/raw/octo/repo/main/plugins/my/SKILL.md":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(manifest))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/raw/octo/repo/main/plugins/my/commands/analyze.md":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(command))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/raw/octo/repo/main/plugins/my/agents/reviewer.md":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(agent))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/repos/octo/repo/contents/plugins/my/commands":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
  {"type":"file","name":"analyze.md","path":"plugins/my/commands/analyze.md"}
]`))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/api/repos/octo/repo/contents/plugins/my/agents":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
  {"type":"file","name":"reviewer.md","path":"plugins/my/agents/reviewer.md"}
]`))
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))

	return srv, &reqs
}

func newTestLoader(srv *httptest.Server, now func() time.Time, ttl time.Duration) *PluginLoader {
	return NewPluginLoader(
		WithPluginLoaderAPIBaseURL(srv.URL+"/api"),
		WithPluginLoaderRawBaseURL(srv.URL+"/raw"),
		WithPluginLoaderNow(now),
		WithPluginLoaderCacheTTL(ttl),
	)
}

func TestPluginLoader_Load_NormalizesGitHubURLs(t *testing.T) {
	t.Parallel()

	srv, _ := newTestPluginServer(t)
	t.Cleanup(srv.Close)

	now := time.Date(2025, 12, 17, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "Canonical", in: "github.com/octo/repo@main/plugins/my", want: "github.com/octo/repo@main/plugins/my"},
		{name: "HTTPS_Tree", in: "https://github.com/octo/repo/tree/main/plugins/my", want: "github.com/octo/repo@main/plugins/my"},
		{name: "HTTPS_Blob_PluginFile", in: "https://github.com/octo/repo/blob/main/plugins/my/PLUGIN.md", want: "github.com/octo/repo@main/plugins/my"},
		{name: "Raw_PluginFile", in: "https://raw.githubusercontent.com/octo/repo/main/plugins/my/PLUGIN.md", want: "github.com/octo/repo@main/plugins/my"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			loader := newTestLoader(srv, func() time.Time { return now }, 1*time.Minute)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			t.Cleanup(cancel)

			p, err := loader.Load(ctx, tc.in)
			if err != nil {
				t.Fatalf("Load() error: %v", err)
			}
			if p == nil {
				t.Fatalf("Load() returned nil plugin")
			}
			if p.URL.String() != tc.want {
				t.Fatalf("unexpected canonical URL: got %q want %q", p.URL.String(), tc.want)
			}
			if p.ID.String() != "octo/repo/plugins/my" {
				t.Fatalf("unexpected plugin id: %q", p.ID.String())
			}
			if p.Ref.Owner.String() != "octo" || p.Ref.Repo.String() != "repo" || p.Ref.Ref.String() != "main" || p.Ref.Path.String() != "plugins/my" {
				t.Fatalf("unexpected ref: %#v", p.Ref)
			}
			if p.Manifest.Name != "My Plugin" || p.Manifest.Description != "Test plugin" || p.Manifest.Version != "1.2.3" {
				t.Fatalf("unexpected manifest: %#v", p.Manifest)
			}
			if len(p.Manifest.Commands) != 1 || p.Manifest.Commands[0].String() != "analyze" {
				t.Fatalf("unexpected manifest commands: %#v", p.Manifest.Commands)
			}
			if len(p.Manifest.Agents) != 1 || p.Manifest.Agents[0].String() != "reviewer" {
				t.Fatalf("unexpected manifest agents: %#v", p.Manifest.Agents)
			}
			cmd, ok := p.Commands[PluginCommandName("analyze")]
			if !ok || strings.TrimSpace(cmd.Prompt) == "" {
				t.Fatalf("missing analyze command: %#v", p.Commands)
			}
			if len(cmd.Tools) != 2 || cmd.Tools[0] != ToolNameFSReadFile || cmd.Tools[1] != ToolNameFSSearch {
				t.Fatalf("unexpected command tools: %#v", cmd.Tools)
			}
			if strings.HasPrefix(strings.TrimSpace(cmd.Prompt), "---") {
				t.Fatalf("expected frontmatter to be stripped from command prompt")
			}
			if len(cmd.AgentRefs) != 1 || cmd.AgentRefs[0].String() != "reviewer" {
				t.Fatalf("unexpected command agent refs: %#v", cmd.AgentRefs)
			}
			agent, ok := p.Agents[PluginAgentName("reviewer")]
			if !ok {
				t.Fatalf("missing reviewer agent: %#v", p.Agents)
			}
			if agent.Description != "Expert reviewer" {
				t.Fatalf("unexpected agent description: %q", agent.Description)
			}
			if len(agent.Tools) != 1 || agent.Tools[0] != ToolNameFSReadFile {
				t.Fatalf("unexpected agent tools: %#v", agent.Tools)
			}
			if strings.HasPrefix(strings.TrimSpace(agent.SystemPrompt), "---") {
				t.Fatalf("expected frontmatter to be stripped from agent system prompt")
			}
			if _, ok := p.RawFiles[PluginRepoPath("plugins/my/PLUGIN.md")]; !ok {
				t.Fatalf("missing raw PLUGIN.md")
			}
			if _, ok := p.RawFiles[PluginRepoPath("plugins/my/commands/analyze.md")]; !ok {
				t.Fatalf("missing raw command file")
			}
			if _, ok := p.RawFiles[PluginRepoPath("plugins/my/agents/reviewer.md")]; !ok {
				t.Fatalf("missing raw agent file")
			}
		})
	}
}

func TestPluginLoader_Cache_PreventsRedundantFetches(t *testing.T) {
	t.Parallel()

	srv, reqs := newTestPluginServer(t)
	t.Cleanup(srv.Close)

	now := time.Date(2025, 12, 17, 0, 0, 0, 0, time.UTC)
	loader := newTestLoader(srv, func() time.Time { return now }, 1*time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	p1, err := loader.Load(ctx, "github.com/octo/repo@main/plugins/my")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := reqs.Load(); got != 5 {
		t.Fatalf("expected 5 HTTP requests, got %d", got)
	}

	// Mutate the returned plugin; cache should remain immutable.
	p1.Commands[PluginCommandName("analyze")] = PluginCommand{Name: PluginCommandName("analyze"), Prompt: "mutated"}

	p2, err := loader.Load(ctx, "https://github.com/octo/repo/tree/main/plugins/my")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := reqs.Load(); got != 5 {
		t.Fatalf("expected cache hit (still 5 HTTP requests), got %d", got)
	}
	if strings.TrimSpace(p2.Commands[PluginCommandName("analyze")].Prompt) == "mutated" {
		t.Fatalf("expected cached plugin to be immutable (got mutated prompt)")
	}
}

func TestPluginLoader_Cache_ExpiresAfterTTL(t *testing.T) {
	t.Parallel()

	srv, reqs := newTestPluginServer(t)
	t.Cleanup(srv.Close)

	now := time.Date(2025, 12, 17, 0, 0, 0, 0, time.UTC)
	current := now
	loader := newTestLoader(srv, func() time.Time { return current }, 250*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	if _, err := loader.Load(ctx, "github.com/octo/repo@main/plugins/my"); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := reqs.Load(); got != 5 {
		t.Fatalf("expected 5 HTTP requests, got %d", got)
	}

	current = current.Add(500 * time.Millisecond)

	if _, err := loader.Load(ctx, "github.com/octo/repo@main/plugins/my"); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := reqs.Load(); got != 10 {
		t.Fatalf("expected cache expiry (10 HTTP requests), got %d", got)
	}
}

func TestPluginLoader_Load_FallsBackToSkillManifest(t *testing.T) {
	t.Parallel()

	srv, reqs := newTestPluginServerSkillManifest(t)
	t.Cleanup(srv.Close)

	now := time.Date(2025, 12, 17, 0, 0, 0, 0, time.UTC)
	loader := newTestLoader(srv, func() time.Time { return now }, 1*time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	p, err := loader.Load(ctx, "github.com/octo/repo@main/plugins/my")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if p.Manifest.Description != "Test plugin (skill)" || p.Manifest.Version != "9.9.9" {
		t.Fatalf("unexpected manifest: %#v", p.Manifest)
	}
	if _, ok := p.RawFiles[PluginRepoPath("plugins/my/SKILL.md")]; !ok {
		t.Fatalf("expected SKILL.md in raw_files")
	}
	if got := reqs.Load(); got != 6 {
		t.Fatalf("expected 6 HTTP requests (PLUGIN 404 + SKILL + lists + files), got %d", got)
	}
}
