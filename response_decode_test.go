package sdk_test

import (
	"encoding/json"
	"testing"

	"github.com/modelrelay/modelrelay/sdk/go/llm"
)

func TestResponseDecode_WarnsOnMissingRole(t *testing.T) {
	data := []byte(`{
		"id": "resp_1",
		"output": [
			{"type": "message", "content": [{"type": "text", "text": "hi"}]}
		],
		"model": "demo",
		"usage": {"input_tokens": 1, "output_tokens": 2, "total_tokens": 3}
	}`)

	var resp llm.Response
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.DecodingWarnings) != 1 {
		t.Fatalf("expected 1 warning, got %v", resp.DecodingWarnings)
	}
	if len(resp.Output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(resp.Output))
	}
}

func TestResponseDecode_AllowsMissingContentWithoutWarning(t *testing.T) {
	data := []byte(`{
		"id": "resp_2",
		"output": [
			{
				"type": "message",
				"role": "assistant",
				"tool_calls": [
					{"id": "call_1", "type": "function", "function": {"name": "do", "arguments": "{}"}}
				]
			}
		],
		"model": "demo",
		"usage": {"input_tokens": 1, "output_tokens": 2, "total_tokens": 3}
	}`)

	var resp llm.Response
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.DecodingWarnings) != 0 {
		t.Fatalf("expected no warnings, got %v", resp.DecodingWarnings)
	}
	if len(resp.Output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(resp.Output))
	}
}
