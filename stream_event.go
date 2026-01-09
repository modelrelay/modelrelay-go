package sdk

import (
	"encoding/json"
	"time"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

// StreamEvent represents events from the unified NDJSON streaming format.
type StreamEvent struct {
	Kind          llm.StreamEventKind
	Name          string // Record type: "start", "update", "completion", "error", "reasoning_delta"
	Data          []byte // Raw NDJSON record (deprecated: use structured fields)
	Usage         *Usage
	ResponseID    string
	Model         ModelID
	StopReason    StopReason
	TextDelta     string             // Text content for update/completion events
	ReasoningDelta string            // Reasoning/thinking content for reasoning_delta events
	ToolCalls     []llm.ToolCall     // Completed tool calls (on tool_use_stop or message_stop)
	ToolCallDelta *llm.ToolCallDelta // Incremental tool call data (on tool_use_start or tool_use_delta)
	ToolResult    json.RawMessage    // Tool result payload (on tool_use_stop)

	// Structured streaming fields
	CompleteFields []string // Fields that have been fully received (from complete_fields)

	// Error record fields
	ErrorCode    string // Error code from error records
	ErrorMessage string // Error message from error records
	ErrorStatus  int    // HTTP status code from error records

	// Timing fields (populated by StreamHandle.Next())
	Elapsed time.Duration // Time since stream started when this event was received
}

// EventName returns the SSE event name that should be emitted for this event.
func (e StreamEvent) EventName() string {
	if e.Name != "" {
		return e.Name
	}
	return string(e.Kind)
}
