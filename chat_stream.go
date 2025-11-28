package sdk

import (
	"context"
	"encoding/json"
	"strings"

	llm "github.com/modelrelay/modelrelay/llmproxy"
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
func (s *ChatStream) Collect(ctx context.Context) (*ProxyResponse, error) {
	defer s.Close()

	var builder strings.Builder
	var usage *Usage
	var stop StopReason
	var model ModelID
	var responseID string

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
		if chunk.TextDelta != "" {
			builder.WriteString(chunk.TextDelta)
		}
		if chunk.StopReason != "" {
			stop = chunk.StopReason
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
	}

	var content []string
	if builder.Len() > 0 {
		content = []string{builder.String()}
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
	chunk := ChatStreamChunk{
		Type:       event.Kind,
		Raw:        event,
		ResponseID: event.ResponseID,
		Model:      event.Model,
		StopReason: event.StopReason,
		Usage:      event.Usage,
	}

	payload := parsePayload(event.Data)

	if chunk.ResponseID == "" {
		chunk.ResponseID = firstString(payload,
			"response_id", "responseId", "id",
			"message.id", "response.id")
	}
	if chunk.Model.IsEmpty() {
		chunk.Model = ParseModelID(firstString(payload,
			"model", "message.model", "response.model"))
	}
	if chunk.StopReason == "" {
		chunk.StopReason = ParseStopReason(firstString(payload,
			"stop_reason", "stopReason", "message.stop_reason", "response.stop_reason"))
	}
	if chunk.Usage == nil {
		if usage := extractUsage(payload); usage != nil {
			chunk.Usage = usage
		}
	}

	if event.Kind == llm.StreamEventKindMessageDelta {
		chunk.TextDelta = extractTextDeltaPayload(payload)
	}

	return chunk
}

func parsePayload(data []byte) map[string]any {
	if len(data) == 0 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	return payload
}

func firstString(payload map[string]any, paths ...string) string {
	for _, path := range paths {
		if val := lookupString(payload, path); val != "" {
			return val
		}
	}
	return ""
}

func lookupString(payload map[string]any, dotted string) string {
	if payload == nil {
		return ""
	}
	segments := strings.Split(dotted, ".")
	var current any = payload
	for i, seg := range segments {
		m, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		val, ok := m[seg]
		if !ok {
			return ""
		}
		if i == len(segments)-1 {
			if s, ok := val.(string); ok {
				return s
			}
			return ""
		}
		current = val
	}
	return ""
}

func extractUsage(payload map[string]any) *Usage {
	if payload == nil {
		return nil
	}
	if usageVal, ok := payload["usage"]; ok {
		data, err := json.Marshal(usageVal)
		if err != nil {
			return nil
		}
		var usage Usage
		if err := json.Unmarshal(data, &usage); err != nil {
			return nil
		}
		return &usage
	}
	return nil
}

func extractTextDeltaPayload(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if deltaVal, ok := payload["delta"]; ok {
		switch v := deltaVal.(type) {
		case string:
			return v
		case map[string]any:
			if text, ok := v["text"].(string); ok {
				return text
			}
			if content, ok := v["content"].(string); ok {
				return content
			}
		}
	}
	if text, ok := payload["text_delta"].(string); ok {
		return text
	}
	if text, ok := payload["textDelta"].(string); ok {
		return text
	}
	return ""
}
