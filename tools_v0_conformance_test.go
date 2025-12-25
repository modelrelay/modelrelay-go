package sdk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

type toolsV0Fixtures struct {
	Workspace struct {
		Root string `json:"root"`
	} `json:"workspace"`
	Tools map[string]toolsV0ToolFixture `json:"tools"`
}

type toolsV0ToolFixture struct {
	SchemaInvalid []toolsV0Case         `json:"schema_invalid"`
	Behavior      []toolsV0BehaviorCase `json:"behavior"`
}

type toolsV0Case struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type toolsV0BehaviorCase struct {
	Name   string         `json:"name"`
	Args   map[string]any `json:"args"`
	Expect toolsV0Expect  `json:"expect"`
}

type toolsV0Expect struct {
	Error             *bool    `json:"error"`
	Retryable         *bool    `json:"retryable"`
	OutputEquals      *string  `json:"output_equals"`
	OutputContains    []string `json:"output_contains"`
	OutputContainsAny []string `json:"output_contains_any"`
	OutputExcludes    []string `json:"output_excludes"`
	ErrorContainsAny  []string `json:"error_contains_any"`
	MaxLines          *int     `json:"max_lines"`
	LineRegex         *string  `json:"line_regex"`
}

func conformanceToolsV0DirForTest(t *testing.T) (string, bool) {
	t.Helper()

	if root := os.Getenv("MODELRELAY_CONFORMANCE_DIR"); root != "" {
		return filepath.Join(root, "tools-v0"), true
	}

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// sdk/go/tools_v0_conformance_test.go -> repo root
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	internal := filepath.Join(repoRoot, "platform", "workflow", "testdata", "conformance", "tools-v0")
	if _, err := os.Stat(filepath.Join(internal, "fixtures.json")); err == nil {
		return internal, true
	}
	if isMonorepo(repoRoot) {
		t.Fatalf("tools.v0 conformance fixtures missing at %s (set MODELRELAY_CONFORMANCE_DIR)", internal)
	}
	return "", false
}

func isMonorepo(repoRoot string) bool {
	if _, err := os.Stat(filepath.Join(repoRoot, "go.work")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "platform")); err == nil {
		return true
	}
	return false
}

func readToolsV0Fixtures(t *testing.T) toolsV0Fixtures {
	t.Helper()
	base, ok := conformanceToolsV0DirForTest(t)
	if !ok {
		t.Skip("conformance fixtures not available (set MODELRELAY_CONFORMANCE_DIR)")
	}
	b, err := os.ReadFile(filepath.Join(base, "fixtures.json"))
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var fixtures toolsV0Fixtures
	if err := json.Unmarshal(b, &fixtures); err != nil {
		t.Fatalf("unmarshal fixtures: %v", err)
	}
	return fixtures
}

func toolCallFromArgs(name ToolName, args map[string]any) llm.ToolCall {
	b, _ := json.Marshal(args)
	return llm.ToolCall{
		ID:   ToolCallID("tc_conformance"),
		Type: llm.ToolTypeFunction,
		Function: &llm.FunctionCall{
			Name:      name,
			Arguments: string(b),
		},
	}
}

func TestToolsV0Conformance_LocalFS(t *testing.T) {
	fixtures := readToolsV0Fixtures(t)
	base, _ := conformanceToolsV0DirForTest(t)
	root := filepath.Join(base, fixtures.Workspace.Root)

	registry := NewLocalFSTools(root)

	cases := fixtures.Tools
	if cases == nil {
		t.Fatalf("fixtures missing tools")
	}

	assertSchemaInvalid := func(tool ToolName, c toolsV0Case) {
		res := registry.Execute(toolCallFromArgs(tool, c.Args))
		if res.Error == nil {
			t.Fatalf("%s schema_invalid %s: expected error", tool, c.Name)
		}
		if !res.IsRetryable {
			t.Fatalf("%s schema_invalid %s: expected retryable error, got %v", tool, c.Name, res.Error)
		}
	}

	assertBehavior := func(tool ToolName, c toolsV0BehaviorCase) {
		res := registry.Execute(toolCallFromArgs(tool, c.Args))
		if c.Expect.Error != nil {
			if *c.Expect.Error && res.Error == nil {
				t.Fatalf("%s behavior %s: expected error", tool, c.Name)
			}
			if !*c.Expect.Error && res.Error != nil {
				t.Fatalf("%s behavior %s: unexpected error: %v", tool, c.Name, res.Error)
			}
		}

		if c.Expect.Retryable != nil {
			if res.IsRetryable != *c.Expect.Retryable {
				t.Fatalf("%s behavior %s: retryable=%v, want %v", tool, c.Name, res.IsRetryable, *c.Expect.Retryable)
			}
		}

		if res.Error != nil {
			if len(c.Expect.ErrorContainsAny) > 0 {
				errText := res.Error.Error()
				matched := false
				for _, frag := range c.Expect.ErrorContainsAny {
					if strings.Contains(errText, frag) {
						matched = true
						break
					}
				}
				if !matched {
					t.Fatalf("%s behavior %s: error %q missing expected fragments %v", tool, c.Name, errText, c.Expect.ErrorContainsAny)
				}
			}
			return
		}

		out, _ := res.Result.(string)
		if c.Expect.OutputEquals != nil && out != *c.Expect.OutputEquals {
			t.Fatalf("%s behavior %s: output mismatch\nwant: %q\ngot:  %q", tool, c.Name, *c.Expect.OutputEquals, out)
		}
		for _, frag := range c.Expect.OutputContains {
			if !strings.Contains(out, frag) {
				t.Fatalf("%s behavior %s: output missing %q (got %q)", tool, c.Name, frag, out)
			}
		}
		if len(c.Expect.OutputContainsAny) > 0 {
			matched := false
			for _, frag := range c.Expect.OutputContainsAny {
				if strings.Contains(out, frag) {
					matched = true
					break
				}
			}
			if !matched {
				t.Fatalf("%s behavior %s: output missing any of %v (got %q)", tool, c.Name, c.Expect.OutputContainsAny, out)
			}
		}
		for _, frag := range c.Expect.OutputExcludes {
			if strings.Contains(out, frag) {
				t.Fatalf("%s behavior %s: output unexpectedly contains %q (got %q)", tool, c.Name, frag, out)
			}
		}
		if c.Expect.MaxLines != nil {
			lines := nonEmptyLines(out)
			if len(lines) > *c.Expect.MaxLines {
				t.Fatalf("%s behavior %s: expected <=%d lines, got %d: %q", tool, c.Name, *c.Expect.MaxLines, len(lines), out)
			}
		}
		if c.Expect.LineRegex != nil {
			re := regexp.MustCompile(*c.Expect.LineRegex)
			for _, line := range nonEmptyLines(out) {
				if !re.MatchString(line) {
					t.Fatalf("%s behavior %s: line %q does not match %s", tool, c.Name, line, *c.Expect.LineRegex)
				}
			}
		}
	}

	t.Run("fs.read_file", func(t *testing.T) {
		fixture, ok := cases[string(ToolNameFSReadFile)]
		if !ok {
			t.Fatalf("missing fs.read_file fixture")
		}
		for _, c := range fixture.SchemaInvalid {
			assertSchemaInvalid(ToolNameFSReadFile, c)
		}
		for _, c := range fixture.Behavior {
			assertBehavior(ToolNameFSReadFile, c)
		}
	})

	t.Run("fs.list_files", func(t *testing.T) {
		fixture, ok := cases[string(ToolNameFSListFiles)]
		if !ok {
			t.Fatalf("missing fs.list_files fixture")
		}
		for _, c := range fixture.SchemaInvalid {
			assertSchemaInvalid(ToolNameFSListFiles, c)
		}
		for _, c := range fixture.Behavior {
			assertBehavior(ToolNameFSListFiles, c)
		}
	})

	t.Run("fs.search", func(t *testing.T) {
		fixture, ok := cases[string(ToolNameFSSearch)]
		if !ok {
			t.Fatalf("missing fs.search fixture")
		}
		for _, c := range fixture.SchemaInvalid {
			assertSchemaInvalid(ToolNameFSSearch, c)
		}
		for _, c := range fixture.Behavior {
			assertBehavior(ToolNameFSSearch, c)
		}
	})
}
