package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

func TestSQLToolLoopClampLimit(t *testing.T) {
	if got := clampLimit(0, 5, 10); got != 5 {
		t.Fatalf("expected fallback limit, got %d", got)
	}
	if got := clampLimit(12, 5, 10); got != 10 {
		t.Fatalf("expected capped limit, got %d", got)
	}
	if got := clampLimit(7, 5, 10); got != 7 {
		t.Fatalf("expected passthrough limit, got %d", got)
	}
}

func TestSQLToolLoopSystemPromptIncludesFlags(t *testing.T) {
	prompt := sqlLoopSystemPrompt(3, 50, true, true, "extra")
	if prompt == "" {
		t.Fatalf("expected non-empty prompt")
	}
	if !containsAll(prompt, []string{"Maximum SQL attempts: 3", "result size <= 50", "schema inspection"}) {
		t.Fatalf("prompt missing expected content: %q", prompt)
	}
}

func TestSQLToolLoopNormalize_DisablesSampleRowsWithoutHandler(t *testing.T) {
	cfg := normalizeSQLToolLoopConfig(SQLToolLoopOptions{}, SQLToolLoopHandlers{})
	if cfg.sampleRowsEnabled {
		t.Fatalf("expected sampleRowsEnabled=false without handler")
	}
}

func TestSQLToolLoopRejectsNonReadOnlyValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sql/validate" {
			http.NotFound(w, r)
			return
		}
		resp := SQLValidateResponse{
			Valid:         true,
			ReadOnly:      false,
			NormalizedSql: "DELETE FROM users",
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	client := newTestClient(t, srv, "mr_sk_test123")
	executed := false
	handlers := SQLToolLoopHandlers{
		ListTables:    func(context.Context) ([]SQLTableInfo, error) { return []SQLTableInfo{}, nil },
		DescribeTable: func(context.Context, SQLDescribeTableArgs) (*SQLTableDescription, error) { return &SQLTableDescription{}, nil },
		ExecuteSQL: func(context.Context, SQLExecuteArgs) (*SQLExecuteResult, error) {
			executed = true
			return &SQLExecuteResult{}, nil
		},
	}
	cfg := normalizeSQLToolLoopConfig(SQLToolLoopOptions{
		RequireSchemaInspection: boolPtr(false),
	}, handlers)
	state := newSQLToolLoopState()
	tools := buildSQLToolRegistry(context.Background(), cfg, state, handlers, client.SQL)

	_, registry := tools.Build()
	call := llm.ToolCall{
		ID:   "call-1",
		Type: llm.ToolTypeFunction,
		Function: &llm.FunctionCall{
			Name:      ToolNameExecuteSQL,
			Arguments: `{"query":"DELETE FROM users"}`,
		},
	}
	result := registry.Execute(call)
	if result.Error == nil {
		t.Fatalf("expected error for non-read-only validation")
	}
	if executed {
		t.Fatalf("execute_sql should not run when validation is non-read-only")
	}
}

func TestSQLToolLoopQuickstartRequiresPolicyOrProfile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request: %s", r.URL.Path)
	}))
	t.Cleanup(srv.Close)

	client := newTestClient(t, srv, "mr_sk_test123")
	handlers := SQLToolLoopHandlers{
		ListTables:    func(context.Context) ([]SQLTableInfo, error) { return []SQLTableInfo{}, nil },
		DescribeTable: func(context.Context, SQLDescribeTableArgs) (*SQLTableDescription, error) { return &SQLTableDescription{}, nil },
		ExecuteSQL:    func(context.Context, SQLExecuteArgs) (*SQLExecuteResult, error) { return &SQLExecuteResult{}, nil },
	}

	_, err := client.SQLToolLoopQuickstart(context.Background(), "model", "prompt", handlers, nil, nil)
	if err == nil {
		t.Fatalf("expected error when policy/profile missing")
	}
}

func containsAll(haystack string, needles []string) bool {
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}

func boolPtr(v bool) *bool {
	return &v
}
