package sdk

import (
	"context"
	"strings"
	"time"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
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
func (s *responseStream) Collect(ctx context.Context) (*Response, error) {
	resp, _, err := s.CollectWithMetrics(ctx)
	return resp, err
}

func (s *responseStream) CollectWithMetrics(ctx context.Context) (*Response, ResponseStreamMetrics, error) {
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = s.handle.Close() }()

	startedAt := s.handle.startedAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}

	var usage *Usage
	var stop StopReason
	var model ModelID
	var responseID string
	var finalContent string
	var contentBuilder strings.Builder
	var sawDelta bool
	var toolCalls []llm.ToolCall
	var firstToken time.Time

	for {
		select {
		case <-ctx.Done():
			return nil, ResponseStreamMetrics{}, ctx.Err()
		default:
		}

		ev, ok, err := s.handle.Next()
		if err != nil {
			return nil, ResponseStreamMetrics{}, err
		}
		if !ok {
			break
		}

		if ev.ErrorStatus > 0 {
			msg := strings.TrimSpace(ev.ErrorMessage)
			if msg == "" {
				msg = "stream error"
			}
			metrics := ResponseStreamMetrics{
				Duration: time.Since(startedAt),
				Model:    model,
				ID:       responseID,
				Usage:    usage,
			}
			if !firstToken.IsZero() {
				metrics.TTFT = firstToken.Sub(startedAt)
			}
			if metrics.TTFT < 0 {
				metrics.TTFT = 0
			}
			if metrics.Duration < 0 {
				metrics.Duration = 0
			}
			return nil, metrics, APIError{
				Status:    ev.ErrorStatus,
				Code:      APIErrorCode(strings.TrimSpace(ev.ErrorCode)),
				Message:   msg,
				RequestID: s.handle.RequestID,
			}
		}

		if ev.ResponseID != "" {
			responseID = ev.ResponseID
		}
		if !ev.Model.IsEmpty() {
			model = ev.Model
		}
		switch ev.Kind {
		case llm.StreamEventKindReasoningDelta:
			// Reasoning tokens count toward TTFT. For reasoning models, the first
			// token arrives during the reasoning phase, which is the correct moment
			// to measure TTFT (not after reasoning completes).
			if ev.ReasoningDelta != "" && firstToken.IsZero() {
				firstToken = time.Now()
			}
		case llm.StreamEventKindMessageDelta:
			if ev.TextDelta != "" {
				if firstToken.IsZero() {
					firstToken = time.Now()
				}
				sawDelta = true
				contentBuilder.WriteString(ev.TextDelta)
			}
		case llm.StreamEventKindMessageStop:
			if ev.TextDelta != "" {
				if firstToken.IsZero() {
					firstToken = time.Now()
				}
				// Completion payload may include the full content; treat it as authoritative.
				finalContent = ev.TextDelta
			}
		default:
			// Ignore non-text events.
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

	if finalContent == "" && (sawDelta || contentBuilder.Len() > 0) {
		finalContent = contentBuilder.String()
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
		ID:         responseID,
		Model:      model,
		Output:     output,
		StopReason: stop,
		RequestID:  s.handle.RequestID,
	}
	if usage != nil {
		resp.Usage = *usage
	}
	metrics := ResponseStreamMetrics{
		Duration: time.Since(startedAt),
		Model:    model,
		ID:       responseID,
		Usage:    usage,
	}
	if !firstToken.IsZero() {
		metrics.TTFT = firstToken.Sub(startedAt)
	}
	if metrics.TTFT < 0 {
		metrics.TTFT = 0
	}
	if metrics.Duration < 0 {
		metrics.Duration = 0
	}
	return resp, metrics, nil
}
