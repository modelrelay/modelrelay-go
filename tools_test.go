package sdk

import (
	"encoding/json"
	"errors"
	"testing"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

func TestToolRegistry(t *testing.T) {
	t.Run("Register and Has", func(t *testing.T) {
		registry := NewToolRegistry()
		registry.Register("my_tool", func(args map[string]any, call llm.ToolCall) (any, error) {
			return "ok", nil
		})

		if !registry.Has("my_tool") {
			t.Error("expected registry to have 'my_tool'")
		}
		if registry.Has("other_tool") {
			t.Error("expected registry not to have 'other_tool'")
		}
	})

	t.Run("RegisteredTools", func(t *testing.T) {
		registry := NewToolRegistry().
			Register("tool_a", func(args map[string]any, call llm.ToolCall) (any, error) { return "", nil }).
			Register("tool_b", func(args map[string]any, call llm.ToolCall) (any, error) { return "", nil })

		tools := registry.RegisteredTools()
		if len(tools) != 2 {
			t.Errorf("expected 2 registered tools, got %d", len(tools))
		}
	})

	t.Run("Unregister", func(t *testing.T) {
		registry := NewToolRegistry().
			Register("my_tool", func(args map[string]any, call llm.ToolCall) (any, error) { return "", nil })

		if !registry.Unregister("my_tool") {
			t.Error("expected Unregister to return true")
		}
		if registry.Has("my_tool") {
			t.Error("expected tool to be unregistered")
		}
		if registry.Unregister("my_tool") {
			t.Error("expected Unregister to return false for already removed tool")
		}
	})

	t.Run("Execute success", func(t *testing.T) {
		registry := NewToolRegistry().
			Register("get_weather", func(args map[string]any, call llm.ToolCall) (any, error) {
				location := args["location"].(string)
				return map[string]any{"temp": 72, "location": location}, nil
			})

		call := llm.ToolCall{
			ID:   "call_123",
			Type: "function",
			Function: &llm.FunctionCall{
				Name:      "get_weather",
				Arguments: `{"location":"NYC"}`,
			},
		}

		result := registry.Execute(call)
		if result.Error != nil {
			t.Errorf("expected no error, got %v", result.Error)
		}
		if result.ToolCallID != "call_123" {
			t.Errorf("expected tool_call_id 'call_123', got '%s'", result.ToolCallID)
		}
		if result.ToolName != "get_weather" {
			t.Errorf("expected tool_name 'get_weather', got '%s'", result.ToolName)
		}
		if result.IsRetryable {
			t.Error("expected IsRetryable to be false for success")
		}
	})

	t.Run("Execute unknown tool", func(t *testing.T) {
		registry := NewToolRegistry().
			Register("known_tool", func(args map[string]any, call llm.ToolCall) (any, error) { return "", nil })

		call := llm.ToolCall{
			ID:       "call_456",
			Type:     "function",
			Function: &llm.FunctionCall{Name: "unknown_tool", Arguments: "{}"},
		}

		result := registry.Execute(call)
		if result.Error == nil {
			t.Error("expected error for unknown tool")
		}
		if _, ok := result.Error.(*UnknownToolError); !ok {
			t.Error("expected UnknownToolError")
		}
		if result.IsRetryable {
			t.Error("expected IsRetryable to be false for unknown tool")
		}
	})

	t.Run("Execute with malformed JSON sets IsRetryable", func(t *testing.T) {
		registry := NewToolRegistry().
			Register("my_tool", func(args map[string]any, call llm.ToolCall) (any, error) { return "", nil })

		call := llm.ToolCall{
			ID:       "call_bad",
			Type:     "function",
			Function: &llm.FunctionCall{Name: "my_tool", Arguments: "{not valid json"},
		}

		result := registry.Execute(call)
		if result.Error == nil {
			t.Error("expected error for malformed JSON")
		}
		if !result.IsRetryable {
			t.Error("expected IsRetryable to be true for JSON parse error")
		}
	})

	t.Run("Execute with handler error not retryable", func(t *testing.T) {
		registry := NewToolRegistry().
			Register("failing_tool", func(args map[string]any, call llm.ToolCall) (any, error) {
				return nil, errors.New("something went wrong")
			})

		call := llm.ToolCall{
			ID:       "call_fail",
			Type:     "function",
			Function: &llm.FunctionCall{Name: "failing_tool", Arguments: "{}"},
		}

		result := registry.Execute(call)
		if result.Error == nil {
			t.Error("expected error")
		}
		if result.IsRetryable {
			t.Error("expected IsRetryable to be false for non-validation errors")
		}
	})

	t.Run("Execute with ToolArgsError is retryable", func(t *testing.T) {
		registry := NewToolRegistry().
			Register("validating_tool", func(args map[string]any, call llm.ToolCall) (any, error) {
				return nil, &ToolArgsError{
					Message:    "missing required field 'value'",
					ToolCallID: call.ID,
					ToolName:   "validating_tool",
				}
			})

		call := llm.ToolCall{
			ID:       "call_val",
			Type:     "function",
			Function: &llm.FunctionCall{Name: "validating_tool", Arguments: "{}"},
		}

		result := registry.Execute(call)
		if result.Error == nil {
			t.Error("expected error")
		}
		if !result.IsRetryable {
			t.Error("expected IsRetryable to be true for ToolArgsError")
		}
	})
}

func TestFormatToolErrorForModel(t *testing.T) {
	t.Run("retryable error includes retry message", func(t *testing.T) {
		result := ToolExecutionResult{
			ToolCallID:  "call_1",
			ToolName:    "my_tool",
			Error:       errors.New("failed to parse arguments"),
			IsRetryable: true,
		}

		formatted := FormatToolErrorForModel(result)
		if formatted == "" {
			t.Error("expected non-empty formatted message")
		}
		if !contains(formatted, "Tool call error for 'my_tool'") {
			t.Error("expected formatted message to contain tool name")
		}
		if !contains(formatted, "failed to parse arguments") {
			t.Error("expected formatted message to contain error")
		}
		if !contains(formatted, "Please correct the arguments") {
			t.Error("expected formatted message to include retry guidance")
		}
	})

	t.Run("non-retryable error excludes retry message", func(t *testing.T) {
		result := ToolExecutionResult{
			ToolCallID:  "call_1",
			ToolName:    "my_tool",
			Error:       errors.New("internal error"),
			IsRetryable: false,
		}

		formatted := FormatToolErrorForModel(result)
		if contains(formatted, "Please correct the arguments") {
			t.Error("expected formatted message not to include retry guidance")
		}
	})
}

func TestHasRetryableErrors(t *testing.T) {
	t.Run("returns true when retryable error present", func(t *testing.T) {
		results := []ToolExecutionResult{
			{ToolCallID: "call_1", ToolName: "tool_a", Result: "ok"},
			{ToolCallID: "call_2", ToolName: "tool_b", Error: errors.New("parse error"), IsRetryable: true},
		}

		if !HasRetryableErrors(results) {
			t.Error("expected HasRetryableErrors to return true")
		}
	})

	t.Run("returns false when no errors", func(t *testing.T) {
		results := []ToolExecutionResult{
			{ToolCallID: "call_1", ToolName: "tool_a", Result: "ok"},
		}

		if HasRetryableErrors(results) {
			t.Error("expected HasRetryableErrors to return false")
		}
	})

	t.Run("returns false when error but not retryable", func(t *testing.T) {
		results := []ToolExecutionResult{
			{ToolCallID: "call_1", ToolName: "tool_a", Error: errors.New("internal error"), IsRetryable: false},
		}

		if HasRetryableErrors(results) {
			t.Error("expected HasRetryableErrors to return false")
		}
	})
}

func TestGetRetryableErrors(t *testing.T) {
	results := []ToolExecutionResult{
		{ToolCallID: "call_1", ToolName: "tool_a", Result: "ok"},
		{ToolCallID: "call_2", ToolName: "tool_b", Error: errors.New("parse error"), IsRetryable: true},
		{ToolCallID: "call_3", ToolName: "tool_c", Error: errors.New("validation"), IsRetryable: true},
		{ToolCallID: "call_4", ToolName: "tool_d", Error: errors.New("internal"), IsRetryable: false},
	}

	retryable := GetRetryableErrors(results)
	if len(retryable) != 2 {
		t.Errorf("expected 2 retryable errors, got %d", len(retryable))
	}
	if retryable[0].ToolCallID != "call_2" {
		t.Errorf("expected first retryable to be call_2, got %s", retryable[0].ToolCallID)
	}
	if retryable[1].ToolCallID != "call_3" {
		t.Errorf("expected second retryable to be call_3, got %s", retryable[1].ToolCallID)
	}
}

func TestCreateRetryMessages(t *testing.T) {
	results := []ToolExecutionResult{
		{ToolCallID: "call_1", ToolName: "tool_a", Result: "ok"},
		{ToolCallID: "call_2", ToolName: "tool_b", Error: errors.New("parse error"), IsRetryable: true},
	}

	messages := CreateRetryMessages(results)
	if len(messages) != 1 {
		t.Errorf("expected 1 retry message, got %d", len(messages))
	}
	if messages[0].Role != "tool" {
		t.Errorf("expected role 'tool', got '%s'", messages[0].Role)
	}
	if messages[0].ToolCallID != "call_2" {
		t.Errorf("expected tool_call_id 'call_2', got '%s'", messages[0].ToolCallID)
	}
	text := ""
	if len(messages[0].Content) > 0 {
		text = messages[0].Content[0].Text
	}
	if !contains(text, "Tool call error") {
		t.Error("expected message content to contain 'Tool call error'")
	}
}

func TestExecuteWithRetry(t *testing.T) {
	t.Run("no retry needed when successful", func(t *testing.T) {
		registry := NewToolRegistry().
			Register("my_tool", func(args map[string]any, call llm.ToolCall) (any, error) {
				return "success", nil
			})

		calls := []llm.ToolCall{{
			ID:       "call_1",
			Type:     "function",
			Function: &llm.FunctionCall{Name: "my_tool", Arguments: "{}"},
		}}

		retryCount := 0
		results, err := ExecuteWithRetry(registry, calls, RetryOptions{
			MaxRetries: 2,
			OnRetry: func(messages []llm.InputItem, attempt int) ([]llm.ToolCall, error) {
				retryCount++
				return nil, nil
			},
		})

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if retryCount != 0 {
			t.Errorf("expected no retries, got %d", retryCount)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d", len(results))
		}
		if results[0].Error != nil {
			t.Errorf("expected success, got error: %v", results[0].Error)
		}
	})

	t.Run("retries on parse error and succeeds", func(t *testing.T) {
		registry := NewToolRegistry().
			Register("my_tool", func(args map[string]any, call llm.ToolCall) (any, error) {
				return "success", nil
			})

		// Start with invalid JSON
		calls := []llm.ToolCall{{
			ID:       "call_1",
			Type:     "function",
			Function: &llm.FunctionCall{Name: "my_tool", Arguments: "{invalid"},
		}}

		retryCount := 0
		results, err := ExecuteWithRetry(registry, calls, RetryOptions{
			MaxRetries: 2,
			OnRetry: func(messages []llm.InputItem, attempt int) ([]llm.ToolCall, error) {
				retryCount++
				// Return corrected tool call
				return []llm.ToolCall{{
					ID:       "call_1_retry",
					Type:     "function",
					Function: &llm.FunctionCall{Name: "my_tool", Arguments: "{}"},
				}}, nil
			},
		})

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if retryCount != 1 {
			t.Errorf("expected 1 retry, got %d", retryCount)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 result, got %d", len(results))
		}
		if results[0].Error != nil {
			t.Errorf("expected success after retry, got error: %v", results[0].Error)
		}
	})

	t.Run("respects max retries", func(t *testing.T) {
		registry := NewToolRegistry().
			Register("my_tool", func(args map[string]any, call llm.ToolCall) (any, error) {
				return "success", nil
			})

		calls := []llm.ToolCall{{
			ID:       "call_1",
			Type:     "function",
			Function: &llm.FunctionCall{Name: "my_tool", Arguments: "{invalid"},
		}}

		retryCount := 0
		results, err := ExecuteWithRetry(registry, calls, RetryOptions{
			MaxRetries: 2,
			OnRetry: func(messages []llm.InputItem, attempt int) ([]llm.ToolCall, error) {
				retryCount++
				// Keep returning invalid JSON
				return []llm.ToolCall{{
					ID:       "call_retry",
					Type:     "function",
					Function: &llm.FunctionCall{Name: "my_tool", Arguments: "{still invalid"},
				}}, nil
			},
		})

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if retryCount != 2 {
			t.Errorf("expected 2 retries (max), got %d", retryCount)
		}
		if results[0].Error == nil {
			t.Error("expected error after max retries exhausted")
		}
	})

	t.Run("preserves successful results across retries", func(t *testing.T) {
		// Register two tools: one that always succeeds, one that fails with invalid JSON
		registry := NewToolRegistry().
			Register("success_tool", func(args map[string]any, call llm.ToolCall) (any, error) {
				return "success_result", nil
			}).
			Register("failing_tool", func(args map[string]any, call llm.ToolCall) (any, error) {
				return "fixed_result", nil
			})

		// Initial calls: one succeeds, one has invalid JSON
		calls := []llm.ToolCall{
			{
				ID:       "call_success",
				Type:     "function",
				Function: &llm.FunctionCall{Name: "success_tool", Arguments: "{}"},
			},
			{
				ID:       "call_fail",
				Type:     "function",
				Function: &llm.FunctionCall{Name: "failing_tool", Arguments: "{invalid"},
			},
		}

		retryCount := 0
		results, err := ExecuteWithRetry(registry, calls, RetryOptions{
			MaxRetries: 2,
			OnRetry: func(messages []llm.InputItem, attempt int) ([]llm.ToolCall, error) {
				retryCount++
				// Return corrected tool call only for the failing one
				return []llm.ToolCall{{
					ID:       "call_fail_retry",
					Type:     "function",
					Function: &llm.FunctionCall{Name: "failing_tool", Arguments: "{}"},
				}}, nil
			},
		})

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if retryCount != 1 {
			t.Errorf("expected 1 retry, got %d", retryCount)
		}

		// Should have 2 results: the original success and the retried success
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}

		// Find the original successful result
		var foundOriginalSuccess, foundRetrySuccess bool
		for _, r := range results {
			if r.ToolCallID == "call_success" && r.Error == nil {
				foundOriginalSuccess = true
				if r.Result != "success_result" {
					t.Errorf("expected success_result, got %v", r.Result)
				}
			}
			if r.ToolCallID == "call_fail_retry" && r.Error == nil {
				foundRetrySuccess = true
				if r.Result != "fixed_result" {
					t.Errorf("expected fixed_result, got %v", r.Result)
				}
			}
		}

		if !foundOriginalSuccess {
			t.Error("original successful result was lost during retry")
		}
		if !foundRetrySuccess {
			t.Error("retried result not found")
		}
	})
}

func TestParseToolArgs(t *testing.T) {
	type WeatherArgs struct {
		Location string `json:"location"`
		Unit     string `json:"unit,omitempty"`
	}

	t.Run("success", func(t *testing.T) {
		call := llm.ToolCall{
			ID:       "call_1",
			Type:     "function",
			Function: &llm.FunctionCall{Name: "get_weather", Arguments: `{"location":"NYC","unit":"celsius"}`},
		}

		var args WeatherArgs
		err := ParseToolArgs(call, &args)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if args.Location != "NYC" {
			t.Errorf("expected location NYC, got %s", args.Location)
		}
		if args.Unit != "celsius" {
			t.Errorf("expected unit celsius, got %s", args.Unit)
		}
	})

	t.Run("invalid JSON returns ToolArgsError", func(t *testing.T) {
		call := llm.ToolCall{
			ID:       "call_2",
			Type:     "function",
			Function: &llm.FunctionCall{Name: "get_weather", Arguments: "{not valid json"},
		}

		var args WeatherArgs
		err := ParseToolArgs(call, &args)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
		argsErr, ok := err.(*ToolArgsError)
		if !ok {
			t.Error("expected ToolArgsError")
		}
		if argsErr.ToolCallID != "call_2" {
			t.Errorf("expected tool_call_id 'call_2', got '%s'", argsErr.ToolCallID)
		}
		if argsErr.ToolName != "get_weather" {
			t.Errorf("expected tool_name 'get_weather', got '%s'", argsErr.ToolName)
		}
	})
}

func TestParseToolArgsMap(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		call := llm.ToolCall{
			ID:       "call_1",
			Type:     "function",
			Function: &llm.FunctionCall{Name: "my_tool", Arguments: `{"key":"value","num":42}`},
		}

		args, err := ParseToolArgsMap(call)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if args["key"] != "value" {
			t.Errorf("expected key='value', got %v", args["key"])
		}
		if args["num"].(float64) != 42 {
			t.Errorf("expected num=42, got %v", args["num"])
		}
	})

	t.Run("empty arguments returns empty map", func(t *testing.T) {
		call := llm.ToolCall{
			ID:       "call_2",
			Type:     "function",
			Function: &llm.FunctionCall{Name: "my_tool", Arguments: ""},
		}

		args, err := ParseToolArgsMap(call)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if len(args) != 0 {
			t.Errorf("expected empty map, got %v", args)
		}
	})
}

// ValidatedArgs implements Validator interface for testing
type ValidatedArgs struct {
	Value int `json:"value"`
}

func (a *ValidatedArgs) Validate() error {
	if a.Value < 0 {
		return errors.New("value must be non-negative")
	}
	if a.Value > 100 {
		return errors.New("value must be at most 100")
	}
	return nil
}

func TestParseAndValidateToolArgs(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		call := llm.ToolCall{
			ID:       "call_1",
			Type:     "function",
			Function: &llm.FunctionCall{Name: "set_value", Arguments: `{"value":50}`},
		}

		var args ValidatedArgs
		err := ParseAndValidateToolArgs(call, &args)
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if args.Value != 50 {
			t.Errorf("expected value 50, got %d", args.Value)
		}
	})

	t.Run("validation failure returns ToolArgsError", func(t *testing.T) {
		call := llm.ToolCall{
			ID:       "call_2",
			Type:     "function",
			Function: &llm.FunctionCall{Name: "set_value", Arguments: `{"value":-5}`},
		}

		var args ValidatedArgs
		err := ParseAndValidateToolArgs(call, &args)
		if err == nil {
			t.Error("expected error for invalid value")
		}
		argsErr, ok := err.(*ToolArgsError)
		if !ok {
			t.Error("expected ToolArgsError")
		}
		if !contains(argsErr.Message, "value must be non-negative") {
			t.Errorf("expected validation message, got '%s'", argsErr.Message)
		}
	})
}

// helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================================
// Message Factory Function Tests
// ============================================================================

func TestNewUserMessage(t *testing.T) {
	msg := NewUserMessage("Hello, world!")
	if msg.Role != "user" {
		t.Errorf("expected role 'user', got '%s'", msg.Role)
	}
	if len(msg.Content) == 0 || msg.Content[0].Text != "Hello, world!" {
		t.Errorf("expected content 'Hello, world!', got %+v", msg.Content)
	}
}

func TestNewAssistantMessage(t *testing.T) {
	msg := NewAssistantMessage("I can help with that.")
	if msg.Role != "assistant" {
		t.Errorf("expected role 'assistant', got '%s'", msg.Role)
	}
	if len(msg.Content) == 0 || msg.Content[0].Text != "I can help with that." {
		t.Errorf("expected content 'I can help with that.', got %+v", msg.Content)
	}
}

func TestNewSystemMessage(t *testing.T) {
	msg := NewSystemMessage("You are a helpful assistant.")
	if msg.Role != "system" {
		t.Errorf("expected role 'system', got '%s'", msg.Role)
	}
	if len(msg.Content) == 0 || msg.Content[0].Text != "You are a helpful assistant." {
		t.Errorf("expected content 'You are a helpful assistant.', got %+v", msg.Content)
	}
}

// ============================================================================
// ToolCall Factory Function Tests
// ============================================================================

func TestNewToolCall(t *testing.T) {
	tc := NewToolCall("call_123", "get_weather", `{"location":"NYC"}`)
	if tc.ID != "call_123" {
		t.Errorf("expected ID 'call_123', got '%s'", tc.ID)
	}
	if tc.Type != llm.ToolTypeFunction {
		t.Errorf("expected type '%s', got '%s'", llm.ToolTypeFunction, tc.Type)
	}
	if tc.Function == nil {
		t.Fatal("expected function to be set")
	}
	if tc.Function.Name != "get_weather" {
		t.Errorf("expected function name 'get_weather', got '%s'", tc.Function.Name)
	}
	if tc.Function.Arguments != `{"location":"NYC"}` {
		t.Errorf("expected arguments, got '%s'", tc.Function.Arguments)
	}
}

func TestNewFunctionCall(t *testing.T) {
	fc := NewFunctionCall("get_weather", `{"location":"NYC"}`)
	if fc.Name != "get_weather" {
		t.Errorf("expected name 'get_weather', got '%s'", fc.Name)
	}
	if fc.Arguments != `{"location":"NYC"}` {
		t.Errorf("expected arguments, got '%s'", fc.Arguments)
	}
}

// ============================================================================
// Usage Factory Function Tests
// ============================================================================

func TestNewUsage(t *testing.T) {
	t.Run("with explicit total", func(t *testing.T) {
		u := NewUsage(100, 50, 150)
		if u.InputTokens != 100 {
			t.Errorf("expected input 100, got %d", u.InputTokens)
		}
		if u.OutputTokens != 50 {
			t.Errorf("expected output 50, got %d", u.OutputTokens)
		}
		if u.TotalTokens != 150 {
			t.Errorf("expected total 150, got %d", u.TotalTokens)
		}
	})

	t.Run("auto-calculates total when zero", func(t *testing.T) {
		u := NewUsage(100, 50, 0)
		if u.TotalTokens != 150 {
			t.Errorf("expected auto-calculated total 150, got %d", u.TotalTokens)
		}
	})
}

// ============================================================================
// Tool Result Message Tests
// ============================================================================

// Ensure tool result input items have proper JSON marshaling for test comparison
func TestToolResultMessage(t *testing.T) {
	msg, err := ToolResultMessage("call_123", "sunny")
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if msg.Role != "tool" {
		t.Errorf("expected role 'tool', got '%s'", msg.Role)
	}
	if len(msg.Content) == 0 || msg.Content[0].Text != "sunny" {
		t.Errorf("expected content 'sunny', got %+v", msg.Content)
	}
	if msg.ToolCallID != "call_123" {
		t.Errorf("expected tool_call_id 'call_123', got '%s'", msg.ToolCallID)
	}
}

func TestToolResultMessageWithJSON(t *testing.T) {
	data := map[string]any{"temp": 72, "unit": "fahrenheit"}
	msg, err := ToolResultMessage("call_456", data)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if msg.Role != "tool" {
		t.Errorf("expected role 'tool', got '%s'", msg.Role)
	}

	// Verify JSON content
	var parsed map[string]any
	if len(msg.Content) == 0 {
		t.Fatalf("expected content")
	}
	if err := json.Unmarshal([]byte(msg.Content[0].Text), &parsed); err != nil {
		t.Errorf("expected valid JSON content, got error: %v", err)
	}
}

// ============================================================================
// Schema Inference Tests
// ============================================================================

func TestTypeToJSONSchema(t *testing.T) {
	t.Run("basic struct with descriptions", func(t *testing.T) {
		type GetWeatherParams struct {
			Location string `json:"location" description:"City name"`
			Unit     string `json:"unit,omitempty" enum:"celsius,fahrenheit" default:"celsius"`
		}

		schema := TypeToJSONSchema(GetWeatherParams{}, nil)

		if schema.Type != "object" {
			t.Errorf("expected type 'object', got '%s'", schema.Type)
		}

		if schema.Properties == nil {
			t.Fatal("expected properties to be set")
		}

		location := schema.Properties["location"]
		if location == nil {
			t.Fatal("expected 'location' property")
		}
		if location.Type != "string" {
			t.Errorf("expected location type 'string', got '%s'", location.Type)
		}
		if location.Description != "City name" {
			t.Errorf("expected location description 'City name', got '%s'", location.Description)
		}

		unit := schema.Properties["unit"]
		if unit == nil {
			t.Fatal("expected 'unit' property")
		}
		if len(unit.Enum) != 2 {
			t.Errorf("expected 2 enum values, got %d", len(unit.Enum))
		}
		if unit.Default != "celsius" {
			t.Errorf("expected default 'celsius', got '%v'", unit.Default)
		}

		// location should be required, unit should not (omitempty)
		if len(schema.Required) != 1 || schema.Required[0] != "location" {
			t.Errorf("expected required=['location'], got %v", schema.Required)
		}
	})

	t.Run("handles nested structs", func(t *testing.T) {
		type Address struct {
			Street string `json:"street"`
			City   string `json:"city"`
		}
		type User struct {
			Name    string  `json:"name"`
			Address Address `json:"address"`
		}

		schema := TypeToJSONSchema(User{}, nil)

		if schema.Properties == nil {
			t.Fatal("expected properties to be set")
		}

		address := schema.Properties["address"]
		if address == nil {
			t.Fatal("expected 'address' property")
		}
		if address.Type != "object" {
			t.Errorf("expected address type 'object', got '%s'", address.Type)
		}
		if address.Properties == nil {
			t.Fatal("expected address to have properties")
		}
		if address.Properties["street"] == nil {
			t.Error("expected 'street' property in address")
		}
	})

	t.Run("handles arrays", func(t *testing.T) {
		type TaggedItem struct {
			Tags []string `json:"tags"`
		}

		schema := TypeToJSONSchema(TaggedItem{}, nil)

		tags := schema.Properties["tags"]
		if tags == nil {
			t.Fatal("expected 'tags' property")
		}
		if tags.Type != "array" {
			t.Errorf("expected tags type 'array', got '%s'", tags.Type)
		}
		if tags.Items == nil || tags.Items.Type != "string" {
			t.Error("expected tags.items.type to be 'string'")
		}
	})

	t.Run("handles pointer fields as optional", func(t *testing.T) {
		type OptionalFields struct {
			Required string  `json:"required"`
			Optional *string `json:"optional"`
		}

		schema := TypeToJSONSchema(OptionalFields{}, nil)

		if len(schema.Required) != 1 || schema.Required[0] != "required" {
			t.Errorf("expected only 'required' in required list, got %v", schema.Required)
		}
	})

	t.Run("handles numeric constraints", func(t *testing.T) {
		type NumericParams struct {
			Count int     `json:"count" minimum:"0" maximum:"100"`
			Price float64 `json:"price" minimum:"0.01"`
		}

		schema := TypeToJSONSchema(NumericParams{}, nil)

		count := schema.Properties["count"]
		if count == nil {
			t.Fatal("expected 'count' property")
		}
		if count.Type != "integer" {
			t.Errorf("expected count type 'integer', got '%s'", count.Type)
		}
		if count.Minimum == nil || *count.Minimum != 0 {
			t.Errorf("expected minimum 0, got %v", count.Minimum)
		}
		if count.Maximum == nil || *count.Maximum != 100 {
			t.Errorf("expected maximum 100, got %v", count.Maximum)
		}
	})

	t.Run("handles string constraints", func(t *testing.T) {
		type StringParams struct {
			Query string `json:"query" minLength:"1" maxLength:"1000"`
			Email string `json:"email" format:"email"`
			ID    string `json:"id" pattern:"^[a-z]+$"`
		}

		schema := TypeToJSONSchema(StringParams{}, nil)

		query := schema.Properties["query"]
		if query.MinLength == nil || *query.MinLength != 1 {
			t.Errorf("expected minLength 1, got %v", query.MinLength)
		}
		if query.MaxLength == nil || *query.MaxLength != 1000 {
			t.Errorf("expected maxLength 1000, got %v", query.MaxLength)
		}

		email := schema.Properties["email"]
		if email.Format != "email" {
			t.Errorf("expected format 'email', got '%s'", email.Format)
		}

		id := schema.Properties["id"]
		if id.Pattern != "^[a-z]+$" {
			t.Errorf("expected pattern '^[a-z]+$', got '%s'", id.Pattern)
		}
	})

	t.Run("handles maps", func(t *testing.T) {
		type MapParams struct {
			Metadata map[string]string `json:"metadata"`
		}

		schema := TypeToJSONSchema(MapParams{}, nil)

		metadata := schema.Properties["metadata"]
		if metadata == nil {
			t.Fatal("expected 'metadata' property")
		}
		if metadata.Type != "object" {
			t.Errorf("expected metadata type 'object', got '%s'", metadata.Type)
		}
		if metadata.AdditionalProperties == nil || metadata.AdditionalProperties.Type != "string" {
			t.Error("expected additionalProperties.type to be 'string'")
		}
	})

	t.Run("includes $schema when requested", func(t *testing.T) {
		type SimpleStruct struct {
			Name string `json:"name"`
		}

		schema := TypeToJSONSchema(SimpleStruct{}, &SchemaOptions{
			IncludeSchema: true,
			SchemaVersion: "draft-07",
		})

		if schema.Schema != "http://json-schema.org/draft-07/schema#" {
			t.Errorf("expected draft-07 schema URL, got '%s'", schema.Schema)
		}
	})

	t.Run("skips json:- fields", func(t *testing.T) {
		type WithIgnored struct {
			Visible string `json:"visible"`
			Ignored string `json:"-"`
		}

		schema := TypeToJSONSchema(WithIgnored{}, nil)

		if schema.Properties["Ignored"] != nil {
			t.Error("expected ignored field to be skipped")
		}
		if schema.Properties["visible"] == nil {
			t.Error("expected visible field to be present")
		}
	})

	t.Run("parses enum values according to field type", func(t *testing.T) {
		type TypedEnums struct {
			IntField    int     `json:"int_field" enum:"1,2,3"`
			FloatField  float64 `json:"float_field" enum:"1.5,2.5,3.5"`
			BoolField   bool    `json:"bool_field" enum:"true,false"`
			StringField string  `json:"string_field" enum:"a,b,c"`
		}

		schema := TypeToJSONSchema(TypedEnums{}, nil)

		// Integer enum should have integer values
		intField := schema.Properties["int_field"]
		if intField == nil {
			t.Fatal("expected 'int_field' property")
		}
		if len(intField.Enum) != 3 {
			t.Errorf("expected 3 enum values, got %d", len(intField.Enum))
		}
		if intField.Enum[0] != 1 {
			t.Errorf("expected int enum value 1, got %v (type %T)", intField.Enum[0], intField.Enum[0])
		}

		// Float enum should have float values
		floatField := schema.Properties["float_field"]
		if floatField == nil {
			t.Fatal("expected 'float_field' property")
		}
		if floatField.Enum[0] != 1.5 {
			t.Errorf("expected float enum value 1.5, got %v (type %T)", floatField.Enum[0], floatField.Enum[0])
		}

		// Bool enum should have bool values
		boolField := schema.Properties["bool_field"]
		if boolField == nil {
			t.Fatal("expected 'bool_field' property")
		}
		if boolField.Enum[0] != true {
			t.Errorf("expected bool enum value true, got %v (type %T)", boolField.Enum[0], boolField.Enum[0])
		}
		if boolField.Enum[1] != false {
			t.Errorf("expected bool enum value false, got %v (type %T)", boolField.Enum[1], boolField.Enum[1])
		}

		// String enum should have string values
		stringField := schema.Properties["string_field"]
		if stringField == nil {
			t.Fatal("expected 'string_field' property")
		}
		if stringField.Enum[0] != "a" {
			t.Errorf("expected string enum value 'a', got %v (type %T)", stringField.Enum[0], stringField.Enum[0])
		}
	})

	t.Run("parses default values according to field type", func(t *testing.T) {
		type TypedDefaults struct {
			IntField    int     `json:"int_field" default:"42"`
			FloatField  float64 `json:"float_field" default:"3.14"`
			BoolField   bool    `json:"bool_field" default:"true"`
			StringField string  `json:"string_field" default:"hello"`
		}

		schema := TypeToJSONSchema(TypedDefaults{}, nil)

		intField := schema.Properties["int_field"]
		if intField.Default != 42 {
			t.Errorf("expected int default 42, got %v (type %T)", intField.Default, intField.Default)
		}

		floatField := schema.Properties["float_field"]
		if floatField.Default != 3.14 {
			t.Errorf("expected float default 3.14, got %v (type %T)", floatField.Default, floatField.Default)
		}

		boolField := schema.Properties["bool_field"]
		if boolField.Default != true {
			t.Errorf("expected bool default true, got %v (type %T)", boolField.Default, boolField.Default)
		}

		stringField := schema.Properties["string_field"]
		if stringField.Default != "hello" {
			t.Errorf("expected string default 'hello', got %v (type %T)", stringField.Default, stringField.Default)
		}
	})

	t.Run("handles pointer fields with typed enum/default", func(t *testing.T) {
		type PointerFields struct {
			OptionalInt *int `json:"optional_int,omitempty" enum:"1,2,3" default:"1"`
		}

		schema := TypeToJSONSchema(PointerFields{}, nil)

		optInt := schema.Properties["optional_int"]
		if optInt == nil {
			t.Fatal("expected 'optional_int' property")
		}
		// Enum values should be integers even for pointer fields
		if optInt.Enum[0] != 1 {
			t.Errorf("expected int enum value 1, got %v (type %T)", optInt.Enum[0], optInt.Enum[0])
		}
		if optInt.Default != 1 {
			t.Errorf("expected int default 1, got %v (type %T)", optInt.Default, optInt.Default)
		}
	})
}

func TestFunctionToolFromType(t *testing.T) {
	t.Run("creates tool from struct type", func(t *testing.T) {
		type GetWeatherParams struct {
			Location string `json:"location" description:"City name"`
			Unit     string `json:"unit,omitempty" enum:"celsius,fahrenheit" default:"celsius"`
		}

		tool, err := FunctionToolFromType[GetWeatherParams]("get_weather", "Get weather for a location")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if tool.Type != llm.ToolTypeFunction {
			t.Errorf("expected tool type '%s', got '%s'", llm.ToolTypeFunction, tool.Type)
		}
		if tool.Function == nil {
			t.Fatal("expected function to be set")
		}
		if tool.Function.Name != "get_weather" {
			t.Errorf("expected name 'get_weather', got '%s'", tool.Function.Name)
		}
		if tool.Function.Description != "Get weather for a location" {
			t.Errorf("expected description 'Get weather for a location', got '%s'", tool.Function.Description)
		}

		// Verify the parameters contain valid JSON schema
		if tool.Function.Parameters == nil {
			t.Fatal("expected parameters to be set")
		}

		var schema JSONSchema
		if err := json.Unmarshal(tool.Function.Parameters, &schema); err != nil {
			t.Fatalf("expected valid JSON schema, got error: %v", err)
		}
		if schema.Type != "object" {
			t.Errorf("expected schema type 'object', got '%s'", schema.Type)
		}
	})

	t.Run("MustFunctionToolFromType works", func(t *testing.T) {
		type SimpleParams struct {
			Query string `json:"query"`
		}

		// Should not panic
		tool := MustFunctionToolFromType[SimpleParams]("search", "Search for something")
		if tool.Function.Name != "search" {
			t.Errorf("expected name 'search', got '%s'", tool.Function.Name)
		}
	})
}

func TestTypedTool(t *testing.T) {
	type ReadFileArgs struct {
		Path string `json:"path"`
	}

	t.Run("parses typed call", func(t *testing.T) {
		tool, err := NewTypedTool[ReadFileArgs]("read_file", "Read a file")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		call := llm.ToolCall{
			ID:   "call_1",
			Type: llm.ToolTypeFunction,
			Function: &llm.FunctionCall{
				Name:      "read_file",
				Arguments: `{"path":"./config.json"}`,
			},
		}

		typedCall, err := tool.ParseCall(call)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if typedCall.Args.Path != "./config.json" {
			t.Fatalf("expected path ./config.json, got %q", typedCall.Args.Path)
		}
	})

	t.Run("rejects mismatched tool name", func(t *testing.T) {
		tool := MustTypedTool[ReadFileArgs]("read_file", "Read a file")
		call := llm.ToolCall{
			ID:   "call_2",
			Type: llm.ToolTypeFunction,
			Function: &llm.FunctionCall{
				Name:      "other_tool",
				Arguments: `{"path":"./config.json"}`,
			},
		}

		_, err := tool.ParseCall(call)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if _, ok := err.(*ToolArgsError); !ok {
			t.Fatalf("expected ToolArgsError, got %T", err)
		}
	})

	t.Run("rejects missing function", func(t *testing.T) {
		tool := MustTypedTool[ReadFileArgs]("read_file", "Read a file")
		call := llm.ToolCall{
			ID:   "call_3",
			Type: llm.ToolTypeFunction,
		}

		_, err := tool.ParseCall(call)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if _, ok := err.(*ToolArgsError); !ok {
			t.Fatalf("expected ToolArgsError, got %T", err)
		}
	})
}
