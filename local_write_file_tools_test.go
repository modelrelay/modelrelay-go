package sdk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalWriteFileTools_DenyAllByDefault(t *testing.T) {
	root := t.TempDir()
	reg := NewLocalWriteFileTools(root)

	res := reg.Execute(toolCallJSON("write_file", map[string]any{"path": "x.txt", "contents": "hi"}))
	if res.Error == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(res.Error.Error(), "disabled by default") {
		t.Fatalf("expected deny-all error, got: %v", res.Error)
	}
}

func TestLocalWriteFileTools_SandboxTraversalAndCreateDirs(t *testing.T) {
	root := t.TempDir()

	t.Run("rejects traversal", func(t *testing.T) {
		reg := NewLocalWriteFileTools(root, WithLocalWriteFileAllow())
		res := reg.Execute(toolCallJSON("write_file", map[string]any{"path": "../x.txt", "contents": "hi"}))
		if res.Error == nil {
			t.Fatalf("expected error")
		}
		if _, ok := res.Error.(*ToolArgsError); !ok {
			t.Fatalf("expected ToolArgsError, got %T", res.Error)
		}
	})

	t.Run("parent missing unless createDirs enabled", func(t *testing.T) {
		reg := NewLocalWriteFileTools(root, WithLocalWriteFileAllow())
		res := reg.Execute(toolCallJSON("write_file", map[string]any{"path": "a/b.txt", "contents": "hi"}))
		if res.Error == nil {
			t.Fatalf("expected error")
		}

		reg = NewLocalWriteFileTools(root, WithLocalWriteFileAllow(), WithLocalWriteFileCreateDirs(true))
		res = reg.Execute(toolCallJSON("write_file", map[string]any{"path": "a/b.txt", "contents": "hi"}))
		if res.Error != nil {
			t.Fatalf("unexpected error: %v", res.Error)
		}
		got, err := os.ReadFile(filepath.Join(root, "a", "b.txt"))
		if err != nil {
			t.Fatalf("read written file: %v", err)
		}
		if string(got) != "hi" {
			t.Fatalf("expected %q, got %q", "hi", string(got))
		}
	})
}

func TestLocalWriteFileTools_MaxSizeAndSymlinkRejection(t *testing.T) {
	root := t.TempDir()

	t.Run("enforces max bytes", func(t *testing.T) {
		reg := NewLocalWriteFileTools(
			root,
			WithLocalWriteFileAllow(),
			WithLocalWriteFileCreateDirs(true),
			WithLocalWriteFileHardMaxBytes(100),
			WithLocalWriteFileMaxBytes(10),
		)
		res := reg.Execute(toolCallJSON("write_file", map[string]any{"path": "x.txt", "contents": strings.Repeat("a", 11)}))
		if res.Error == nil {
			t.Fatalf("expected error")
		}
		if _, ok := res.Error.(*ToolArgsError); !ok {
			t.Fatalf("expected ToolArgsError, got %T", res.Error)
		}
	})

	t.Run("rejects symlink target", func(t *testing.T) {
		outsideDir := t.TempDir()
		outsidePath := filepath.Join(outsideDir, "secret.txt")
		if err := os.WriteFile(outsidePath, []byte("secret"), 0o644); err != nil {
			t.Fatalf("write outside: %v", err)
		}
		linkPath := filepath.Join(root, "link.txt")
		if err := os.Symlink(outsidePath, linkPath); err != nil {
			t.Skipf("symlink not supported: %v", err)
		}

		reg := NewLocalWriteFileTools(root, WithLocalWriteFileAllow())
		res := reg.Execute(toolCallJSON("write_file", map[string]any{"path": "link.txt", "contents": "hi"}))
		if res.Error == nil {
			t.Fatalf("expected error")
		}
		if !strings.Contains(res.Error.Error(), "symlink") {
			t.Fatalf("expected symlink error, got: %v", res.Error)
		}
	})
}
