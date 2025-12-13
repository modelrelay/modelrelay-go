package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelrelay/modelrelay/platform/routes"
	llm "github.com/modelrelay/modelrelay/providers"
)

// ptr is a helper to create string pointers for tests.
func ptr(s string) *string { return &s }

func TestStructuredDecodeError(t *testing.T) {
	err := StructuredDecodeError{
		RawJSON: `{"invalid": json}`,
		Message: "unexpected character 'j'",
		Attempt: 1,
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "decode error") {
		t.Errorf("expected error to mention decode error, got: %s", errStr)
	}
	if !strings.Contains(errStr, "attempt 1") {
		t.Errorf("expected error to mention attempt number, got: %s", errStr)
	}
	if !strings.Contains(errStr, "unexpected character") {
		t.Errorf("expected error to include message, got: %s", errStr)
	}
}

func TestStructuredExhaustedError(t *testing.T) {
	err := StructuredExhaustedError{
		LastRawJSON: `{"name": ""}`,
		AllAttempts: []AttemptRecord{
			{
				Attempt: 1,
				RawJSON: `{"name": 123}`,
				Error: StructuredErrorDetail{
					Kind:   StructuredErrorKindValidation,
					Issues: []ValidationIssue{{Path: ptr("name"), Message: "expected string"}},
				},
			},
			{
				Attempt: 2,
				RawJSON: `{"name": ""}`,
				Error: StructuredErrorDetail{
					Kind:   StructuredErrorKindValidation,
					Issues: []ValidationIssue{{Path: ptr("name"), Message: "string too short"}},
				},
			},
		},
		FinalError: StructuredErrorDetail{
			Kind:   StructuredErrorKindValidation,
			Issues: []ValidationIssue{{Path: ptr("name"), Message: "string too short"}},
		},
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "2 attempts") {
		t.Errorf("expected error to mention attempt count, got: %s", errStr)
	}
	if !strings.Contains(errStr, "string too short") {
		t.Errorf("expected error to include final error message, got: %s", errStr)
	}
}

func TestDefaultRetryHandler(t *testing.T) {
	handler := DefaultRetryHandler{}

	errDetail := StructuredErrorDetail{
		Kind: StructuredErrorKindValidation,
		Issues: []ValidationIssue{
			{Path: ptr("name"), Message: "expected string"},
			{Path: ptr("age"), Message: "expected integer"},
		},
	}

	items := handler.OnValidationError(1, `{}`, errDetail, []llm.InputItem{llm.NewUserText("Extract info")})
	if len(items) != 1 {
		t.Fatalf("expected 1 message, got %d", len(items))
	}
	if items[0].Role != llm.RoleUser {
		t.Fatalf("expected user role, got %s", items[0].Role)
	}
	text := ""
	if len(items[0].Content) > 0 {
		text = items[0].Content[0].Text
	}
	if !strings.Contains(text, "expected string") || !strings.Contains(text, "expected integer") {
		t.Fatalf("unexpected retry message text: %s", text)
	}
}

func TestStructuredHappyPath(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.Responses {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llm.Response{
			ID:    "resp_1",
			Model: "demo",
			Output: []llm.OutputItem{{
				Type:    llm.OutputItemTypeMessage,
				Role:    llm.RoleAssistant,
				Content: []llm.ContentPart{llm.TextPart(`{"name":"John","age":30}`)},
			}},
			Usage: llm.Usage{TotalTokens: 3},
		})
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	req, _, err := client.Responses.New().
		Model(NewModelID("demo")).
		User("Extract: John, 30").
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	result, err := Structured[Person](context.Background(), client.Responses, req, StructuredOptions{})
	if err != nil {
		t.Fatalf("structured: %v", err)
	}
	if result.Value.Name != "John" || result.Value.Age != 30 {
		t.Fatalf("unexpected value %+v", result.Value)
	}
}

