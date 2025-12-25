package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelrelay/modelrelay/sdk/go/headers"
	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

func newTestClient(t *testing.T, srv *httptest.Server, key string) *Client {
	t.Helper()
	client, err := NewClientWithKey(
		mustSecretKey(t, key),
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}

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

	client := newTestClient(t, srv, "mr_sk_test")

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

func TestResponsesTextHelper(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.Responses {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var reqPayload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if reqPayload["model"] != "demo" {
			t.Fatalf("unexpected model %v", reqPayload["model"])
		}
		input, ok := reqPayload["input"].([]any)
		if !ok || len(input) < 2 {
			t.Fatalf("expected input messages")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llm.Response{
			ID:    "resp_text",
			Model: "demo",
			Output: []llm.OutputItem{
				{
					Type:    llm.OutputItemTypeMessage,
					Role:    llm.RoleUser,
					Content: []llm.ContentPart{llm.TextPart("ignore")},
				},
				{
					Type:    llm.OutputItemTypeMessage,
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{llm.TextPart("Hello "), llm.TextPart("world")},
				},
			},
			Usage: llm.Usage{TotalTokens: 3},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_test")

	text, err := client.Responses.Text(context.Background(), NewModelID("demo"), "sys", "user")
	if err != nil {
		t.Fatalf("text: %v", err)
	}
	if text != "Hello world" {
		t.Fatalf("unexpected text %q", text)
	}
}

func TestResponsesTextForCustomerOmitsModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.Responses {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := strings.TrimSpace(r.Header.Get(headers.CustomerID)); got != "customer_123" {
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
		_ = json.NewEncoder(w).Encode(llm.Response{
			ID:    "resp_customer_text",
			Model: "tier-model",
			Output: []llm.OutputItem{{
				Type:    llm.OutputItemTypeMessage,
				Role:    llm.RoleAssistant,
				Content: []llm.ContentPart{llm.TextPart("ok")},
			}},
			Usage: llm.Usage{TotalTokens: 2},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_test")

	text, err := client.Responses.TextForCustomer(context.Background(), "customer_123", "sys", "user")
	if err != nil {
		t.Fatalf("text for customer: %v", err)
	}
	if text != "ok" {
		t.Fatalf("unexpected text %q", text)
	}
}

func TestResponsesTextErrorsOnEmptyAssistantText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llm.Response{
			ID:    "resp_empty_text",
			Model: "demo",
			Output: []llm.OutputItem{{
				Type:      llm.OutputItemTypeMessage,
				Role:      llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{{ID: "call_1", Type: llm.ToolTypeFunction}},
			}},
			Usage: llm.Usage{TotalTokens: 1},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_test")

	_, err := client.Responses.Text(context.Background(), NewModelID("demo"), "sys", "user")
	if err == nil {
		t.Fatalf("expected error")
	}
	var te TransportError
	if !errors.As(err, &te) {
		t.Fatalf("expected transport error, got %T", err)
	}
}

func TestResponsesStreamTextDeltas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.Responses {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set(headers.RequestID, "resp-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(`{"type":"start","request_id":"resp_1","model":"demo"}` + "\n"))
		flusher.Flush()
		_, _ = w.Write([]byte(`{"type":"update","delta":"Hello"}` + "\n"))
		flusher.Flush()
		time.Sleep(10 * time.Millisecond)
		_, _ = w.Write([]byte(`{"type":"update","delta":" world"}` + "\n"))
		flusher.Flush()
		_, _ = w.Write([]byte(`{"type":"completion","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}` + "\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_test")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.Responses.StreamTextDeltas(ctx, NewModelID("demo"), "sys", "user")
	if err != nil {
		t.Fatalf("stream text deltas: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })

	first, ok, err := stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected first delta got err=%v ok=%v", err, ok)
	}
	if first != "Hello" {
		t.Fatalf("unexpected delta %q", first)
	}
	second, ok, err := stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected second delta got err=%v ok=%v", err, ok)
	}
	if second != " world" {
		t.Fatalf("unexpected delta %q", second)
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
		_, _ = w.Write([]byte(`{"type":"update","delta":"Hello "}` + "\n"))
		flusher.Flush()
		time.Sleep(25 * time.Millisecond)
		_, _ = w.Write([]byte(`{"type":"update","delta":"world"}` + "\n"))
		flusher.Flush()
		_, _ = w.Write([]byte(`{"type":"completion","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}` + "\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_test")

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
	if err != nil || !ok {
		t.Fatalf("expected second delta event got err=%v ok=%v", err, ok)
	}
	if event.Kind != llm.StreamEventKindMessageDelta {
		t.Fatalf("expected second delta kind, got %s", event.Kind)
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

func TestResponsesStreamRejectsNonNDJSONContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>nope</html>"))
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_test")

	req, opts, err := client.Responses.New().
		Model(NewModelID("demo")).
		User("hi").
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	_, err = client.Responses.Stream(context.Background(), req, opts...)
	if err == nil {
		t.Fatal("expected error")
	}
	var pe StreamProtocolError
	if !errors.As(err, &pe) {
		t.Fatalf("expected StreamProtocolError, got %T", err)
	}
	if pe.ReceivedContentType == "" {
		t.Fatalf("expected received content type to be set")
	}
}

func TestResponsesStreamJSONRejectsNonNDJSONContentType(t *testing.T) {
	type Simple struct {
		Name string `json:"name"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>nope</html>"))
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_test")

	format, err := OutputFormatFromType[Simple]("simple")
	if err != nil {
		t.Fatalf("OutputFormatFromType: %v", err)
	}

	req, opts, err := client.Responses.New().
		Model(NewModelID("demo")).
		User("hi").
		OutputFormat(*format).
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	_, err = StreamJSON[Simple](context.Background(), client.Responses, req, opts...)
	if err == nil {
		t.Fatal("expected error")
	}
	var pe StreamProtocolError
	if !errors.As(err, &pe) {
		t.Fatalf("expected StreamProtocolError, got %T", err)
	}
	if pe.ReceivedContentType == "" {
		t.Fatalf("expected received content type to be set")
	}
}

func TestResponsesStreamTTFTTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(`{"type":"start","request_id":"resp_1","model":"demo"}` + "\n"))
		flusher.Flush()
		time.Sleep(75 * time.Millisecond)
		_, _ = w.Write([]byte(`{"type":"completion","content":"Hello world"}` + "\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_test")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, opts, err := client.Responses.New().
		Model(NewModelID("demo")).
		User("hi").
		StreamTTFTTimeout(25 * time.Millisecond).
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	stream, err := client.Responses.Stream(ctx, req, opts...)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })

	// First event (start) should arrive before TTFT elapses.
	_, ok, err := stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected start event got err=%v ok=%v", err, ok)
	}

	// Next should fail due to TTFT timeout before first content is observed.
	_, _, err = stream.Next()
	if err == nil {
		t.Fatal("expected error")
	}
	var te StreamTimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("expected StreamTimeoutError, got %T", err)
	}
	if te.Kind != StreamTimeoutTTFT {
		t.Fatalf("expected kind=%s got %s", StreamTimeoutTTFT, te.Kind)
	}
}

func TestResponsesStreamIdleTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(`{"type":"start","request_id":"resp_1","model":"demo"}` + "\n"))
		flusher.Flush()
		time.Sleep(75 * time.Millisecond)
		_, _ = w.Write([]byte(`{"type":"completion","content":"Hello world"}` + "\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_test")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, opts, err := client.Responses.New().
		Model(NewModelID("demo")).
		User("hi").
		StreamIdleTimeout(25 * time.Millisecond).
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	stream, err := client.Responses.Stream(ctx, req, opts...)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })

	_, ok, err := stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected start event got err=%v ok=%v", err, ok)
	}

	_, _, err = stream.Next()
	if err == nil {
		t.Fatal("expected error")
	}
	var te StreamTimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("expected StreamTimeoutError, got %T", err)
	}
	if te.Kind != StreamTimeoutIdle {
		t.Fatalf("expected kind=%s got %s", StreamTimeoutIdle, te.Kind)
	}
}

func TestResponsesStreamTotalTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(`{"type":"start","request_id":"resp_1","model":"demo"}` + "\n"))
		flusher.Flush()

		// Keep the connection active so idle doesn't fire.
		for i := 0; i < 20; i++ {
			time.Sleep(10 * time.Millisecond)
			_, _ = w.Write([]byte(`{"type":"keepalive"}` + "\n"))
			flusher.Flush()
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_test")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, opts, err := client.Responses.New().
		Model(NewModelID("demo")).
		User("hi").
		StreamTotalTimeout(35 * time.Millisecond).
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	stream, err := client.Responses.Stream(ctx, req, opts...)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })

	var gotErr error
	for i := 0; i < 50; i++ {
		_, _, err := stream.Next()
		if err != nil {
			gotErr = err
			break
		}
	}
	if gotErr == nil {
		t.Fatal("expected error")
	}
	var te StreamTimeoutError
	if !errors.As(gotErr, &te) {
		t.Fatalf("expected StreamTimeoutError, got %T", gotErr)
	}
	if te.Kind != StreamTimeoutTotal {
		t.Fatalf("expected kind=%s got %s", StreamTimeoutTotal, te.Kind)
	}
}

func TestResponsesStreamJSONTTFTTimeout(t *testing.T) {
	type Simple struct {
		Name string `json:"name"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(`{"type":"start","request_id":"resp_1","model":"demo"}` + "\n"))
		flusher.Flush()
		time.Sleep(75 * time.Millisecond)
		_, _ = w.Write([]byte(`{"type":"completion","payload":{"name":"Jane"}}` + "\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_test")

	format, err := OutputFormatFromType[Simple]("simple")
	if err != nil {
		t.Fatalf("OutputFormatFromType: %v", err)
	}

	req, opts, err := client.Responses.New().
		Model(NewModelID("demo")).
		User("hi").
		OutputFormat(*format).
		StreamTTFTTimeout(25 * time.Millisecond).
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	stream, err := StreamJSON[Simple](context.Background(), client.Responses, req, opts...)
	if err != nil {
		t.Fatalf("stream json: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })

	_, _, err = stream.Next()
	if err == nil {
		t.Fatal("expected error")
	}
	var te StreamTimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("expected StreamTimeoutError, got %T", err)
	}
	if te.Kind != StreamTimeoutTTFT {
		t.Fatalf("expected kind=%s got %s", StreamTimeoutTTFT, te.Kind)
	}
}

func TestResponsesCustomerHeaderAllowsMissingModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.Responses {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := strings.TrimSpace(r.Header.Get(headers.CustomerID)); got != "customer_123" {
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

	client := newTestClient(t, srv, "mr_sk_test")

	req, opts, err := client.Responses.New().
		CustomerID("customer_123").
		User("hi").
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if _, err := client.Responses.Create(context.Background(), req, opts...); err != nil {
		t.Fatalf("create: %v", err)
	}
}

func TestResponsesStreamToolResultParsing(t *testing.T) {
	toolResult := `{"data":[{"url":"https://example.com/image.png"}]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.Responses {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set(headers.RequestID, "resp-tool-result")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(`{"type":"start","request_id":"resp_1","model":"demo"}` + "\n"))
		flusher.Flush()
		_, _ = w.Write([]byte(`{"type":"tool_use_start","tool_call_delta":{"index":0,"id":"call_1","type":"function","function":{"name":"image_generation"}}}` + "\n"))
		flusher.Flush()
		_, _ = w.Write([]byte(`{"type":"tool_use_delta","tool_call_delta":{"index":0,"function":{"arguments":"{\"prompt\":\"hello\"}"}}}` + "\n"))
		flusher.Flush()
		_, _ = w.Write([]byte(`{"type":"tool_use_stop","tool_calls":[{"id":"call_1","type":"function","function":{"name":"image_generation","arguments":"{\"prompt\":\"hello\"}"}}],"tool_result":` + toolResult + `}` + "\n"))
		flusher.Flush()
		_, _ = w.Write([]byte(`{"type":"completion","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}` + "\n"))
		flusher.Flush()
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_test")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, opts, err := client.Responses.New().
		Model(NewModelID("demo")).
		User("hi").
		RequestID("tool-result-test").
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	stream, err := client.Responses.Stream(ctx, req, opts...)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })

	var sawToolUseStart, sawToolUseDelta, sawToolUseStop bool
	var capturedToolResult json.RawMessage

	for {
		event, ok, err := stream.Next()
		if err != nil {
			t.Fatalf("stream error: %v", err)
		}
		if !ok {
			break
		}

		switch event.Kind {
		case llm.StreamEventKindToolUseStart:
			sawToolUseStart = true
			if event.ToolCallDelta == nil {
				t.Error("tool_use_start missing ToolCallDelta")
			} else if event.ToolCallDelta.ID != "call_1" {
				t.Errorf("ToolCallDelta.ID = %q, want %q", event.ToolCallDelta.ID, "call_1")
			}
		case llm.StreamEventKindToolUseDelta:
			sawToolUseDelta = true
			if event.ToolCallDelta == nil {
				t.Error("tool_use_delta missing ToolCallDelta")
			}
		case llm.StreamEventKindToolUseStop:
			sawToolUseStop = true
			if len(event.ToolCalls) != 1 {
				t.Errorf("tool_use_stop has %d tool calls, want 1", len(event.ToolCalls))
			}
			capturedToolResult = event.ToolResult
		default:
			// Ignore other event types (start, delta, stop, ping, etc.)
		}
	}

	if !sawToolUseStart {
		t.Error("did not see tool_use_start event")
	}
	if !sawToolUseDelta {
		t.Error("did not see tool_use_delta event")
	}
	if !sawToolUseStop {
		t.Error("did not see tool_use_stop event")
	}

	// Verify ToolResult was parsed correctly
	if len(capturedToolResult) == 0 {
		t.Fatal("ToolResult was not populated on tool_use_stop event")
	}

	var parsed map[string]any
	if err := json.Unmarshal(capturedToolResult, &parsed); err != nil {
		t.Fatalf("failed to parse ToolResult: %v", err)
	}
	if _, ok := parsed["data"]; !ok {
		t.Error("ToolResult missing 'data' field")
	}
}
