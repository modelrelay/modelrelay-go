package sdk

import llm "github.com/modelrelay/modelrelay/llmproxy"

// StreamEvent mirrors SSE events emitted by /llm/proxy with typed metadata.
type StreamEvent struct {
	Kind       llm.StreamEventKind
	Name       string
	Data       []byte
	Usage      *Usage
	ResponseID string
	Model      ModelID
	StopReason StopReason
}

// EventName returns the SSE event name that should be emitted for this event.
func (e StreamEvent) EventName() string {
	if e.Name != "" {
		return e.Name
	}
	return string(e.Kind)
}
