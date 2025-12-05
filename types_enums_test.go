package sdk

import (
	llm "github.com/modelrelay/modelrelay/providers"
	"testing"
)

func TestStopReasonParsingAndOther(t *testing.T) {
	var reason StopReason
	if err := reason.UnmarshalJSON([]byte(`"end_turn"`)); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if reason != StopReasonEndTurn {
		t.Fatalf("expected end_turn got %s", reason)
	}
	reason = ParseStopReason("vendor_custom")
	if !reason.IsOther() || string(reason) != "vendor_custom" {
		t.Fatalf("expected other preserved, got %q", reason)
	}
}

func TestProviderAndModelParsing(t *testing.T) {
	provider := ParseProviderID("openai")
	if provider != ProviderOpenAI {
		t.Fatalf("expected openai provider, got %s", provider)
	}

	model := ParseModelID("gpt-4o-mini")
	if model != ModelGPT4oMini {
		t.Fatalf("expected gpt-4o-mini got %s", model)
	}
	latest := ParseModelID("gpt-5.1")
	if latest != ModelGPT51 {
		t.Fatalf("expected gpt-5.1 got %s", latest)
	}
	// Anthropic: only provider-agnostic ids are recognized.
	opus := ParseModelID("claude-opus-4-5")
	if opus != ModelClaudeOpus4_5 {
		t.Fatalf("expected claude-opus-4-5 got %s", opus)
	}

	customModel := ParseModelID("my/model")
	if !customModel.IsOther() || customModel.String() != "my/model" {
		t.Fatalf("expected custom model preserved, got %q", customModel)
	}
}

func TestProxyRequestBuilderValidation(t *testing.T) {
	_, err := NewProxyRequestBuilder("").Build()
	if err == nil {
		t.Fatalf("expected missing model validation error")
	}

	_, err = NewProxyRequest(ModelGPT51, []llm.ProxyMessage{})
	if err == nil {
		t.Fatalf("expected validation error for empty messages")
	}

	req, err := NewProxyRequestBuilder(ModelGPT51).
		User("hello").
		MetadataEntry("trace_id", "abc").
		ResponseFormat(llm.ResponseFormat{Type: llm.ResponseFormatTypeJSONObject}).
		Build()
	if err != nil {
		t.Fatalf("unexpected builder error: %v", err)
	}
	if len(req.Messages) != 1 || req.Metadata["trace_id"] != "abc" || req.ResponseFormat == nil || req.ResponseFormat.Type != llm.ResponseFormatTypeJSONObject {
		t.Fatalf("builder failed: %+v", req)
	}
}
