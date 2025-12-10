// Package sdk provides the ModelRelay Go SDK for interacting with the ModelRelay API.
package sdk

import (
	"context"

	llm "github.com/modelrelay/modelrelay/providers"
)

// ChatStream wraps a StreamHandle and yields normalized chat deltas while
// preserving access to the underlying raw events.
type ChatStream struct {
	handle *StreamHandle
}

// ChatStreamChunk is a normalized view over streaming chat events.
type ChatStreamChunk struct {
	Type       llm.StreamEventKind
	TextDelta  string
	Usage      *Usage
	StopReason StopReason
	ResponseID string
	Model      ModelID
	Raw        StreamEvent
}

func newChatStream(handle *StreamHandle) *ChatStream {
	return &ChatStream{handle: handle}
}

// Collect drains the stream into an aggregated ProxyResponse. It is pull-based
// (no internal buffering beyond the current SSE frame) and respects context
// cancellation. The stream is closed when the call returns.
//
// In the unified NDJSON format, update events contain accumulated content (not deltas),
// so Collect uses the final content from the completion event.
func (s *ChatStream) Collect(ctx context.Context) (*ProxyResponse, error) {
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = s.Close() }()

	var usage *Usage
	var stop StopReason
	var model ModelID
	var responseID string
	var finalContent string

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		chunk, ok, err := s.Next()
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}

		if chunk.ResponseID != "" {
			responseID = chunk.ResponseID
		}
		if !chunk.Model.IsEmpty() {
			model = chunk.Model
		}
		// In unified format, TextDelta contains accumulated content.
		// Use the latest value (completion event has final content).
		if chunk.TextDelta != "" {
			finalContent = chunk.TextDelta
		}
		if chunk.StopReason != "" {
			stop = chunk.StopReason
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}

	var content []string
	if finalContent != "" {
		content = []string{finalContent}
	}
	resp := &ProxyResponse{
		ID:         responseID,
		Model:      model,
		Content:    content,
		StopReason: stop,
		RequestID:  s.RequestID(),
	}
	if usage != nil {
		resp.Usage = *usage
	}
	return resp, nil
}

// RequestID echoes the X-ModelRelay-Chat-Request-Id header returned by the API.
func (s *ChatStream) RequestID() string {
	return s.handle.RequestID
}

// Raw exposes the underlying StreamHandle for callers that need low-level access.
func (s *ChatStream) Raw() *StreamHandle {
	return s.handle
}

// Next advances the stream, returning false when the stream is complete. Calls
// are pull-based: no internal buffering beyond the current SSE frame, so slow
// consumers backpressure the server naturally.
func (s *ChatStream) Next() (ChatStreamChunk, bool, error) {
	event, ok, err := s.handle.Next()
	if err != nil || !ok {
		return ChatStreamChunk{}, ok, err
	}
	return mapChatStreamChunk(event), true, nil
}

// Close terminates the underlying stream.
func (s *ChatStream) Close() error {
	return s.handle.Close()
}

func mapChatStreamChunk(event StreamEvent) ChatStreamChunk {
	return ChatStreamChunk{
		Type:       event.Kind,
		Raw:        event,
		ResponseID: event.ResponseID,
		Model:      event.Model,
		StopReason: event.StopReason,
		Usage:      event.Usage,
		TextDelta:  event.TextDelta,
	}
}
