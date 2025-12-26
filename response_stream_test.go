package sdk

import (
	"context"
	"errors"
	"testing"
	"time"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

type testStream struct {
	events []StreamEvent
	idx    int
	err    error
	closed bool
}

func (t *testStream) Next() (StreamEvent, bool, error) {
	if t.err != nil {
		return StreamEvent{}, false, t.err
	}
	if t.idx >= len(t.events) {
		return StreamEvent{}, false, nil
	}
	ev := t.events[t.idx]
	t.idx++
	return ev, true, nil
}

func (t *testStream) Close() error {
	t.closed = true
	return nil
}

func TestResponseStreamCollectAggregates(t *testing.T) {
	usage := &Usage{TotalTokens: 3}
	handle := &StreamHandle{
		RequestID: "req-1",
		stream: &testStream{events: []StreamEvent{
			{Kind: llm.StreamEventKindMessageDelta, ResponseID: "resp-1", Model: NewModelID("gpt-4o"), TextDelta: "Hello"},
			{
				Kind:       llm.StreamEventKindMessageStop,
				TextDelta:  "Hello world",
				StopReason: StopReasonEndTurn,
				Usage:      usage,
				ToolCalls:  []llm.ToolCall{{ID: "call_1", Type: llm.ToolTypeFunction}},
			},
		}},
		startedAt: time.Now().Add(-10 * time.Millisecond),
	}

	resp, metrics, err := newResponseStream(handle).CollectWithMetrics(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if resp.ID != "resp-1" || resp.Model.String() != "gpt-4o" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Text() != "Hello world" {
		t.Fatalf("unexpected response text %q", resp.Text())
	}
	if len(resp.Output) != 1 || len(resp.Output[0].ToolCalls) != 1 {
		t.Fatalf("expected tool call output")
	}
	if resp.StopReason != StopReasonEndTurn {
		t.Fatalf("unexpected stop reason %s", resp.StopReason)
	}
	if metrics.ID != "resp-1" || metrics.Model.String() != "gpt-4o" || metrics.Usage != usage {
		t.Fatalf("unexpected metrics %+v", metrics)
	}
	if metrics.Duration <= 0 {
		t.Fatalf("expected positive duration")
	}
}

func TestResponseStreamCollectErrorEvent(t *testing.T) {
	handle := &StreamHandle{
		RequestID: "req-2",
		stream: &testStream{events: []StreamEvent{{
			ErrorStatus:  429,
			ErrorCode:    "RATE_LIMIT",
			ErrorMessage: "slow down",
		}}},
		startedAt: time.Now(),
	}

	_, _, err := newResponseStream(handle).CollectWithMetrics(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
	var apiErr APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Status != 429 || apiErr.Code != APIErrorCode("RATE_LIMIT") {
		t.Fatalf("unexpected api error %+v", apiErr)
	}
}

func TestResponseStreamCollectHandlesReaderError(t *testing.T) {
	sentinel := errors.New("stream fail")
	handle := &StreamHandle{RequestID: "req-3", stream: &testStream{err: sentinel}}
	_, _, err := newResponseStream(handle).CollectWithMetrics(context.Background())
	if err == nil || !errors.Is(err, sentinel) {
		t.Fatalf("expected reader error, got %v", err)
	}
}
