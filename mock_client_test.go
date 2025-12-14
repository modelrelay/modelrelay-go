package sdk

import (
	"context"
	"errors"
	"testing"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

func TestMockClient_ProxyMessageQueue(t *testing.T) {
	mock := NewMockClient().
		WithResponse(Response{Output: []llm.OutputItem{{Type: llm.OutputItemTypeMessage, Role: llm.RoleAssistant, Content: []llm.ContentPart{llm.TextPart("one")}}}}).
		WithResponseError(errors.New("boom"))

	req := ResponseRequest{model: NewModelID("demo"), input: []llm.InputItem{llm.NewUserText("hi")}}

	resp, err := mock.Responses.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Text() != "one" {
		t.Fatalf("expected first response text 'one', got %q", resp.Text())
	}

	_, err = mock.Responses.Create(context.Background(), req)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected queued error, got %v", err)
	}

	_, err = mock.Responses.Create(context.Background(), req)
	var mErr MockClientError
	if err == nil || !errors.As(err, &mErr) {
		t.Fatalf("expected MockClientError when queue exhausted, got %T %v", err, err)
	}
}

func TestMockClient_ProxyStreamEvents(t *testing.T) {
	events := []StreamEvent{
		{Kind: llm.StreamEventKindMessageStart, ResponseID: "resp_1"},
		{Kind: llm.StreamEventKindMessageDelta, TextDelta: "hi"},
		{Kind: llm.StreamEventKindMessageStop, TextDelta: "hi there"},
	}
	mock := NewMockClient().WithStreamEvents(events)

	req := ResponseRequest{model: NewModelID("demo"), input: []llm.InputItem{llm.NewUserText("hi")}}

	stream, err := mock.Responses.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error creating stream: %v", err)
	}

	for i := 0; i < len(events); i++ {
		ev, ok, nextErr := stream.Next()
		if nextErr != nil || !ok {
			t.Fatalf("expected event %d, err=%v ok=%v", i, nextErr, ok)
		}
		if ev.Kind != events[i].Kind {
			t.Fatalf("expected kind %s, got %s", events[i].Kind, ev.Kind)
		}
	}

	_, ok, err := stream.Next()
	if err != nil || ok {
		t.Fatalf("expected end of stream, err=%v ok=%v", err, ok)
	}
}
