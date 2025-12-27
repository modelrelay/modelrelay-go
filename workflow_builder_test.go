package sdk

import (
	"errors"
	"testing"
)

func TestValidateBindingTarget_MessageIndexOutOfBounds(t *testing.T) {
	// Request with only 2 messages (indices 0 and 1)
	req, _, err := (ResponseBuilder{}).
		Model(NewModelID("claude-sonnet-4-20250514")).
		System("System prompt").
		User("User prompt").
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Binding targets index 3 - doesn't exist
	bindings := []LLMResponsesBindingV0{{
		From:    "upstream",
		Pointer: LLMTextOutput,
		To:      LLMInput().Message(3).Text(), // /input/3/content/0/text
	}}

	_, err = WorkflowV0().
		LLMResponsesNodeWithBindings("target", req, nil, bindings)

	if err == nil {
		t.Fatal("expected error for out-of-bounds message index")
	}

	var bindingErr BindingTargetError
	if !errors.As(err, &bindingErr) {
		t.Fatalf("expected BindingTargetError, got %T: %v", err, err)
	}

	if bindingErr.NodeID != "target" {
		t.Errorf("expected NodeID 'target', got %q", bindingErr.NodeID)
	}

	if bindingErr.BindingIndex != 0 {
		t.Errorf("expected BindingIndex 0, got %d", bindingErr.BindingIndex)
	}

	t.Logf("Got expected error: %v", err)
}

func TestValidateBindingTarget_ContentIndexOutOfBounds(t *testing.T) {
	// Request with a single-content message
	req, _, err := (ResponseBuilder{}).
		Model(NewModelID("claude-sonnet-4-20250514")).
		System("System prompt").
		User("User prompt").
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Binding targets content[2] on message 1 - doesn't exist (only has 1 content block)
	bindings := []LLMResponsesBindingV0{{
		From:    "upstream",
		Pointer: LLMTextOutput,
		To:      LLMInput().Message(1).Content(2).Text(), // /input/1/content/2/text
	}}

	_, err = WorkflowV0().
		LLMResponsesNodeWithBindings("target", req, nil, bindings)

	if err == nil {
		t.Fatal("expected error for out-of-bounds content index")
	}

	var bindingErr BindingTargetError
	if !errors.As(err, &bindingErr) {
		t.Fatalf("expected BindingTargetError, got %T: %v", err, err)
	}

	t.Logf("Got expected error: %v", err)
}

func TestValidateBindingTarget_ValidMessageIndex(t *testing.T) {
	// Request with 2 messages
	req, _, err := (ResponseBuilder{}).
		Model(NewModelID("claude-sonnet-4-20250514")).
		System("System prompt").
		User("User prompt").
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Binding targets index 1 - exists
	bindings := []LLMResponsesBindingV0{{
		From:    "upstream",
		Pointer: LLMTextOutput,
		To:      LLMUserMessageText, // /input/1/content/0/text
	}}

	_, err = WorkflowV0().
		LLMResponsesNodeWithBindings("target", req, nil, bindings)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBindingTarget_PlaceholderBindingsSkipped(t *testing.T) {
	// Request with placeholder
	req, _, err := (ResponseBuilder{}).
		Model(NewModelID("claude-sonnet-4-20250514")).
		System("Summarize: {{data}}").
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// ToPlaceholder bindings don't have a To path to validate
	bindings := []LLMResponsesBindingV0{{
		From:          "upstream",
		Pointer:       LLMTextOutput,
		ToPlaceholder: "data",
	}}

	_, err = WorkflowV0().
		LLMResponsesNodeWithBindings("target", req, nil, bindings)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBindingTarget_OutputPointersSkipped(t *testing.T) {
	// Request with 1 message
	req, _, err := (ResponseBuilder{}).
		Model(NewModelID("claude-sonnet-4-20250514")).
		User("Hello").
		Build()
	if err != nil {
		t.Fatal(err)
	}

	// Binding targets /output/... - not an input pointer, so not validated
	bindings := []LLMResponsesBindingV0{{
		From:    "upstream",
		Pointer: LLMTextOutput,
		To:      "/output/0/content/0/text", // Not an input pointer
	}}

	_, err = WorkflowV0().
		LLMResponsesNodeWithBindings("target", req, nil, bindings)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBindingTarget_EmptyBindingsPass(t *testing.T) {
	req, _, err := (ResponseBuilder{}).
		Model(NewModelID("claude-sonnet-4-20250514")).
		User("Hello").
		Build()
	if err != nil {
		t.Fatal(err)
	}

	_, err = WorkflowV0().
		LLMResponsesNodeWithBindings("target", req, nil, nil)

	if err != nil {
		t.Fatalf("unexpected error with empty bindings: %v", err)
	}
}

func TestBindingTargetError_Message(t *testing.T) {
	err := BindingTargetError{
		NodeID:       "aggregator",
		BindingIndex: 2,
		Pointer:      "/input/3/content/0/text",
		Message:      "targets /input/3/content/0/text but request only has 2 messages (indices 0-1)",
	}

	expected := `node "aggregator" binding 2: targets /input/3/content/0/text but request only has 2 messages (indices 0-1)`
	if err.Error() != expected {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, err.Error())
	}
}
