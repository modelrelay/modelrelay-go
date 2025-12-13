package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelrelay/modelrelay/platform/headers"
	"github.com/modelrelay/modelrelay/platform/routes"
	llm "github.com/modelrelay/modelrelay/providers"
)

func TestResponsesCreate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.Responses {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get(headers.RequestID); got != "req-123" {
			t.Fatalf("expected request id header got %q", got)
		}
		var reqPayload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if reqPayload["model"] != "demo" {
			t.Fatalf("unexpected model %v", reqPayload["model"])
		}
		if _, ok := reqPayload["input"]; !ok {
			t.Fatalf("missing input")
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set(headers.RequestID, "resp-req-123")
		_ = json.NewEncoder(w).Encode(llm.Response{
			ID:       "resp_123",
			Provider: "openai",
			Model:    "demo",
			Output: []llm.OutputItem{{
				Type:    llm.OutputItemTypeMessage,
				Role:    llm.RoleAssistant,
				Content: []llm.ContentPart{llm.TextPart("hi")},
			}},
			Usage: llm.Usage{TotalTokens: 3},
		})
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	req, opts, err := client.Responses.New().
		Model(NewModelID("demo")).
		MaxOutputTokens(32).
		User("ping").
		RequestID("req-123").
		Build()
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	resp, err := client.Responses.Create(context.Background(), req, opts...)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if resp.ID != "resp_123" {
		t.Fatalf("unexpected id %s", resp.ID)
	}
	if resp.RequestID != "resp-req-123" {
		t.Fatalf("unexpected echoed request id %s", resp.RequestID)
	}
	if got := resp.Text(); got != "hi" {
		t.Fatalf("unexpected text %q", got)
	}
}

func TestResponsesStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.Responses {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get(headers.RequestID) != "stream-req" {
			t.Fatalf("missing request id header")
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set(headers.RequestID, "resp-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(`{"type":"start","request_id":"resp_1","model":"demo"}` + "\n"))
		flusher.Flush()
		_, _ = w.Write([]byte(`{"type":"update","payload":{"content":"Hello"}}` + "\n"))
		flusher.Flush()
		time.Sleep(25 * time.Millisecond)
		_, _ = w.Write([]byte(`{"type":"completion","payload":{"content":"Hello world"},"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}` + "\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, opts, err := client.Responses.New().
		Model(NewModelID("demo")).
		MaxOutputTokens(16).
		User("hi").
		RequestID("stream-req").
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	stream, err := client.Responses.Stream(ctx, req, opts...)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })

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
		t.Fatalf("unexpected usage %+v", event.Usage)
	}

	_ = stream.Close()

	// Collect on a fresh stream handle.
	req2, opts2, err := client.Responses.New().
		Model(NewModelID("demo")).
		MaxOutputTokens(16).
		User("hi").
		RequestID("stream-req").
		Build()
	if err != nil {
		t.Fatalf("build2: %v", err)
	}
	stream2, err := client.Responses.Stream(context.Background(), req2, opts2...)
	if err != nil {
		t.Fatalf("stream2: %v", err)
	}
	resp, err := stream2.Collect(context.Background())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if resp.RequestID != "resp-stream" {
		t.Fatalf("unexpected request id %q", resp.RequestID)
	}
	if got := resp.Text(); got != "Hello world" {
		t.Fatalf("unexpected text %q", got)
	}

	// CollectWithMetrics on a fresh stream handle.
	stream3, err := client.Responses.Stream(context.Background(), req2, opts2...)
	if err != nil {
		t.Fatalf("stream3: %v", err)
	}
	resp3, m, err := stream3.CollectWithMetrics(context.Background())
	if err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	if got := resp3.Text(); got != "Hello world" {
		t.Fatalf("unexpected text %q", got)
	}
	if m.Duration <= 0 {
		t.Fatalf("expected duration > 0, got %s", m.Duration)
	}
	if m.TTFT < 0 || m.TTFT > m.Duration {
		t.Fatalf("expected 0 <= ttft <= duration, got ttft=%s duration=%s", m.TTFT, m.Duration)
	}
}

func TestResponsesCustomerHeaderAllowsMissingModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.Responses {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := strings.TrimSpace(r.Header.Get(headers.CustomerID)); got != "cust_123" {
			t.Fatalf("expected customer header got %q", got)
		}
		var reqPayload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if _, ok := reqPayload["model"]; ok {
			t.Fatalf("expected model omitted for customer request")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llm.Response{ID: "resp_1", Model: "tier-model", Output: nil, Usage: llm.Usage{}})
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	req, opts, err := client.Responses.New().
		CustomerID("cust_123").
		User("hi").
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := client.Responses.Create(context.Background(), req, opts...); err != nil {
		t.Fatalf("create: %v", err)
	}
}
