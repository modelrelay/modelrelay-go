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
	customProvider := ParseProviderID("acme-cloud")
	if !customProvider.IsOther() || customProvider.String() != "acme-cloud" {
		t.Fatalf("expected custom provider preserved, got %q", customProvider)
	}

	model := ParseModelID("openai/gpt-4o-mini")
	if model != ModelOpenAIGPT4oMini {
		t.Fatalf("expected gpt-4o-mini got %s", model)
	}
	latest := ParseModelID("openai/gpt-5.1")
	if latest != ModelOpenAIGPT51 {
		t.Fatalf("expected gpt-5.1 got %s", latest)
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

	_, err = NewProxyRequest(ParseModelID("openai/gpt-5.1"), []llm.ProxyMessage{})
	if err == nil {
		t.Fatalf("expected validation error for empty messages")
	}

	req, err := NewProxyRequestBuilder(ModelOpenAIGPT51).
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
