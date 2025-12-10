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
	model := NewModelID(" gpt-4o-mini ")
	if model.String() != "gpt-4o-mini" {
		t.Fatalf("expected gpt-4o-mini got %s", model)
	}
	customModel := NewModelID("my/model")
	if customModel.String() != "my/model" {
		t.Fatalf("expected custom model preserved, got %q", customModel)
	}
}

func TestProxyRequestBuilderValidation(t *testing.T) {
	_, err := NewProxyRequestBuilder("").Build()
	if err == nil {
		t.Fatalf("expected missing model validation error")
	}

	_, err = NewProxyRequest(NewModelID("gpt-5.1"), []llm.ProxyMessage{})
	if err == nil {
		t.Fatalf("expected validation error for empty messages")
	}

	req, err := NewProxyRequestBuilder(NewModelID("gpt-5.1")).
		User("hello").
		MetadataEntry("trace_id", "abc").
		ResponseFormat(llm.ResponseFormat{
			Type: llm.ResponseFormatTypeJSONSchema,
			JSONSchema: &llm.JSONSchemaFormat{
				Name:   "test",
				Schema: []byte(`{"type":"object"}`),
			},
		}).
		Build()
	if err != nil {
		t.Fatalf("unexpected builder error: %v", err)
	}
	if len(req.Messages) != 1 || req.Metadata["trace_id"] != "abc" || req.ResponseFormat == nil || req.ResponseFormat.Type != llm.ResponseFormatTypeJSONSchema {
		t.Fatalf("builder failed: %+v", req)
	}
}
