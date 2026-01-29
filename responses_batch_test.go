package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelrelay/modelrelay/sdk/go/headers"
	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

func TestResponsesBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.ResponsesBatch {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get(headers.RequestID); got != "batch-req-123" {
			t.Fatalf("expected request id header got %q", got)
		}
		var reqPayload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&reqPayload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		reqs, ok := reqPayload["requests"].([]any)
		if !ok || len(reqs) != 2 {
			t.Fatalf("expected 2 requests, got %v", reqPayload["requests"])
		}
		opts, ok := reqPayload["options"].(map[string]any)
		if !ok {
			t.Fatalf("missing options")
		}
		if opts["max_concurrent"] != float64(2) {
			t.Fatalf("unexpected max_concurrent %v", opts["max_concurrent"])
		}
		if opts["fail_fast"] != true {
			t.Fatalf("unexpected fail_fast %v", opts["fail_fast"])
		}
		if opts["timeout_ms"] != float64(123) {
			t.Fatalf("unexpected timeout_ms %v", opts["timeout_ms"])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(BatchResponse{
			ID: "batch_123",
			Results: []BatchResult{
				{
					ID:     "req-1",
					Status: BatchStatusSuccess,
					Response: &Response{
						ID:    "resp-1",
						Model: NewModelID("demo"),
						Output: []llm.OutputItem{{
							Type:    llm.OutputItemTypeMessage,
							Role:    llm.RoleAssistant,
							Content: []llm.ContentPart{llm.TextPart("ok")},
						}},
						Usage: Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2},
					},
				},
				{
					ID:     "req-2",
					Status: BatchStatusError,
					Error: &BatchError{
						Status:  502,
						Message: "provider error",
						Code:    "PROVIDER_ERROR",
					},
				},
			},
			Usage: BatchUsage{
				TotalInputTokens:   1,
				TotalOutputTokens:  1,
				TotalRequests:      2,
				SuccessfulRequests: 1,
				FailedRequests:     1,
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_test")

	req1, _, err := client.Responses.New().Model(NewModelID("demo")).User("ping").Build()
	if err != nil {
		t.Fatalf("build req1: %v", err)
	}
	req2, _, err := client.Responses.New().Model(NewModelID("demo")).User("pong").Build()
	if err != nil {
		t.Fatalf("build req2: %v", err)
	}

	resp, err := client.Responses.BatchResponses(
		context.Background(),
		[]BatchRequestItem{
			{ID: "req-1", Request: req1},
			{ID: "req-2", Request: req2},
		},
		WithBatchRequestID("batch-req-123"),
		WithBatchMaxConcurrent(2),
		WithBatchFailFast(true),
		WithBatchItemTimeoutMs(123),
	)
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if resp.ID != "batch_123" {
		t.Fatalf("unexpected batch id %s", resp.ID)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	if resp.Results[0].Status != BatchStatusSuccess || resp.Results[0].Response == nil {
		t.Fatalf("unexpected result[0] %+v", resp.Results[0])
	}
	if resp.Results[1].Status != BatchStatusError || resp.Results[1].Error == nil {
		t.Fatalf("unexpected result[1] %+v", resp.Results[1])
	}
}
