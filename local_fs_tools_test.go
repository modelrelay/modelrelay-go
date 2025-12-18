package sdk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

func TestLocalFSTools_ReadFile_SandboxAndCaps(t *testing.T) {
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}

	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsidePath, []byte("secret"), 0o644); err != nil {
		t.Fatalf("write secret.txt: %v", err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(root, "link.txt")); err != nil {
		// Some environments disallow symlinks; don't fail the suite.
		t.Skipf("symlink not supported: %v", err)
	}

	reg := NewLocalFSTools(root)

	t.Run("reads file within root", func(t *testing.T) {
		res := reg.Execute(toolCallJSON("fs.read_file", map[string]any{"path": "a.txt"}))
		if res.Error != nil {
			t.Fatalf("unexpected error: %v", res.Error)
		}
		if got := res.Result.(string); got != "hello" {
			t.Fatalf("expected %q, got %q", "hello", got)
		}
	})

	t.Run("rejects traversal", func(t *testing.T) {
		res := reg.Execute(toolCallJSON("fs.read_file", map[string]any{"path": "../secret.txt"}))
		if res.Error == nil {
			t.Fatalf("expected error")
		}
		if _, ok := res.Error.(*ToolArgsError); !ok {
			t.Fatalf("expected ToolArgsError, got %T", res.Error)
		}
	})

	t.Run("rejects symlink escape", func(t *testing.T) {
		res := reg.Execute(toolCallJSON("fs.read_file", map[string]any{"path": "link.txt"}))
		if res.Error == nil {
			t.Fatalf("expected error")
		}
		if !strings.Contains(res.Error.Error(), "escapes root") {
			t.Fatalf("expected escape error, got: %v", res.Error)
		}
	})

	t.Run("enforces max_bytes", func(t *testing.T) {
		res := reg.Execute(toolCallJSON("fs.read_file", map[string]any{"path": "a.txt", "max_bytes": 2}))
		if res.Error == nil {
			t.Fatalf("expected error")
		}
		if strings.Contains(res.Error.Error(), "hard cap") {
			t.Fatalf("expected file size error, got: %v", res.Error)
		}
	})

	t.Run("rejects max_bytes over hard cap", func(t *testing.T) {
		res := reg.Execute(toolCallJSON("fs.read_file", map[string]any{"path": "a.txt", "max_bytes": localFSHardMaxReadBytes + 1}))
		if res.Error == nil {
			t.Fatalf("expected error")
		}
		if _, ok := res.Error.(*ToolArgsError); !ok {
			t.Fatalf("expected ToolArgsError, got %T", res.Error)
		}
	})
}

func TestLocalFSTools_ListFiles_IgnoreAndCap(t *testing.T) {
	root := t.TempDir()

	mustWrite(t, filepath.Join(root, "a.txt"), "a")
	mustWrite(t, filepath.Join(root, ".git", "config"), "secret")
	mustWrite(t, filepath.Join(root, "node_modules", "x.js"), "x")

	reg := NewLocalFSTools(root)

	t.Run("skips ignored dirs", func(t *testing.T) {
		res := reg.Execute(toolCallJSON("fs.list_files", map[string]any{"path": ".", "max_entries": 100}))
		if res.Error != nil {
			t.Fatalf("unexpected error: %v", res.Error)
		}
		out := res.Result.(string)
		if !strings.Contains(out, "a.txt") {
			t.Fatalf("expected a.txt in output, got: %q", out)
		}
		if strings.Contains(out, ".git/") || strings.Contains(out, "node_modules/") {
			t.Fatalf("expected ignored dirs to be excluded, got: %q", out)
		}
	})

	t.Run("enforces max_entries", func(t *testing.T) {
		res := reg.Execute(toolCallJSON("fs.list_files", map[string]any{"path": ".", "max_entries": 1}))
		if res.Error != nil {
			t.Fatalf("unexpected error: %v", res.Error)
		}
		lines := nonEmptyLines(res.Result.(string))
		if len(lines) != 1 {
			t.Fatalf("expected 1 entry, got %d: %q", len(lines), res.Result.(string))
		}
	})
}

func TestLocalFSTools_Search_MaxMatchesAndArgsValidation(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "foo\nfoo\nfoo\nbar\n")

	reg := NewLocalFSTools(root, WithLocalFSSearchTimeout(2*time.Second))

	t.Run("enforces max_matches", func(t *testing.T) {
		res := reg.Execute(toolCallJSON("fs.search", map[string]any{"query": "foo", "path": ".", "max_matches": 2}))
		if res.Error != nil {
			t.Fatalf("unexpected error: %v", res.Error)
		}
		lines := nonEmptyLines(res.Result.(string))
		if len(lines) != 2 {
			t.Fatalf("expected 2 matches, got %d: %q", len(lines), res.Result.(string))
		}
	})

	t.Run("invalid regex is ToolArgsError", func(t *testing.T) {
		res := reg.Execute(toolCallJSON("fs.search", map[string]any{"query": "(", "path": "."}))
		if res.Error == nil {
			t.Fatalf("expected error")
		}
		if _, ok := res.Error.(*ToolArgsError); !ok {
			t.Fatalf("expected ToolArgsError, got %T", res.Error)
		}
	})
}

func toolCallJSON(toolName string, args map[string]any) llm.ToolCall {
	b, _ := json.Marshal(args)
	return llm.ToolCall{
		ID:   "tc_1",
		Type: llm.ToolTypeFunction,
		Function: &llm.FunctionCall{
			Name:      toolName,
			Arguments: string(b),
		},
	}
}

func mustWrite(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func nonEmptyLines(s string) []string {
	raw := strings.Split(s, "\n")
	var out []string
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}
