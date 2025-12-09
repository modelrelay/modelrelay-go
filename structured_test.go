package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
		t.Errorf("Expected error to mention decode error, got: %s", errStr)
	}
	if !strings.Contains(errStr, "attempt 1") {
		t.Errorf("Expected error to mention attempt number, got: %s", errStr)
	}
	if !strings.Contains(errStr, "unexpected character") {
		t.Errorf("Expected error to include message, got: %s", errStr)
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
					Kind:    StructuredErrorKindValidation,
					Issues:  []ValidationIssue{{Path: ptr("name"), Message: "expected string"}},
				},
			},
			{
				Attempt: 2,
				RawJSON: `{"name": ""}`,
				Error: StructuredErrorDetail{
					Kind:    StructuredErrorKindValidation,
					Issues:  []ValidationIssue{{Path: ptr("name"), Message: "string too short"}},
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
		t.Errorf("Expected error to mention attempt count, got: %s", errStr)
	}
	if !strings.Contains(errStr, "string too short") {
		t.Errorf("Expected error to include final error message, got: %s", errStr)
	}
}

func TestStructuredExhaustedError_DecodeError(t *testing.T) {
	err := StructuredExhaustedError{
		LastRawJSON: "not json",
		AllAttempts: []AttemptRecord{
			{
				Attempt: 1,
				RawJSON: "not json",
				Error: StructuredErrorDetail{
					Kind:    StructuredErrorKindDecode,
					Message: "invalid character 'o'",
				},
			},
		},
		FinalError: StructuredErrorDetail{
			Kind:    StructuredErrorKindDecode,
			Message: "invalid character 'o'",
		},
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "invalid character") {
		t.Errorf("Expected error to include decode message, got: %s", errStr)
	}
}

func TestDefaultRetryHandler_DecodeError(t *testing.T) {
	handler := DefaultRetryHandler{}

	errDetail := StructuredErrorDetail{
		Kind:    StructuredErrorKindDecode,
		Message: "unexpected end of JSON",
	}

	msgs := handler.OnValidationError(1, `{"incomplete`, errDetail, []llm.ProxyMessage{
		{Role: "user", Content: "Extract info"},
	})

	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("Expected user role, got %s", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Content, "unexpected end of JSON") {
		t.Errorf("Expected error message in content, got: %s", msgs[0].Content)
	}
	if !strings.Contains(msgs[0].Content, "did not match") {
		t.Errorf("Expected retry instruction in content, got: %s", msgs[0].Content)
	}
}

func TestDefaultRetryHandler_ValidationError(t *testing.T) {
	handler := DefaultRetryHandler{}

	errDetail := StructuredErrorDetail{
		Kind: StructuredErrorKindValidation,
		Issues: []ValidationIssue{
			{Path: ptr("name"), Message: "expected string"},
			{Path: ptr("age"), Message: "expected integer"},
		},
	}

	msgs := handler.OnValidationError(2, `{"name": 123, "age": "invalid"}`, errDetail, []llm.ProxyMessage{
		{Role: "user", Content: "Extract info"},
	})

	if len(msgs) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "expected string") {
		t.Errorf("Expected first validation issue in content, got: %s", msgs[0].Content)
	}
	if !strings.Contains(msgs[0].Content, "expected integer") {
		t.Errorf("Expected second validation issue in content, got: %s", msgs[0].Content)
	}
	if !strings.Contains(msgs[0].Content, "name:") {
		t.Errorf("Expected path in content, got: %s", msgs[0].Content)
	}
}

func TestStructuredOptions_Defaults(t *testing.T) {
	opts := StructuredOptions{}

	if opts.MaxRetries != 0 {
		t.Errorf("Expected default MaxRetries=0, got %d", opts.MaxRetries)
	}
	if opts.RetryHandler != nil {
		t.Error("Expected default RetryHandler=nil")
	}
	if opts.SchemaName != "" {
		t.Errorf("Expected default SchemaName='', got %s", opts.SchemaName)
	}
}

func TestStructuredResult(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	result := StructuredResult[Person]{
		Value:     Person{Name: "John", Age: 30},
		Attempts:  1,
		RequestID: "req-123",
	}

	if result.Value.Name != "John" {
		t.Errorf("Expected Name=John, got %s", result.Value.Name)
	}
	if result.Value.Age != 30 {
		t.Errorf("Expected Age=30, got %d", result.Value.Age)
	}
	if result.Attempts != 1 {
		t.Errorf("Expected Attempts=1, got %d", result.Attempts)
	}
	if result.RequestID != "req-123" {
		t.Errorf("Expected RequestID=req-123, got %s", result.RequestID)
	}
}

// CustomRetryHandler for testing
type customRetryHandler struct {
	stopAfterAttempt int
	messagePrefix    string
}

func (h customRetryHandler) OnValidationError(
	attempt int,
	_ string,
	_ StructuredErrorDetail,
	_ []llm.ProxyMessage,
) []llm.ProxyMessage {
	if attempt >= h.stopAfterAttempt {
		return nil // Stop retrying
	}
	return []llm.ProxyMessage{
		{Role: "user", Content: h.messagePrefix + ": please try again"},
	}
}

func TestCustomRetryHandler(t *testing.T) {
	handler := customRetryHandler{stopAfterAttempt: 2, messagePrefix: "Custom"}

	errDetail := StructuredErrorDetail{Kind: StructuredErrorKindDecode, Message: "error"}

	// First attempt should return message
	msgs := handler.OnValidationError(1, "{}", errDetail, nil)
	if msgs == nil {
		t.Fatal("Expected retry message on attempt 1")
	}
	if !strings.Contains(msgs[0].Content, "Custom") {
		t.Errorf("Expected custom prefix, got: %s", msgs[0].Content)
	}

	// Second attempt should return nil (stop)
	msgs = handler.OnValidationError(2, "{}", errDetail, nil)
	if msgs != nil {
		t.Error("Expected nil on attempt 2 to stop retrying")
	}
}

// ============================================================================
// Schema Validation Tests
// ============================================================================

func TestValidateAgainstSchema_ValidJSON(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	responseFormat, err := ResponseFormatFromType[Person]("person")
	if err != nil {
		t.Fatalf("Failed to generate response format: %v", err)
	}

	schema, err := compileSchema(responseFormat.JSONSchema.Schema)
	if err != nil {
		t.Fatalf("Failed to compile schema: %v", err)
	}

	// Valid JSON should pass
	errDetail := validateAgainstSchema(schema, `{"name": "John", "age": 30}`)
	if errDetail != nil {
		t.Errorf("Expected valid JSON to pass validation, got: %v", errDetail)
	}
}

func TestValidateAgainstSchema_MissingRequiredField(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	responseFormat, err := ResponseFormatFromType[Person]("person")
	if err != nil {
		t.Fatalf("Failed to generate response format: %v", err)
	}

	schema, err := compileSchema(responseFormat.JSONSchema.Schema)
	if err != nil {
		t.Fatalf("Failed to compile schema: %v", err)
	}

	// Missing "age" field should fail validation
	errDetail := validateAgainstSchema(schema, `{"name": "John"}`)
	if errDetail == nil {
		t.Fatal("Expected validation error for missing required field")
	}
	if errDetail.Kind != StructuredErrorKindValidation {
		t.Errorf("Expected validation error kind, got: %s", errDetail.Kind)
	}
	if len(errDetail.Issues) == 0 {
		t.Error("Expected at least one validation issue")
	}
}

func TestValidateAgainstSchema_WrongType(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	responseFormat, err := ResponseFormatFromType[Person]("person")
	if err != nil {
		t.Fatalf("Failed to generate response format: %v", err)
	}

	schema, err := compileSchema(responseFormat.JSONSchema.Schema)
	if err != nil {
		t.Fatalf("Failed to compile schema: %v", err)
	}

	// Wrong type for "age" should fail validation
	errDetail := validateAgainstSchema(schema, `{"name": "John", "age": "thirty"}`)
	if errDetail == nil {
		t.Fatal("Expected validation error for wrong type")
	}
	if errDetail.Kind != StructuredErrorKindValidation {
		t.Errorf("Expected validation error kind, got: %s", errDetail.Kind)
	}

	// Check that the error mentions the problematic field
	found := false
	for _, issue := range errDetail.Issues {
		if issue.Path != nil && strings.Contains(*issue.Path, "age") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected validation issue to mention 'age' field, got: %v", errDetail.Issues)
	}
}

func TestValidateAgainstSchema_InvalidJSON(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	responseFormat, err := ResponseFormatFromType[Person]("person")
	if err != nil {
		t.Fatalf("Failed to generate response format: %v", err)
	}

	schema, err := compileSchema(responseFormat.JSONSchema.Schema)
	if err != nil {
		t.Fatalf("Failed to compile schema: %v", err)
	}

	// Invalid JSON should fail with decode error
	errDetail := validateAgainstSchema(schema, `{not valid json}`)
	if errDetail == nil {
		t.Fatal("Expected error for invalid JSON")
	}
	if errDetail.Kind != StructuredErrorKindDecode {
		t.Errorf("Expected decode error kind, got: %s", errDetail.Kind)
	}
}

func TestValidateAgainstSchema_NestedStruct(t *testing.T) {
	type Address struct {
		City    string `json:"city"`
		Country string `json:"country"`
	}
	type Person struct {
		Name    string  `json:"name"`
		Address Address `json:"address"`
	}

	responseFormat, err := ResponseFormatFromType[Person]("person")
	if err != nil {
		t.Fatalf("Failed to generate response format: %v", err)
	}

	schema, err := compileSchema(responseFormat.JSONSchema.Schema)
	if err != nil {
		t.Fatalf("Failed to compile schema: %v", err)
	}

	// Valid nested JSON should pass
	errDetail := validateAgainstSchema(schema, `{"name": "John", "address": {"city": "NYC", "country": "USA"}}`)
	if errDetail != nil {
		t.Errorf("Expected valid nested JSON to pass validation, got: %v", errDetail)
	}

	// Missing nested required field should fail
	errDetail = validateAgainstSchema(schema, `{"name": "John", "address": {"city": "NYC"}}`)
	if errDetail == nil {
		t.Fatal("Expected validation error for missing nested required field")
	}
}

func TestValidateAgainstSchema_OptionalField(t *testing.T) {
	type Person struct {
		Name     string  `json:"name"`
		Nickname *string `json:"nickname"` // Optional via pointer
	}

	responseFormat, err := ResponseFormatFromType[Person]("person")
	if err != nil {
		t.Fatalf("Failed to generate response format: %v", err)
	}

	schema, err := compileSchema(responseFormat.JSONSchema.Schema)
	if err != nil {
		t.Fatalf("Failed to compile schema: %v", err)
	}

	// Missing optional field should pass
	errDetail := validateAgainstSchema(schema, `{"name": "John"}`)
	if errDetail != nil {
		t.Errorf("Expected missing optional field to pass validation, got: %v", errDetail)
	}

	// With optional field should also pass
	errDetail = validateAgainstSchema(schema, `{"name": "John", "nickname": "Johnny"}`)
	if errDetail != nil {
		t.Errorf("Expected valid JSON with optional field to pass validation, got: %v", errDetail)
	}
}

func TestValidateAgainstSchema_ArrayField(t *testing.T) {
	type Person struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}

	responseFormat, err := ResponseFormatFromType[Person]("person")
	if err != nil {
		t.Fatalf("Failed to generate response format: %v", err)
	}

	schema, err := compileSchema(responseFormat.JSONSchema.Schema)
	if err != nil {
		t.Fatalf("Failed to compile schema: %v", err)
	}

	// Valid array should pass
	errDetail := validateAgainstSchema(schema, `{"name": "John", "tags": ["developer", "golang"]}`)
	if errDetail != nil {
		t.Errorf("Expected valid array to pass validation, got: %v", errDetail)
	}

	// Wrong type in array should fail
	errDetail = validateAgainstSchema(schema, `{"name": "John", "tags": [1, 2, 3]}`)
	if errDetail == nil {
		t.Fatal("Expected validation error for wrong array element type")
	}
}

func TestExtractValidationIssues_PathFormatting(t *testing.T) {
	type Address struct {
		City string `json:"city"`
	}
	type Person struct {
		Name    string  `json:"name"`
		Address Address `json:"address"`
	}

	responseFormat, err := ResponseFormatFromType[Person]("person")
	if err != nil {
		t.Fatalf("Failed to generate response format: %v", err)
	}

	schema, err := compileSchema(responseFormat.JSONSchema.Schema)
	if err != nil {
		t.Fatalf("Failed to compile schema: %v", err)
	}

	// Wrong type for nested field
	errDetail := validateAgainstSchema(schema, `{"name": "John", "address": {"city": 123}}`)
	if errDetail == nil {
		t.Fatal("Expected validation error")
	}

	// Check that nested paths are properly formatted (e.g., "address.city")
	foundNestedPath := false
	for _, issue := range errDetail.Issues {
		if issue.Path != nil && strings.Contains(*issue.Path, "address") {
			foundNestedPath = true
			// Path should use dots, not slashes
			if strings.Contains(*issue.Path, "/") {
				t.Errorf("Path should use dot notation, got: %s", *issue.Path)
			}
		}
	}
	if !foundNestedPath {
		t.Errorf("Expected to find nested path in issues: %v", errDetail.Issues)
	}
}

// ============================================================================
// Integration Tests for Structured() Function
// ============================================================================

// mockProxyResponse creates a JSON response matching the ProxyResponse format
func mockProxyResponse(content string) []byte {
	resp := struct {
		Content    []string `json:"content"`
		StopReason string   `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}{
		Content:    []string{content},
		StopReason: "end_turn",
	}
	resp.Usage.InputTokens = 10
	resp.Usage.OutputTokens = 20
	resp.Usage.TotalTokens = 30
	data, _ := json.Marshal(resp)
	return data
}

func TestStructured_HappyPath(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	// Create mock server that returns valid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-ModelRelay-Chat-Request-Id", "test-req-123")
		w.Write(mockProxyResponse(`{"name": "John", "age": 30}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	result, err := Structured[Person](context.Background(), client.LLM, ProxyRequest{
		Model:    NewModelID("test-model"),
		Messages: []llm.ProxyMessage{{Role: "user", Content: "Extract: John is 30"}},
	}, StructuredOptions{})

	if err != nil {
		t.Fatalf("Structured() failed: %v", err)
	}
	if result.Value.Name != "John" {
		t.Errorf("Expected Name=John, got %s", result.Value.Name)
	}
	if result.Value.Age != 30 {
		t.Errorf("Expected Age=30, got %d", result.Value.Age)
	}
	if result.Attempts != 1 {
		t.Errorf("Expected Attempts=1, got %d", result.Attempts)
	}
	if result.RequestID != "test-req-123" {
		t.Errorf("Expected RequestID=test-req-123, got %s", result.RequestID)
	}
}

func TestStructured_ValidationFailure_NoRetries(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	// Create mock server that returns invalid JSON (wrong type for age)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockProxyResponse(`{"name": "John", "age": "thirty"}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	_, err = Structured[Person](context.Background(), client.LLM, ProxyRequest{
		Model:    NewModelID("test-model"),
		Messages: []llm.ProxyMessage{{Role: "user", Content: "Extract: John is thirty"}},
	}, StructuredOptions{MaxRetries: 0})

	if err == nil {
		t.Fatal("Expected error for validation failure")
	}

	var exhausted StructuredExhaustedError
	if !errors.As(err, &exhausted) {
		t.Fatalf("Expected StructuredExhaustedError, got: %T", err)
	}
	if len(exhausted.AllAttempts) != 1 {
		t.Errorf("Expected 1 attempt, got %d", len(exhausted.AllAttempts))
	}
	if exhausted.FinalError.Kind != StructuredErrorKindValidation {
		t.Errorf("Expected validation error, got: %s", exhausted.FinalError.Kind)
	}
}

func TestStructured_RetryOnValidationFailure(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-ModelRelay-Chat-Request-Id", "test-req-retry")

		if requestCount == 1 {
			// First request: return invalid data
			w.Write(mockProxyResponse(`{"name": "John", "age": "thirty"}`))
		} else {
			// Retry: return valid data
			w.Write(mockProxyResponse(`{"name": "John", "age": 30}`))
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	result, err := Structured[Person](context.Background(), client.LLM, ProxyRequest{
		Model:    NewModelID("test-model"),
		Messages: []llm.ProxyMessage{{Role: "user", Content: "Extract: John is 30"}},
	}, StructuredOptions{MaxRetries: 2})

	if err != nil {
		t.Fatalf("Structured() failed: %v", err)
	}
	if result.Attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", result.Attempts)
	}
	if result.Value.Name != "John" {
		t.Errorf("Expected Name=John, got %s", result.Value.Name)
	}
	if result.Value.Age != 30 {
		t.Errorf("Expected Age=30, got %d", result.Value.Age)
	}
	if requestCount != 2 {
		t.Errorf("Expected 2 HTTP requests, got %d", requestCount)
	}
}

func TestStructured_ExhaustedRetries(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		// Always return invalid data
		w.Write(mockProxyResponse(`{"name": "John", "age": "invalid"}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	_, err = Structured[Person](context.Background(), client.LLM, ProxyRequest{
		Model:    NewModelID("test-model"),
		Messages: []llm.ProxyMessage{{Role: "user", Content: "Extract: John is invalid"}},
	}, StructuredOptions{MaxRetries: 2})

	if err == nil {
		t.Fatal("Expected error after exhausting retries")
	}

	var exhausted StructuredExhaustedError
	if !errors.As(err, &exhausted) {
		t.Fatalf("Expected StructuredExhaustedError, got: %T", err)
	}
	if len(exhausted.AllAttempts) != 3 { // 1 initial + 2 retries
		t.Errorf("Expected 3 attempts, got %d", len(exhausted.AllAttempts))
	}
	if requestCount != 3 {
		t.Errorf("Expected 3 HTTP requests, got %d", requestCount)
	}
}

func TestStructured_CustomRetryHandlerStopsEarly(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		// Always return invalid data
		w.Write(mockProxyResponse(`{"name": "John", "age": "invalid"}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Custom handler that stops after first attempt
	handler := customRetryHandler{stopAfterAttempt: 1, messagePrefix: "Stop"}

	_, err = Structured[Person](context.Background(), client.LLM, ProxyRequest{
		Model:    NewModelID("test-model"),
		Messages: []llm.ProxyMessage{{Role: "user", Content: "Extract: John is invalid"}},
	}, StructuredOptions{MaxRetries: 5, RetryHandler: handler})

	if err == nil {
		t.Fatal("Expected error when handler stops retrying")
	}

	var exhausted StructuredExhaustedError
	if !errors.As(err, &exhausted) {
		t.Fatalf("Expected StructuredExhaustedError, got: %T", err)
	}
	// Handler returns nil on attempt 1, so only 1 attempt should occur
	if len(exhausted.AllAttempts) != 1 {
		t.Errorf("Expected 1 attempt (handler stopped), got %d", len(exhausted.AllAttempts))
	}
	if requestCount != 1 {
		t.Errorf("Expected 1 HTTP request (handler stopped), got %d", requestCount)
	}
}

func TestStructured_MissingRequiredField(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	// Create mock server that returns JSON missing a required field
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockProxyResponse(`{"name": "John"}`)) // Missing "age"
	}))
	defer server.Close()

	client, err := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	_, err = Structured[Person](context.Background(), client.LLM, ProxyRequest{
		Model:    NewModelID("test-model"),
		Messages: []llm.ProxyMessage{{Role: "user", Content: "Extract: John"}},
	}, StructuredOptions{MaxRetries: 0})

	if err == nil {
		t.Fatal("Expected error for missing required field")
	}

	var exhausted StructuredExhaustedError
	if !errors.As(err, &exhausted) {
		t.Fatalf("Expected StructuredExhaustedError, got: %T", err)
	}
	if exhausted.FinalError.Kind != StructuredErrorKindValidation {
		t.Errorf("Expected validation error, got: %s", exhausted.FinalError.Kind)
	}
}

func TestStructured_InvalidJSON(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	// Create mock server that returns malformed JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockProxyResponse(`{not valid json}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	_, err = Structured[Person](context.Background(), client.LLM, ProxyRequest{
		Model:    NewModelID("test-model"),
		Messages: []llm.ProxyMessage{{Role: "user", Content: "Extract info"}},
	}, StructuredOptions{MaxRetries: 0})

	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}

	// First-attempt decode errors should return StructuredDecodeError (not StructuredExhaustedError)
	var decodeErr StructuredDecodeError
	if !errors.As(err, &decodeErr) {
		t.Fatalf("Expected StructuredDecodeError for first-attempt decode failure, got: %T", err)
	}
	if decodeErr.Attempt != 1 {
		t.Errorf("Expected attempt=1, got %d", decodeErr.Attempt)
	}
	if decodeErr.RawJSON != `{not valid json}` {
		t.Errorf("Expected RawJSON to contain the malformed content, got: %s", decodeErr.RawJSON)
	}
}

func TestStructured_InvalidJSON_WithRetries_ReturnsExhausted(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	// Create mock server that always returns malformed JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockProxyResponse(`{not valid json}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	_, err = Structured[Person](context.Background(), client.LLM, ProxyRequest{
		Model:    NewModelID("test-model"),
		Messages: []llm.ProxyMessage{{Role: "user", Content: "Extract info"}},
	}, StructuredOptions{MaxRetries: 1}) // With retries, should return StructuredExhaustedError

	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}

	// With retries configured, should return StructuredExhaustedError
	var exhausted StructuredExhaustedError
	if !errors.As(err, &exhausted) {
		t.Fatalf("Expected StructuredExhaustedError when retries configured, got: %T", err)
	}
	if exhausted.FinalError.Kind != StructuredErrorKindDecode {
		t.Errorf("Expected decode error, got: %s", exhausted.FinalError.Kind)
	}
}
