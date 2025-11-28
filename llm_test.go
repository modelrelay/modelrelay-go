package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
		Model:     ParseModelID("demo"),
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
		Model:     ParseModelID("demo"),
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

func TestProxyStreamNDJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/x-ndjson" {
			t.Fatalf("expected ndjson accept header, got %s", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set(requestIDHeader, "resp-ndjson")
		lines := []string{
			`{"event":"message_start","response_id":"resp_json","model":"demo"}`,
			`{"event":"message_delta","data":{"foo":"bar"}}`,
			`{"event":"message_stop","response_id":"resp_json","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}`,
		}
		payload := strings.Join(lines, "\n") + "\n"
		w.Write([]byte(payload))
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	stream, err := client.LLM.ProxyStream(context.Background(), ProxyRequest{
		Model:     ParseModelID("demo"),
		MaxTokens: 16,
		Messages:  []llm.ProxyMessage{{Role: "user", Content: "hi"}},
	}, WithNDJSONStream())
	if err != nil {
		t.Fatalf("proxy stream: %v", err)
	}
	t.Cleanup(func() { stream.Close() })

	event, ok, err := stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected first event, err=%v ok=%v", err, ok)
	}
	if event.Kind != llm.StreamEventKindMessageStart {
		t.Fatalf("unexpected kind %s", event.Kind)
	}
	event, ok, err = stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected delta event, err=%v ok=%v", err, ok)
	}
	if event.Kind != llm.StreamEventKindMessageDelta {
		t.Fatalf("expected message_delta, got %s", event.Kind)
	}
	if string(event.Data) != `{"foo":"bar"}` {
		t.Fatalf("unexpected delta data %s", event.Data)
	}
	event, ok, err = stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected stop event, err=%v ok=%v", err, ok)
	}
	if event.Kind != llm.StreamEventKindMessageStop {
		t.Fatalf("expected message_stop, got %s", event.Kind)
	}
	if event.Usage == nil || event.Usage.TotalTokens != 3 {
		t.Fatalf("missing usage %+v", event.Usage)
	}
	if stream.RequestID != "resp-ndjson" {
		t.Fatalf("unexpected request id %s", stream.RequestID)
	}
}

func TestProxyOptionsMergeDefaults(t *testing.T) {
	configHeaders := http.Header{"X-Debug": []string{"cfg", ""}, "X-App": []string{"go-client"}}
	configMetadata := map[string]string{"env": "prod", "trace_id": "cfg", "app": "go"}

	t.Run("blocking", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Debug") != "call" {
				t.Fatalf("expected call-scoped header override, got %q", r.Header.Get("X-Debug"))
			}
			vals := r.Header.Values("X-Multi")
			if len(vals) != 2 || vals[0] != "one" || vals[1] != "two" {
				t.Fatalf("expected two X-Multi values, got %+v", vals)
			}
			if r.Header.Get("X-App") != "go-client" {
				t.Fatalf("expected default header, got %q", r.Header.Get("X-App"))
			}
			if client := r.Header.Get("X-ModelRelay-Client"); client == "" {
				t.Fatalf("missing client header")
			}
			var payload proxyRequestPayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			if payload.Metadata["env"] != "staging" {
				t.Fatalf("expected env from request, got %+v", payload.Metadata)
			}
			if payload.Metadata["trace_id"] != "call" {
				t.Fatalf("expected call metadata override, got %+v", payload.Metadata)
			}
			if payload.Metadata["app"] != "go" {
				t.Fatalf("expected default metadata to persist, got %+v", payload.Metadata)
			}
			if payload.Metadata["user"] != "alice" {
				t.Fatalf("expected per-call metadata entry, got %+v", payload.Metadata)
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(llm.ProxyResponse{ID: "resp_blocking", Provider: "echo", Model: payload.Model, Content: []string{"ok"}, Usage: llm.Usage{}})
		}))
		defer srv.Close()

		client, err := NewClient(Config{
			BaseURL:         srv.URL,
			APIKey:          "test",
			HTTPClient:      srv.Client(),
			DefaultHeaders:  configHeaders,
			DefaultMetadata: configMetadata,
		})
		if err != nil {
			t.Fatalf("new client: %v", err)
		}

		_, err = client.LLM.ProxyMessage(context.Background(), ProxyRequest{
			Model:    ParseModelID("demo"),
			Messages: []llm.ProxyMessage{{Role: "user", Content: "hi"}},
			Metadata: map[string]string{"env": "staging", "request": "true"},
		},
			WithMetadata(map[string]string{"trace_id": "call"}),
			WithMetadataEntry("user", "alice"),
			WithHeader("X-Debug", "call"),
			WithHeader("X-Multi", "one"),
			WithHeader("X-Multi", "two"),
		)
		if err != nil {
			t.Fatalf("proxy message: %v", err)
		}
	})

	t.Run("streaming", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Debug") != "call-stream" {
				t.Fatalf("expected stream header override, got %q", r.Header.Get("X-Debug"))
			}
			var payload proxyRequestPayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			if payload.Metadata["env"] != "sandbox" || payload.Metadata["trace_id"] != "stream" {
				t.Fatalf("unexpected metadata %+v", payload.Metadata)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set(requestIDHeader, "resp-stream-merge")
			fmt.Fprintf(w, "event: message_start\ndata: {\"response_id\":\"resp-123\",\"model\":\"demo\"}\n\n")
			fmt.Fprintf(w, "event: message_stop\ndata: {\"response_id\":\"resp-123\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}\n\n")
		}))
		defer srv.Close()

		client, err := NewClient(Config{
			BaseURL:         srv.URL,
			APIKey:          "test",
			HTTPClient:      srv.Client(),
			DefaultHeaders:  configHeaders,
			DefaultMetadata: configMetadata,
		})
		if err != nil {
			t.Fatalf("new client: %v", err)
		}

		stream, err := client.LLM.ProxyStream(context.Background(), ProxyRequest{
			Model:    ParseModelID("demo"),
			Messages: []llm.ProxyMessage{{Role: "user", Content: "hello"}},
			Metadata: map[string]string{"env": "sandbox"},
		},
			WithMetadataEntry("trace_id", "stream"),
			WithHeader("X-Debug", "call-stream"),
		)
		if err != nil {
			t.Fatalf("proxy stream: %v", err)
		}
		defer stream.Close()

		if stream.RequestID != "resp-stream-merge" {
			t.Fatalf("unexpected request id %s", stream.RequestID)
		}
		_, ok, err := stream.Next()
		if err != nil || !ok {
			t.Fatalf("expected first event err=%v ok=%v", err, ok)
		}
		_, ok, err = stream.Next()
		if err != nil || !ok {
			t.Fatalf("expected stop event err=%v ok=%v", err, ok)
		}
		_, ok, err = stream.Next()
		if err != nil {
			t.Fatalf("final event err=%v", err)
		}
		if ok {
			t.Fatalf("expected end of stream")
		}
	})
}

func TestChatBuilderBlocking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(requestIDHeader); got != "builder-req" {
			t.Fatalf("expected request id header, got %s", got)
		}
		if got := r.Header.Get("X-Debug"); got != "true" {
			t.Fatalf("expected debug header, got %s", got)
		}
		var payload proxyRequestPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.Provider != "openai" || payload.Model != "demo" {
			t.Fatalf("unexpected provider/model %+v", payload)
		}
		if payload.MaxTokens != 32 || payload.Temperature == nil || *payload.Temperature != 0.4 {
			t.Fatalf("unexpected params %+v", payload)
		}
		if len(payload.Messages) != 2 || payload.Messages[0].Role != "system" || payload.Messages[1].Role != "user" {
			t.Fatalf("unexpected messages %+v", payload.Messages)
		}
		if len(payload.Stop) != 1 || payload.Stop[0] != "DONE" {
			t.Fatalf("unexpected stop %+v", payload.Stop)
		}
		if payload.Metadata["trace_id"] != "abc123" {
			t.Fatalf("unexpected metadata %+v", payload.Metadata)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set(requestIDHeader, "resp-builder")
		json.NewEncoder(w).Encode(llm.ProxyResponse{ID: "resp_abc", Provider: "openai", Model: "demo", Content: []string{"ok"}, StopReason: "end_turn", Usage: llm.Usage{TotalTokens: 3}})
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	resp, err := client.LLM.Chat(ParseModelID("demo")).
		Provider(ParseProviderID("openai")).
		MaxTokens(32).
		Temperature(0.4).
		System("you are helpful").
		User("hi there").
		Stop("DONE").
		MetadataEntry("trace_id", "abc123").
		Header("X-Debug", "true").
		RequestID("builder-req").
		Send(context.Background())
	if err != nil {
		t.Fatalf("chat builder send: %v", err)
	}
	if resp.RequestID != "resp-builder" {
		t.Fatalf("unexpected echoed request id %s", resp.RequestID)
	}
	if resp.StopReason != ParseStopReason("end_turn") {
		t.Fatalf("unexpected stop reason %+v", resp.StopReason)
	}
}

func TestChatStreamAdapter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set(requestIDHeader, "resp-chat-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprintf(w, "event: message_start\ndata: {\"response_id\":\"resp_1\",\"model\":\"demo\"}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: message_delta\ndata: {\"response_id\":\"resp_1\",\"delta\":\"Hel\"}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: message_delta\ndata: {\"response_id\":\"resp_1\",\"delta\":{\"text\":\"lo\"}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: message_stop\ndata: {\"response_id\":\"resp_1\",\"model\":\"demo\",\"stop_reason\":\"end_turn\",\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}\n\n")
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	stream, err := client.LLM.Chat(ParseModelID("demo")).
		User("ping").
		RequestID("stream-builder").
		Stream(context.Background())
	if err != nil {
		t.Fatalf("chat stream: %v", err)
	}
	t.Cleanup(func() { stream.Close() })

	if stream.RequestID() != "resp-chat-stream" {
		t.Fatalf("unexpected request id %s", stream.RequestID())
	}

	chunk, ok, err := stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected start event err=%v ok=%v", err, ok)
	}
	if chunk.Type != llm.StreamEventKindMessageStart || chunk.Raw.Kind != llm.StreamEventKindMessageStart {
		t.Fatalf("unexpected start chunk %+v", chunk)
	}

	chunk, ok, err = stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected first delta err=%v ok=%v", err, ok)
	}
	if chunk.TextDelta != "Hel" || chunk.ResponseID != "resp_1" {
		t.Fatalf("unexpected delta chunk %+v", chunk)
	}

	chunk, ok, err = stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected second delta err=%v ok=%v", err, ok)
	}
	if chunk.TextDelta != "lo" {
		t.Fatalf("expected nested text delta, got %+v", chunk)
	}

	chunk, ok, err = stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected stop event err=%v ok=%v", err, ok)
	}
	if chunk.StopReason != ParseStopReason("end_turn") {
		t.Fatalf("unexpected stop reason %+v", chunk)
	}
	if chunk.Usage == nil || chunk.Usage.TotalTokens != 3 {
		t.Fatalf("expected usage in stop chunk %+v", chunk)
	}

	_, ok, err = stream.Next()
	if err != nil {
		t.Fatalf("final event err=%v", err)
	}
	if ok {
		t.Fatalf("expected end of stream")
	}
}

func TestChatStreamAdapterPopulatesMetadataFromMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set(requestIDHeader, "resp-msg-metadata")
		flusher, _ := w.(http.Flusher)
		fmt.Fprintf(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_nested\",\"model\":\"openai/gpt-5.1\"}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"text\":\"hi\"},\"message\":{\"id\":\"msg_nested\",\"model\":\"openai/gpt-5.1\"}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: message_stop\ndata: {\"type\":\"message_stop\",\"stop_reason\":\"end_turn\",\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"total_tokens\":5},\"message\":{\"id\":\"msg_nested\",\"model\":\"openai/gpt-5.1\"}}\n\n")
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	stream, err := client.LLM.Chat(ParseModelID("openai/gpt-5.1")).User("hi").Stream(context.Background())
	if err != nil {
		t.Fatalf("chat stream: %v", err)
	}
	t.Cleanup(func() { stream.Close() })

	chunk, ok, err := stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected start chunk err=%v ok=%v", err, ok)
	}
	if chunk.ResponseID != "msg_nested" || chunk.Model != ParseModelID("openai/gpt-5.1") {
		t.Fatalf("expected response metadata, got %+v", chunk)
	}

	_, ok, err = stream.Next() // delta
	if err != nil || !ok {
		t.Fatalf("expected delta chunk err=%v ok=%v", err, ok)
	}

	chunk, ok, err = stream.Next() // stop
	if err != nil || !ok {
		t.Fatalf("expected stop chunk err=%v ok=%v", err, ok)
	}
	if chunk.StopReason != ParseStopReason("end_turn") || chunk.Usage == nil || chunk.Usage.TotalTokens != 5 {
		t.Fatalf("unexpected stop chunk %+v", chunk)
	}
	if chunk.ResponseID != "msg_nested" || chunk.Model != ParseModelID("openai/gpt-5.1") {
		t.Fatalf("expected response metadata on stop, got %+v", chunk)
	}
}

func TestChatStreamCollectAggregates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set(requestIDHeader, "resp-collect")
		flusher, _ := w.(http.Flusher)
		fmt.Fprintf(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"resp_1\",\"model\":\"openai/gpt-5.1\"}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"text\":\"Hel\"},\"message\":{\"id\":\"resp_1\",\"model\":\"openai/gpt-5.1\"}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"text\":\"lo\"}}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "event: message_stop\ndata: {\"type\":\"message_stop\",\"stop_reason\":\"end_turn\",\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"total_tokens\":5},\"message\":{\"id\":\"resp_1\",\"model\":\"openai/gpt-5.1\"}}\n\n")
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.LLM.Chat(ParseModelID("openai/gpt-5.1")).User("hi").Collect(ctx)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(resp.Content) != 1 || resp.Content[0] != "Hello" {
		t.Fatalf("unexpected content %+v", resp.Content)
	}
	if resp.StopReason != ParseStopReason("end_turn") {
		t.Fatalf("unexpected stop reason %+v", resp.StopReason)
	}
	if resp.Usage.TotalTokens != 5 {
		t.Fatalf("unexpected usage %+v", resp.Usage)
	}
	if resp.Model != ParseModelID("openai/gpt-5.1") || resp.ID != "resp_1" || resp.RequestID != "resp-collect" {
		t.Fatalf("unexpected metadata %+v", resp)
	}
}

func TestSSEStreamCancelsOnContext(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	stream := newSSEStream(ctx, clientConn, TelemetryHooks{})

	type result struct {
		ok  bool
		err error
	}
	done := make(chan result, 1)
	go func() {
		_, ok, err := stream.Next()
		done <- result{ok: ok, err: err}
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	_ = serverConn.Close()
	select {
	case res := <-done:
		if res.ok {
			t.Fatalf("expected stream to end after cancel")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("stream did not cancel promptly")
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
	if got.ResponseID != "resp_xyz" || got.Model != ParseModelID("openai/gpt-test") || got.StopReason != ParseStopReason("end_turn") {
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

	_, err = client.LLM.ProxyMessage(context.Background(), ProxyRequest{Model: ParseModelID("demo"), MaxTokens: 1, Messages: []llm.ProxyMessage{{Role: "user", Content: "ping"}}})
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
