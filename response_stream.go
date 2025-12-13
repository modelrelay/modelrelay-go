package sdk

import (
	"context"

	llm "github.com/modelrelay/modelrelay/providers"
)

type responseStream struct {
	handle *StreamHandle
}

func newResponseStream(handle *StreamHandle) *responseStream {
	return &responseStream{handle: handle}
}

// Collect drains the stream into an aggregated Response. It is pull-based (no
// internal buffering beyond the current NDJSON frame) and respects context
// cancellation. The stream is closed when the call returns.
//
// In the unified NDJSON format, update events contain accumulated content (not
// per-token deltas), so Collect uses the final accumulated content from the
// completion event.
func (s *responseStream) Collect(ctx context.Context) (*Response, error) {
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = s.handle.Close() }()

	var usage *Usage
	var stop StopReason
	var model ModelID
	var responseID string
	var finalContent string
	var toolCalls []llm.ToolCall

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		ev, ok, err := s.handle.Next()
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}

		if ev.ResponseID != "" {
			responseID = ev.ResponseID
		}
		if !ev.Model.IsEmpty() {
			model = ev.Model
		}
		if ev.TextDelta != "" {
			finalContent = ev.TextDelta
		}
		if ev.StopReason != "" {
			stop = ev.StopReason
		}
		if ev.Usage != nil {
			usage = ev.Usage
		}
		if len(ev.ToolCalls) > 0 {
			toolCalls = append(toolCalls[:0], ev.ToolCalls...)
		}
	}

	var output []llm.OutputItem
	if finalContent != "" || len(toolCalls) > 0 {
		item := llm.OutputItem{
			Type:      llm.OutputItemTypeMessage,
			Role:      llm.RoleAssistant,
			ToolCalls: toolCalls,
		}
		if finalContent != "" {
			item.Content = []llm.ContentPart{llm.TextPart(finalContent)}
		}
		output = []llm.OutputItem{item}
	}

	resp := &Response{
		ID:        responseID,
		Model:     model,
		Output:    output,
		StopReason: stop,
		RequestID: s.handle.RequestID,
	}
	if usage != nil {
		resp.Usage = *usage
	}
	return resp, nil
}

