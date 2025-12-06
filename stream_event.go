package sdk

import llm "github.com/modelrelay/modelrelay/providers"

// StreamEvent mirrors SSE events emitted by /llm/proxy with typed metadata.
type StreamEvent struct {
	Kind          llm.StreamEventKind
	Name          string
	Data          []byte // Raw backend data (deprecated: use normalized fields)
	Usage         *Usage
	ResponseID    string
	Model         ModelID
	StopReason    StopReason
	TextDelta     string             // Text fragment for message_delta events
	ToolCalls     []llm.ToolCall     // Completed tool calls (on tool_use_stop or message_stop)
	ToolCallDelta *llm.ToolCallDelta // Incremental tool call data (on tool_use_start or tool_use_delta)
}

// EventName returns the SSE event name that should be emitted for this event.
func (e StreamEvent) EventName() string {
	if e.Name != "" {
		return e.Name
	}
	return string(e.Kind)
}
