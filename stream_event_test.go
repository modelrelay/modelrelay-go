package sdk

import (
	"testing"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

func TestStreamEventEventName(t *testing.T) {
	ev := StreamEvent{Kind: llm.StreamEventKindMessageDelta}
	if ev.EventName() != string(llm.StreamEventKindMessageDelta) {
		t.Fatalf("expected kind event name, got %s", ev.EventName())
	}
	ev.Name = "custom"
	if ev.EventName() != "custom" {
		t.Fatalf("expected custom event name, got %s", ev.EventName())
	}
}
