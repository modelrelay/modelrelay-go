package sdk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalBashTools_DenyAllByDefault(t *testing.T) {
	root := t.TempDir()
	reg := NewLocalBashTools(root)

	res := reg.Execute(toolCallJSON("bash", map[string]any{"command": "echo hi"}))
	if res.Error == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(res.Error.Error(), "disabled by default") {
		t.Fatalf("expected deny-all error, got: %v", res.Error)
	}
}

func TestLocalBashTools_TimeoutAndOutputCap(t *testing.T) {
	root := t.TempDir()

	t.Run("timeout", func(t *testing.T) {
		reg := NewLocalBashTools(
			root,
			WithLocalBashAllowAllCommands(),
			WithLocalBashTimeout(50*time.Millisecond),
		)
		res := reg.Execute(toolCallJSON("bash", map[string]any{"command": "sleep 2"}))
		if res.Error != nil {
			t.Fatalf("unexpected error: %v", res.Error)
		}
		out := res.Result.(BashResult)
		if !out.TimedOut {
			t.Fatalf("expected timed_out=true, got %+v", out)
		}
		if out.Error == "" {
			t.Fatalf("expected error string, got %+v", out)
		}
	})

	t.Run("output cap", func(t *testing.T) {
		reg := NewLocalBashTools(
			root,
			WithLocalBashAllowAllCommands(),
			WithLocalBashTimeout(2*time.Second),
			WithLocalBashHardMaxOutputBytes(1_000),
			WithLocalBashMaxOutputBytes(50),
		)
		res := reg.Execute(toolCallJSON("bash", map[string]any{"command": "printf 'a%.0s' {1..200}"}))
		if res.Error != nil {
			t.Fatalf("unexpected error: %v", res.Error)
		}
		out := res.Result.(BashResult)
		if !out.OutputTruncated {
			t.Fatalf("expected output_truncated=true, got %+v", out)
		}
		if len(out.Output) > 50 {
			t.Fatalf("expected output <= 50 bytes, got %d", len(out.Output))
		}
	})
}

func TestLocalBashTools_WorkingDirAndEnvPolicy(t *testing.T) {
	root := t.TempDir()
	rootFile := filepath.Join(root, "a.txt")
	if err := os.WriteFile(rootFile, []byte("x"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}

	const envKey = "MODELRELAY_LOCAL_BASH_TOOLS_TEST"
	t.Setenv(envKey, "secret")

	reg := NewLocalBashTools(
		root,
		WithLocalBashAllowAllCommands(),
	)

	t.Run("runs in root working directory", func(t *testing.T) {
		res := reg.Execute(toolCallJSON("bash", map[string]any{"command": "pwd"}))
		if res.Error != nil {
			t.Fatalf("unexpected error: %v", res.Error)
		}
		out := res.Result.(BashResult)
		if out.Output == "" {
			t.Fatalf("expected pwd output")
		}
		if !strings.Contains(out.Output, root) {
			t.Fatalf("expected pwd to include %q, got %q", root, out.Output)
		}
	})

	t.Run("default env does not inherit vars", func(t *testing.T) {
		res := reg.Execute(toolCallJSON("bash", map[string]any{"command": "echo -n \"$" + envKey + "\""}))
		if res.Error != nil {
			t.Fatalf("unexpected error: %v", res.Error)
		}
		out := res.Result.(BashResult)
		if out.Output != "" {
			t.Fatalf("expected empty output, got %q", out.Output)
		}
	})

	t.Run("allowlisted env vars are inherited", func(t *testing.T) {
		reg := NewLocalBashTools(
			root,
			WithLocalBashAllowAllCommands(),
			WithLocalBashAllowEnvVars(envKey),
		)
		res := reg.Execute(toolCallJSON("bash", map[string]any{"command": "echo -n \"$" + envKey + "\""}))
		if res.Error != nil {
			t.Fatalf("unexpected error: %v", res.Error)
		}
		out := res.Result.(BashResult)
		if out.Output != "secret" {
			t.Fatalf("expected %q, got %q", "secret", out.Output)
		}
	})
}
