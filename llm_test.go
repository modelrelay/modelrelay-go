package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	llm "github.com/modelrelay/modelrelay/llmproxy"
	"github.com/modelrelay/modelrelay/llmproxy/sse"
)

func TestProxyMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/llm/proxy" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get(requestIDHeader); got != "req-123" {
			t.Fatalf("expected request id header got %s", got)
		}
		var reqPayload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if reqPayload["model"] != "demo" {
			t.Fatalf("unexpected model %v", reqPayload["model"])
		}
		meta, _ := reqPayload["metadata"].(map[string]any)
		if meta["trace_id"] != "abc123" {
			t.Fatalf("missing metadata: %+v", reqPayload)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set(requestIDHeader, "resp-req-123")
		json.NewEncoder(w).Encode(llm.ProxyResponse{ID: "resp_123", Provider: "openai", Model: "demo", Content: []string{"hi"}, Usage: llm.Usage{}})
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	resp, err := client.LLM.ProxyMessage(context.Background(), ProxyRequest{
		Model:     "demo",
		MaxTokens: 32,
		Messages:  []llm.ProxyMessage{{Role: "user", Content: "ping"}},
		Metadata:  map[string]string{"trace_id": "abc123"},
	}, WithRequestID("req-123"))
	if err != nil {
		t.Fatalf("proxy message: %v", err)
	}
	if resp.ID != "resp_123" {
		t.Fatalf("unexpected id %s", resp.ID)
	}
	if resp.RequestID != "resp-req-123" {
		t.Fatalf("unexpected echoed request id %s", resp.RequestID)
	}
}

func TestProxyStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(requestIDHeader) != "stream-req" {
			t.Fatalf("missing request id header")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set(requestIDHeader, "resp-stream")
		flusher, _ := w.(http.Flusher)
		w.Write([]byte("event: message_start\ndata: {\"response_id\":\"resp_1\",\"model\":\"demo\"}\n\n"))
		flusher.Flush()
		w.Write([]byte("event: message_stop\ndata: {\"response_id\":\"resp_1\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}\n\n"))
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	stream, err := client.LLM.ProxyStream(context.Background(), ProxyRequest{
		Model:     "demo",
		MaxTokens: 16,
		Messages:  []llm.ProxyMessage{{Role: "user", Content: "hi"}},
	}, WithRequestID("stream-req"))
	if err != nil {
		t.Fatalf("proxy stream: %v", err)
	}
	t.Cleanup(func() { stream.Close() })

	event, ok, err := stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected first event got err=%v ok=%v", err, ok)
	}
	if event.Kind != llm.StreamEventKindMessageStart {
		t.Fatalf("unexpected kind %s", event.Kind)
	}
	event, ok, err = stream.Next()
	if err != nil {
		t.Fatalf("second event error: %v", err)
	}
	if !ok {
		t.Fatalf("expected stop event")
	}
	if event.Usage == nil || event.Usage.TotalTokens != 3 {
		t.Fatalf("missing usage: %+v", event)
	}
	_, ok, err = stream.Next()
	if err != nil {
		t.Fatalf("final event err: %v", err)
	}
	if ok {
		t.Fatalf("expected stream end")
	}
	if stream.RequestID != "resp-stream" {
		t.Fatalf("unexpected stream request id %s", stream.RequestID)
	}
}

func TestUsageSummary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/llm/usage" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"summary":{"plan":"pro","limit":10,"used":1,"remaining":9,"window_start":"2024-01-01T00:00:00Z","window_end":"2024-01-31T00:00:00Z","state":"allowed"}}`)
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	summary, err := client.Usage.Summary(context.Background())
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.Remaining != 9 {
		t.Fatalf("unexpected summary %+v", summary)
	}
}

func TestStreamParsesLLMProxySSE(t *testing.T) {
	rec := httptest.NewRecorder()
	writer, err := sse.NewHTTPWriter(rec)
	if err != nil {
		t.Fatalf("new http writer: %v", err)
	}
	event := llm.StreamEvent{
		Kind: llm.StreamEventKindMessageStop,
		Data: llm.MarshalEvent(map[string]any{
			"response_id": "resp_xyz",
			"model":       "openai/gpt-test",
			"stop_reason": "end_turn",
		}),
	}
	if err := writer.Write(event); err != nil {
		t.Fatalf("write event: %v", err)
	}

	stream := newSSEStream(context.Background(), io.NopCloser(bytes.NewReader(rec.Body.Bytes())), TelemetryHooks{})
	got, ok, err := stream.Next()
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if !ok {
		t.Fatalf("expected event")
	}
	if got.Kind != llm.StreamEventKindMessageStop {
		t.Fatalf("unexpected kind %s", got.Kind)
	}
	if got.ResponseID != "resp_xyz" || got.Model != "openai/gpt-test" || got.StopReason != "end_turn" {
		t.Fatalf("unexpected event metadata %+v", got)
	}
}

func TestAPIErrorDecoding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":{"code":"INVALID","message":"nope","status":401},"request_id":"req_123"}`)
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.LLM.ProxyMessage(context.Background(), ProxyRequest{Model: "demo", MaxTokens: 1, Messages: []llm.ProxyMessage{{Role: "user", Content: "ping"}}})
	if err == nil {
		t.Fatalf("expected error")
	}
	var apiErr APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError got %T", err)
	}
	if apiErr.Code != "INVALID" || apiErr.RequestID != "req_123" {
		t.Fatalf("unexpected api error %+v", apiErr)
	}
}
