package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelrelay/modelrelay/platform/headers"
	llm "github.com/modelrelay/modelrelay/providers"
)

func TestProxyMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/llm/proxy" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get(headers.ChatRequestID); got != "req-123" {
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
		w.Header().Set(headers.ChatRequestID, "resp-req-123")
		json.NewEncoder(w).Encode(llm.ProxyResponse{ID: "resp_123", Provider: "openai", Model: "demo", Content: []string{"hi"}, Usage: llm.Usage{}})
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	resp, err := client.LLM.ProxyMessage(context.Background(), ProxyRequest{
		Model:     NewModelID("demo"),
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
		if r.Header.Get(headers.ChatRequestID) != "stream-req" {
			t.Fatalf("missing request id header")
		}
		// Output unified NDJSON format
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set(headers.ChatRequestID, "resp-stream")
		flusher, _ := w.(http.Flusher)
		w.Write([]byte(`{"type":"start","request_id":"resp_1","model":"demo"}` + "\n"))
		flusher.Flush()
		w.Write([]byte(`{"type":"update","payload":{"content":"Hello"}}` + "\n"))
		flusher.Flush()
		w.Write([]byte(`{"type":"completion","payload":{"content":"Hello world"},"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}` + "\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.LLM.ProxyStream(ctx, ProxyRequest{
		Model:     NewModelID("demo"),
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
	if err != nil || !ok {
		t.Fatalf("expected delta event got err=%v ok=%v", err, ok)
	}
	if event.Kind != llm.StreamEventKindMessageDelta {
		t.Fatalf("expected delta kind, got %s", event.Kind)
	}
	event, ok, err = stream.Next()
	if err != nil {
		t.Fatalf("completion event error: %v", err)
	}
	if !ok {
		t.Fatalf("expected completion event")
	}
	if event.Kind != llm.StreamEventKindMessageStop {
		t.Fatalf("expected stop kind, got %s", event.Kind)
	}
	if event.Usage == nil || event.Usage.TotalTokens != 3 {
		t.Fatalf("missing usage: %+v", event)
	}
	// Note: We don't test for end-of-stream (ok=false) here because the HTTP
	// connection close timing is non-deterministic and can cause test flakiness.
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
		w.Header().Set(headers.ChatRequestID, "resp-ndjson")
		lines := []string{
			`{"type":"start","request_id":"resp_json","model":"demo"}`,
			`{"type":"update","payload":{"content":"bar"}}`,
			`{"type":"completion","payload":{"content":"bar complete"},"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}`,
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
		Model:     NewModelID("demo"),
		MaxTokens: 16,
		Messages:  []llm.ProxyMessage{{Role: "user", Content: "hi"}},
	})
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
	if event.TextDelta != "bar" {
		t.Fatalf("unexpected text delta %s", event.TextDelta)
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

type structuredPayload struct {
	Items []struct {
		ID string `json:"id"`
	} `json:"items"`
}

func TestProxyStreamJSON_StructuredHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/x-ndjson" {
			t.Fatalf("expected ndjson accept header, got %s", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set(headers.ChatRequestID, "req-structured")
		lines := []string{
			`{"type":"start","request_id":"stream-req-123"}`,
			`{"type":"update","payload":{"items":[{"id":"one"}]}}`,
			`{"type":"completion","payload":{"items":[{"id":"one"},{"id":"two"}]}}`,
		}
		payload := strings.Join(lines, "\n") + "\n"
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	schema := json.RawMessage(`{"type":"object","properties":{"tiers":{"type":"array"}}}`)
	req := ProxyRequest{
		Model:     NewModelID("demo"),
		MaxTokens: 16,
		Messages:  []llm.ProxyMessage{{Role: "user", Content: "hi"}},
		ResponseFormat: &llm.ResponseFormat{
			Type: llm.ResponseFormatTypeJSONSchema,
			JSONSchema: &llm.JSONSchemaFormat{
				Name:   "tiers",
				Schema: schema,
			},
		},
	}

	stream, err := ProxyStreamJSON[structuredPayload](ctx, client.LLM, req)
	if err != nil {
		t.Fatalf("ProxyStreamJSON: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })

	// First update
	event, ok, err := stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected first event, err=%v ok=%v", err, ok)
	}
	if event.Type != StructuredRecordTypeUpdate {
		t.Fatalf("expected update record, got %s", event.Type)
	}
	if event.Payload == nil || len(event.Payload.Items) != 1 || event.Payload.Items[0].ID != "one" {
		t.Fatalf("unexpected update payload: %+v", event.Payload)
	}
	if event.RequestID != "req-structured" {
		t.Fatalf("unexpected request id on event: %s", event.RequestID)
	}

	// Completion
	event, ok, err = stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected completion event, err=%v ok=%v", err, ok)
	}
	if event.Type != StructuredRecordTypeCompletion {
		t.Fatalf("expected completion record, got %s", event.Type)
	}
	if event.Payload == nil || len(event.Payload.Items) != 2 {
		t.Fatalf("unexpected completion payload: %+v", event.Payload)
	}

	// Collect from a fresh stream to exercise the helper.
	stream2, err := ProxyStreamJSON[structuredPayload](ctx, client.LLM, req)
	if err != nil {
		t.Fatalf("ProxyStreamJSON second call: %v", err)
	}
	t.Cleanup(func() { _ = stream2.Close() })

	payload, err := stream2.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(payload.Items) != 2 {
		t.Fatalf("expected 2 items from Collect, got %d", len(payload.Items))
	}
	if stream2.RequestID() != "req-structured" {
		t.Fatalf("unexpected stream RequestID %s", stream2.RequestID())
	}
}

func TestProxyStreamJSON_ProtocolViolation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set(headers.ChatRequestID, "req-bad")
		// Stream ends without completion or error.
		_, _ = w.Write([]byte(`{"type":"update","payload":{"items":[{"id":"one"}]}}` + "\n"))
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := ProxyRequest{
		Model:     NewModelID("demo"),
		MaxTokens: 16,
		Messages:  []llm.ProxyMessage{{Role: "user", Content: "hi"}},
		ResponseFormat: &llm.ResponseFormat{
			Type: llm.ResponseFormatTypeJSONSchema,
			JSONSchema: &llm.JSONSchemaFormat{
				Name:   "test",
				Schema: []byte(`{"type":"object"}`),
			},
		},
	}

	stream, err := ProxyStreamJSON[structuredPayload](ctx, client.LLM, req)
	if err != nil {
		t.Fatalf("ProxyStreamJSON: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })

	_, err = stream.Collect(ctx)
	var terr TransportError
	if err == nil || !errors.As(err, &terr) {
		t.Fatalf("expected TransportError on protocol violation, got %T %v", err, err)
	}
}

func TestProxyStreamJSON_ErrorRecord(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set(headers.ChatRequestID, "req-error")
		_, _ = w.Write([]byte(`{"type":"error","code":"SERVICE_UNAVAILABLE","message":"upstream timeout","status":502}` + "\n"))
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := ProxyRequest{
		Model:     NewModelID("demo"),
		MaxTokens: 16,
		Messages:  []llm.ProxyMessage{{Role: "user", Content: "hi"}},
		ResponseFormat: &llm.ResponseFormat{
			Type: llm.ResponseFormatTypeJSONSchema,
			JSONSchema: &llm.JSONSchemaFormat{
				Name:   "test",
				Schema: []byte(`{"type":"object"}`),
			},
		},
	}

	stream, err := ProxyStreamJSON[structuredPayload](ctx, client.LLM, req)
	if err != nil {
		t.Fatalf("ProxyStreamJSON: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })

	_, _, err = stream.Next()
	var apiErr APIError
	if err == nil || !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError from error record, got %T %v", err, err)
	}
	if apiErr.Status != 502 || apiErr.Code != ErrCodeUnavailable {
		t.Fatalf("unexpected api error: status=%d code=%s", apiErr.Status, apiErr.Code)
	}
	if apiErr.RequestID != "req-error" {
		t.Fatalf("expected request id on api error, got %s", apiErr.RequestID)
	}
}

func TestProxyStreamJSON_RequiresStructuredResponseFormat(t *testing.T) {
	ctx := context.Background()
	llmClient := &LLMClient{}

	baseReq := ProxyRequest{
		Model:     NewModelID("demo"),
		MaxTokens: 16,
		Messages:  []llm.ProxyMessage{{Role: "user", Content: "hi"}},
	}

	tests := []struct {
		name string
		req  ProxyRequest
	}{
		{name: "missing response_format", req: baseReq},
		{name: "non-structured response_format", req: func() ProxyRequest {
			r := baseReq
			r.ResponseFormat = &llm.ResponseFormat{Type: llm.ResponseFormatTypeText}
			return r
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ProxyStreamJSON[structuredPayload](ctx, llmClient, tt.req)
			var cfgErr ConfigError
			if err == nil || !errors.As(err, &cfgErr) {
				t.Fatalf("expected ConfigError, got %T %v", err, err)
			}
		})
	}
}

func TestStructuredJSONStream_IgnoresUnknownTypes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set(headers.ChatRequestID, "req-unknown")
		lines := []string{
			`{"type":"progress","payload":{"ignored":true}}`,
			`{"type":"update","payload":{"items":[{"id":"one"}]}}`,
			`{"type":"completion","payload":{"items":[{"id":"one"},{"id":"two"}]}}`,
		}
		_, _ = w.Write([]byte(strings.Join(lines, "\n") + "\n"))
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := ProxyRequest{
		Model:     NewModelID("demo"),
		MaxTokens: 16,
		Messages:  []llm.ProxyMessage{{Role: "user", Content: "hi"}},
		ResponseFormat: &llm.ResponseFormat{
			Type: llm.ResponseFormatTypeJSONSchema,
			JSONSchema: &llm.JSONSchemaFormat{
				Name:   "test",
				Schema: []byte(`{"type":"object"}`),
			},
		},
	}

	stream, err := ProxyStreamJSON[structuredPayload](ctx, client.LLM, req)
	if err != nil {
		t.Fatalf("ProxyStreamJSON: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })

	var kinds []StructuredRecordType
	for {
		event, ok, err := stream.Next()
		if err != nil || !ok {
			break
		}
		kinds = append(kinds, event.Type)
	}
	if len(kinds) != 2 || kinds[0] != StructuredRecordTypeUpdate || kinds[1] != StructuredRecordTypeCompletion {
		t.Fatalf("unexpected record kinds: %+v", kinds)
	}
}

func TestStructuredJSONStream_InvalidJSONLine(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set(headers.ChatRequestID, "req-invalid-json")
		// Invalid JSON line in the stream.
		_, _ = w.Write([]byte("not-json\n"))
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := ProxyRequest{
		Model:     NewModelID("demo"),
		MaxTokens: 16,
		Messages:  []llm.ProxyMessage{{Role: "user", Content: "hi"}},
		ResponseFormat: &llm.ResponseFormat{
			Type: llm.ResponseFormatTypeJSONSchema,
			JSONSchema: &llm.JSONSchemaFormat{
				Name:   "test",
				Schema: []byte(`{"type":"object"}`),
			},
		},
	}

	stream, err := ProxyStreamJSON[structuredPayload](ctx, client.LLM, req)
	if err != nil {
		t.Fatalf("ProxyStreamJSON: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })

	_, _, err = stream.Next()
	var terr TransportError
	if err == nil || !errors.As(err, &terr) {
		t.Fatalf("expected TransportError for invalid JSON, got %T %v", err, err)
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
			Model:    NewModelID("demo"),
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
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.Header().Set(headers.ChatRequestID, "resp-stream-merge")
			flusher, _ := w.(http.Flusher)
			w.Write([]byte(`{"type":"start","request_id":"resp-123","model":"demo"}` + "\n"))
			flusher.Flush()
			w.Write([]byte(`{"type":"completion","payload":{"content":"done"},"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}` + "\n"))
			flusher.Flush()
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

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		stream, err := client.LLM.ProxyStream(ctx, ProxyRequest{
			Model:    NewModelID("demo"),
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
		// Note: We don't test for end-of-stream (ok=false) here because the HTTP
		// connection close timing is non-deterministic and can cause test flakiness.
	})
}

func TestChatBuilderBlocking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(headers.ChatRequestID); got != "builder-req" {
			t.Fatalf("expected request id header, got %s", got)
		}
		if got := r.Header.Get("X-Debug"); got != "true" {
			t.Fatalf("expected debug header, got %s", got)
		}
		var payload proxyRequestPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.Model != "demo" {
			t.Fatalf("unexpected model %+v", payload)
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
		w.Header().Set(headers.ChatRequestID, "resp-builder")
		json.NewEncoder(w).Encode(llm.ProxyResponse{ID: "resp_abc", Provider: "openai", Model: "demo", Content: []string{"ok"}, StopReason: "end_turn", Usage: llm.Usage{TotalTokens: 3}})
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	resp, err := client.LLM.Chat(NewModelID("demo")).
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
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set(headers.ChatRequestID, "resp-chat-stream")
		flusher, _ := w.(http.Flusher)
		w.Write([]byte(`{"type":"start","request_id":"resp_1","model":"demo"}` + "\n"))
		flusher.Flush()
		w.Write([]byte(`{"type":"update","payload":{"content":"Hel"}}` + "\n"))
		flusher.Flush()
		w.Write([]byte(`{"type":"update","payload":{"content":"Hello"}}` + "\n"))
		flusher.Flush()
		w.Write([]byte(`{"type":"completion","payload":{"content":"Hello"},"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}` + "\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.LLM.Chat(NewModelID("demo")).
		User("ping").
		RequestID("stream-builder").
		Stream(ctx)
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
	if chunk.TextDelta != "Hello" {
		t.Fatalf("expected accumulated text, got %+v", chunk)
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
	// Note: We don't test for end-of-stream (ok=false) here because the HTTP
	// connection close timing is non-deterministic and can cause test flakiness.
	// The message_stop event with usage data indicates the stream is complete.
}

func TestChatStreamAdapterPopulatesMetadataFromMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set(headers.ChatRequestID, "resp-msg-metadata")
		flusher, _ := w.(http.Flusher)
		w.Write([]byte(`{"type":"start","request_id":"msg_nested","model":"gpt-5.1"}` + "\n"))
		flusher.Flush()
		w.Write([]byte(`{"type":"update","payload":{"content":"hi"}}` + "\n"))
		flusher.Flush()
		w.Write([]byte(`{"type":"completion","payload":{"content":"hi"},"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}` + "\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.LLM.Chat(NewModelID("gpt-5.1")).User("hi").Stream(ctx)
	if err != nil {
		t.Fatalf("chat stream: %v", err)
	}
	t.Cleanup(func() { stream.Close() })

	chunk, ok, err := stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected start chunk err=%v ok=%v", err, ok)
	}
	if chunk.ResponseID != "msg_nested" || chunk.Model != NewModelID("gpt-5.1") {
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
	if chunk.ResponseID != "msg_nested" || chunk.Model != NewModelID("gpt-5.1") {
		t.Fatalf("expected response metadata on stop, got %+v", chunk)
	}
}

func TestChatStreamCollectAggregates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set(headers.ChatRequestID, "resp-collect")
		flusher, _ := w.(http.Flusher)
		w.Write([]byte(`{"type":"start","request_id":"resp_1","model":"gpt-5.1"}` + "\n"))
		flusher.Flush()
		w.Write([]byte(`{"type":"update","payload":{"content":"Hel"}}` + "\n"))
		flusher.Flush()
		w.Write([]byte(`{"type":"update","payload":{"content":"Hello"}}` + "\n"))
		flusher.Flush()
		w.Write([]byte(`{"type":"completion","payload":{"content":"Hello"},"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}` + "\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.LLM.Chat(NewModelID("gpt-5.1")).User("hi").Collect(ctx)
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
	if resp.Model != NewModelID("gpt-5.1") || resp.ID != "resp_1" || resp.RequestID != "resp-collect" {
		t.Fatalf("unexpected metadata %+v", resp)
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

func TestStreamParsesNDJSONFormat(t *testing.T) {
	// Test parsing the unified NDJSON streaming format with record types
	ndjsonContent := `{"type":"start","request_id":"req-123","provider":"test","model":"gpt-test"}
{"type":"update","payload":{"content":"Hello"}}
{"type":"completion","payload":{"content":"Hello World"},"complete_fields":["content"]}
`
	stream := newNDJSONStream(context.Background(), io.NopCloser(strings.NewReader(ndjsonContent)), TelemetryHooks{})
	defer stream.Close()

	// First event: start
	got, ok, err := stream.Next()
	if err != nil {
		t.Fatalf("next start: %v", err)
	}
	if !ok {
		t.Fatalf("expected start event")
	}
	if got.Kind != llm.StreamEventKindMessageStart {
		t.Fatalf("expected message_start, got %s", got.Kind)
	}

	// Second event: update
	got, ok, err = stream.Next()
	if err != nil {
		t.Fatalf("next update: %v", err)
	}
	if !ok {
		t.Fatalf("expected update event")
	}
	if got.Kind != llm.StreamEventKindMessageDelta {
		t.Fatalf("expected message_delta, got %s", got.Kind)
	}

	// Third event: completion
	got, ok, err = stream.Next()
	if err != nil {
		t.Fatalf("next completion: %v", err)
	}
	if !ok {
		t.Fatalf("expected completion event")
	}
	if got.Kind != llm.StreamEventKindMessageStop {
		t.Fatalf("expected message_stop, got %s", got.Kind)
	}

	// Stream should be exhausted
	_, ok, err = stream.Next()
	if err != nil {
		t.Fatalf("next eof: %v", err)
	}
	if ok {
		t.Fatalf("expected stream to be exhausted")
	}
}

func TestStreamParsesToolUseEvents(t *testing.T) {
	// Test parsing tool_use_start, tool_use_delta, and tool_use_stop events
	ndjsonContent := `{"type":"start","request_id":"req-tools","model":"gpt-4"}
{"type":"tool_use_start","tool_call_delta":{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather"}}}
{"type":"tool_use_delta","tool_call_delta":{"index":0,"function":{"arguments":"{\"location\":"}}}
{"type":"tool_use_stop","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"location\":\"NYC\"}"}}]}
{"type":"completion","payload":{"content":"Done"},"stop_reason":"tool_calls"}
`
	stream := newNDJSONStream(context.Background(), io.NopCloser(strings.NewReader(ndjsonContent)), TelemetryHooks{})
	defer stream.Close()

	// First event: start
	got, ok, err := stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected start event, err=%v ok=%v", err, ok)
	}
	if got.Kind != llm.StreamEventKindMessageStart {
		t.Fatalf("expected message_start, got %s", got.Kind)
	}

	// Second event: tool_use_start
	got, ok, err = stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected tool_use_start event, err=%v ok=%v", err, ok)
	}
	if got.Kind != llm.StreamEventKindToolUseStart {
		t.Fatalf("expected tool_use_start, got %s", got.Kind)
	}
	if got.ToolCallDelta == nil {
		t.Fatalf("expected tool_call_delta to be populated")
	}
	if got.ToolCallDelta.Index != 0 {
		t.Fatalf("expected index 0, got %d", got.ToolCallDelta.Index)
	}
	if got.ToolCallDelta.ID != "call_1" {
		t.Fatalf("expected id call_1, got %s", got.ToolCallDelta.ID)
	}
	if got.ToolCallDelta.Function == nil || got.ToolCallDelta.Function.Name != "get_weather" {
		t.Fatalf("expected function name get_weather, got %+v", got.ToolCallDelta.Function)
	}

	// Third event: tool_use_delta
	got, ok, err = stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected tool_use_delta event, err=%v ok=%v", err, ok)
	}
	if got.Kind != llm.StreamEventKindToolUseDelta {
		t.Fatalf("expected tool_use_delta, got %s", got.Kind)
	}
	if got.ToolCallDelta == nil {
		t.Fatalf("expected tool_call_delta to be populated for delta")
	}

	// Fourth event: tool_use_stop
	got, ok, err = stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected tool_use_stop event, err=%v ok=%v", err, ok)
	}
	if got.Kind != llm.StreamEventKindToolUseStop {
		t.Fatalf("expected tool_use_stop, got %s", got.Kind)
	}
	if len(got.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(got.ToolCalls))
	}
	if got.ToolCalls[0].ID != "call_1" {
		t.Fatalf("expected tool call id call_1, got %s", got.ToolCalls[0].ID)
	}
	if got.ToolCalls[0].Function.Name != "get_weather" {
		t.Fatalf("expected function name get_weather, got %s", got.ToolCalls[0].Function.Name)
	}
	if got.ToolCalls[0].Function.Arguments != `{"location":"NYC"}` {
		t.Fatalf("expected arguments, got %s", got.ToolCalls[0].Function.Arguments)
	}

	// Fifth event: completion
	got, ok, err = stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected completion event, err=%v ok=%v", err, ok)
	}
	if got.Kind != llm.StreamEventKindMessageStop {
		t.Fatalf("expected message_stop, got %s", got.Kind)
	}
}

func TestStreamParsesSingleToolCall(t *testing.T) {
	// Test parsing tool_use_stop with a single tool_call (not array)
	ndjsonContent := `{"type":"tool_use_stop","tool_call":{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{}"}}}
`
	stream := newNDJSONStream(context.Background(), io.NopCloser(strings.NewReader(ndjsonContent)), TelemetryHooks{})
	defer stream.Close()

	got, ok, err := stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected tool_use_stop event, err=%v ok=%v", err, ok)
	}
	if got.Kind != llm.StreamEventKindToolUseStop {
		t.Fatalf("expected tool_use_stop, got %s", got.Kind)
	}
	if len(got.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call from single tool_call field, got %d", len(got.ToolCalls))
	}
	if got.ToolCalls[0].ID != "call_1" {
		t.Fatalf("expected tool call id call_1, got %s", got.ToolCalls[0].ID)
	}
}

func TestAPIErrorDecoding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, `{"error":"unauthorized","code":"INVALID","message":"nope","request_id":"req_123"}`)
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.LLM.ProxyMessage(context.Background(), ProxyRequest{Model: NewModelID("demo"), MaxTokens: 1, Messages: []llm.ProxyMessage{{Role: llm.RoleUser, Content: "ping"}}})
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

func TestCustomerChatBuilderBlocking(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify customer ID header is set
		customerID := r.Header.Get(headers.CustomerID)
		if customerID != "cust-123" {
			t.Fatalf("expected customer ID header 'cust-123', got %q", customerID)
		}
		// Verify no model is required in payload (tier determines it)
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		// Model should be empty or not present for customer-attributed requests
		if model, ok := payload["model"].(string); ok && model != "" {
			t.Fatalf("expected no model in customer chat request, got %q", model)
		}
		if payload["max_tokens"] != float64(64) {
			t.Fatalf("unexpected max_tokens: %v", payload["max_tokens"])
		}
		msgs, _ := payload["messages"].([]any)
		if len(msgs) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(msgs))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set(headers.ChatRequestID, "resp-customer")
		json.NewEncoder(w).Encode(llm.ProxyResponse{
			ID:       "resp_customer_123",
			Provider: "openai",
			Model:    "gpt-4o",
			Content:  []string{"Hello customer!"},
			Usage:    llm.Usage{TotalTokens: 10},
		})
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	resp, err := client.LLM.ChatForCustomer("cust-123").
		System("You are a helpful assistant.").
		User("Hello!").
		MaxTokens(64).
		RequestID("customer-req").
		Send(context.Background())
	if err != nil {
		t.Fatalf("customer chat send: %v", err)
	}
	if resp.ID != "resp_customer_123" {
		t.Fatalf("unexpected response id: %s", resp.ID)
	}
	if resp.RequestID != "resp-customer" {
		t.Fatalf("unexpected request id: %s", resp.RequestID)
	}
}

func TestCustomerChatBuilderStreaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify customer ID header is set
		customerID := r.Header.Get(headers.CustomerID)
		if customerID != "cust-456" {
			t.Fatalf("expected customer ID header 'cust-456', got %q", customerID)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set(headers.ChatRequestID, "resp-customer-stream")
		flusher, _ := w.(http.Flusher)
		w.Write([]byte(`{"type":"start","request_id":"resp_1","model":"gpt-4o"}` + "\n"))
		flusher.Flush()
		w.Write([]byte(`{"type":"update","payload":{"content":"Hi!"}}` + "\n"))
		flusher.Flush()
		w.Write([]byte(`{"type":"completion","payload":{"content":"Hi!"},"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}` + "\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.LLM.ChatForCustomer("cust-456").
		User("Hello!").
		Stream(ctx)
	if err != nil {
		t.Fatalf("customer chat stream: %v", err)
	}
	t.Cleanup(func() { stream.Close() })

	if stream.RequestID() != "resp-customer-stream" {
		t.Fatalf("unexpected request id: %s", stream.RequestID())
	}

	// Consume events - in unified format, both update and completion events have content
	var lastContent string
	var lastErr error
	for {
		chunk, ok, err := stream.Next()
		if err != nil {
			lastErr = err
			break
		}
		if !ok {
			break
		}
		if chunk.TextDelta != "" {
			lastContent = chunk.TextDelta
		}
	}
	if lastErr != nil {
		t.Fatalf("unexpected error during stream consumption: %v", lastErr)
	}
	if lastContent != "Hi!" {
		t.Fatalf("unexpected final content: %v", lastContent)
	}
}

func TestCustomerChatBuilderCollect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		customerID := r.Header.Get(headers.CustomerID)
		if customerID != "cust-789" {
			t.Fatalf("expected customer ID header 'cust-789', got %q", customerID)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set(headers.ChatRequestID, "resp-customer-collect")
		flusher, _ := w.(http.Flusher)
		w.Write([]byte(`{"type":"start","request_id":"resp_collect","model":"gpt-4o"}` + "\n"))
		flusher.Flush()
		w.Write([]byte(`{"type":"update","payload":{"content":"Hello "}}` + "\n"))
		flusher.Flush()
		w.Write([]byte(`{"type":"update","payload":{"content":"Hello there!"}}` + "\n"))
		flusher.Flush()
		w.Write([]byte(`{"type":"completion","payload":{"content":"Hello there!"},"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}` + "\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.LLM.ChatForCustomer("cust-789").
		User("Hi").
		Collect(ctx)
	if err != nil {
		t.Fatalf("customer chat collect: %v", err)
	}
	if len(resp.Content) != 1 || resp.Content[0] != "Hello there!" {
		t.Fatalf("unexpected content: %v", resp.Content)
	}
	if resp.Usage.TotalTokens != 7 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
	if resp.RequestID != "resp-customer-collect" {
		t.Fatalf("unexpected request id: %s", resp.RequestID)
	}
}

func TestMessageRoleConstants(t *testing.T) {
	// Verify that MessageRole constants serialize to expected strings
	if llm.RoleUser != "user" {
		t.Fatalf("expected RoleUser to be 'user', got %q", llm.RoleUser)
	}
	if llm.RoleAssistant != "assistant" {
		t.Fatalf("expected RoleAssistant to be 'assistant', got %q", llm.RoleAssistant)
	}
	if llm.RoleSystem != "system" {
		t.Fatalf("expected RoleSystem to be 'system', got %q", llm.RoleSystem)
	}
	if llm.RoleTool != "tool" {
		t.Fatalf("expected RoleTool to be 'tool', got %q", llm.RoleTool)
	}
}

func TestChatBuilderWithTypedRoles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload proxyRequestPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		// Verify roles are serialized correctly
		if len(payload.Messages) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(payload.Messages))
		}
		if payload.Messages[0].Role != llm.RoleSystem {
			t.Fatalf("expected system role, got %v", payload.Messages[0].Role)
		}
		if payload.Messages[1].Role != llm.RoleUser {
			t.Fatalf("expected user role, got %v", payload.Messages[1].Role)
		}
		if payload.Messages[2].Role != llm.RoleAssistant {
			t.Fatalf("expected assistant role, got %v", payload.Messages[2].Role)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(llm.ProxyResponse{ID: "resp_typed", Provider: "openai", Model: "demo", Content: []string{"ok"}, Usage: llm.Usage{}})
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.LLM.Chat(NewModelID("demo")).
		Message(llm.RoleSystem, "System prompt").
		Message(llm.RoleUser, "User message").
		Message(llm.RoleAssistant, "Assistant response").
		Send(context.Background())
	if err != nil {
		t.Fatalf("chat with typed roles: %v", err)
	}
}

func TestCustomerChatBuilderEmptyCustomerID(t *testing.T) {
	client, err := NewClient(Config{BaseURL: "http://localhost", APIKey: "test"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.LLM.ChatForCustomer("").
		User("Hello").
		Send(context.Background())
	if err == nil {
		t.Fatal("expected error for empty customer ID")
	}
	if err.Error() != "customer ID is required" {
		t.Fatalf("unexpected error: %v", err)
	}

	// Also test streaming
	_, err = client.LLM.ChatForCustomer("").
		User("Hello").
		Stream(context.Background())
	if err == nil {
		t.Fatal("expected error for empty customer ID on stream")
	}
	if err.Error() != "customer ID is required" {
		t.Fatalf("unexpected error on stream: %v", err)
	}
}

func TestCustomerChatBuilderNoMessages(t *testing.T) {
	client, err := NewClient(Config{BaseURL: "http://localhost", APIKey: "test"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.LLM.ChatForCustomer("cust-123").
		Send(context.Background())
	if err == nil {
		t.Fatal("expected error for no messages")
	}
	if err.Error() != "at least one message is required" {
		t.Fatalf("unexpected error: %v", err)
	}

	// Also test streaming
	_, err = client.LLM.ChatForCustomer("cust-123").
		Stream(context.Background())
	if err == nil {
		t.Fatal("expected error for no messages on stream")
	}
	if err.Error() != "at least one message is required" {
		t.Fatalf("unexpected error on stream: %v", err)
	}
}

func TestCustomerChatAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set(headers.ChatRequestID, "req-customer-error")
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"error":"customer_not_found","code":"CUSTOMER_NOT_FOUND","message":"Customer cust-999 not found","request_id":"req-customer-error"}`)
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.LLM.ChatForCustomer("cust-999").
		User("Hello").
		Send(context.Background())
	if err == nil {
		t.Fatal("expected error for customer not found")
	}

	var apiErr APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.Status != http.StatusNotFound {
		t.Fatalf("unexpected status: %d", apiErr.Status)
	}
	if apiErr.Code != "CUSTOMER_NOT_FOUND" {
		t.Fatalf("unexpected code: %s", apiErr.Code)
	}
	if apiErr.RequestID != "req-customer-error" {
		t.Fatalf("unexpected request id: %s", apiErr.RequestID)
	}
}
