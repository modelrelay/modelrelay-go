package sdk

import (
	"testing"

	"github.com/google/uuid"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
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

func TestResponseBuilderValidation(t *testing.T) {
	_, _, err := (&ResponsesClient{}).New().Build()
	if err == nil {
		t.Fatalf("expected validation error")
	}

	_, _, err = (&ResponsesClient{}).New().Model(NewModelID("gpt-5.1")).Build()
	if err == nil {
		t.Fatalf("expected validation error for empty input")
	}

	req, _, err := (&ResponsesClient{}).New().
		Model(NewModelID("gpt-5.1")).
		User("hello").
		OutputFormat(llm.OutputFormat{
			Type: llm.OutputFormatTypeJSONSchema,
			JSONSchema: &llm.JSONSchemaFormat{
				Name:   "test",
				Schema: []byte(`{"type":"object"}`),
			},
		}).
		Build()
	if err != nil {
		t.Fatalf("unexpected builder error: %v", err)
	}
	if len(req.input) != 1 || req.outputFormat == nil || req.outputFormat.Type != llm.OutputFormatTypeJSONSchema {
		t.Fatalf("builder failed: %+v", req)
	}
}

func TestResponseBuilderRejectsSessionAndState(t *testing.T) {
	sessionID := uuid.New()
	stateID := uuid.New()
	_, _, err := (&ResponsesClient{}).New().
		Model(NewModelID("gpt-5.1")).
		User("hello").
		SessionID(sessionID).
		StateID(stateID).
		Build()
	if err == nil {
		t.Fatalf("expected validation error for session_id and state_id")
	}
}
