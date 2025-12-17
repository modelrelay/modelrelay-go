package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

func TestPluginRunner_Run_HandlesClientToolHandoff(t *testing.T) {
	t.Parallel()

	const runID = "11111111-1111-1111-1111-111111111111"
	const planHash = "0000000000000000000000000000000000000000000000000000000000000000"

	var toolResultsPosted int32
	var toolsDone atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/runs":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"run_id":"` + runID + `","status":"waiting","plan_hash":"` + planHash + `"}`))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/runs/"+runID:
			w.Header().Set("Content-Type", "application/json")
			if toolsDone.Load() {
				_, _ = w.Write([]byte(`{
  "run_id":"` + runID + `",
  "status":"succeeded",
  "plan_hash":"` + planHash + `",
  "cost_summary":{"total_usd_cents":1,"line_items":[]},
  "outputs":{"result":"\"ok\""}
}`))
				return
			}
			_, _ = w.Write([]byte(`{
  "run_id":"` + runID + `",
  "status":"waiting",
  "plan_hash":"` + planHash + `",
  "cost_summary":{"total_usd_cents":1,"line_items":[]}
}`))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/runs/"+runID+"/pending-tools":
			w.Header().Set("Content-Type", "application/json")
			if toolsDone.Load() {
				_, _ = w.Write([]byte(`{"run_id":"` + runID + `","pending":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{
  "run_id":"` + runID + `",
  "pending":[
    {"node_id":"n1","step":0,"request_id":"req_1","tool_calls":[
      {"tool_call_id":"tc_1","name":"bash","arguments":"{\"command\":\"echo hi\"}"}
    ]}
  ]
}`))
			return
		case r.Method == http.MethodPost && r.URL.Path == "/runs/"+runID+"/tool-results":
			var payload RunsToolResultsRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode tool-results payload: %v", err)
			}
			if payload.NodeID != "n1" || payload.RequestID != "req_1" || payload.Step != 0 {
				t.Fatalf("unexpected tool-results routing: %#v", payload)
			}
			if len(payload.Results) != 1 || payload.Results[0].Name != "bash" || !strings.Contains(payload.Results[0].Output, "ok") {
				t.Fatalf("unexpected tool-results: %#v", payload.Results)
			}
			atomic.AddInt32(&toolResultsPosted, 1)
			toolsDone.Store(true)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"accepted":1,"status":"running"}`))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/runs/"+runID+"/events":
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.WriteHeader(http.StatusOK)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	t.Cleanup(srv.Close)

	client, err := NewClientWithToken("tok", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("NewClientWithToken: %v", err)
	}

	registry := NewToolRegistry().Register("bash", func(args map[string]any, call llm.ToolCall) (any, error) {
		return "ok", nil
	})

	runner := NewPluginRunner(client)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	spec := &WorkflowSpecV0{
		Kind: WorkflowKindV0,
		Nodes: []WorkflowNodeV0{
			{ID: "n1", Type: WorkflowNodeTypeLLMResponses, Input: json.RawMessage(`{"request":{"model":"x","input":[{"type":"message","role":"system","content":[{"type":"text","text":"x"}]}],"tools":[{"type":"function","function":{"name":"bash","parameters":{}}}]},"tool_execution":{"mode":"client"}}`)},
		},
		Outputs: []WorkflowOutputRefV0{{Name: "result", From: "n1"}},
	}

	res, err := runner.Run(ctx, spec, PluginRunConfig{ToolHandler: registry, PollInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if res == nil || res.Status != RunStatusSucceeded {
		t.Fatalf("unexpected result: %#v", res)
	}
	if atomic.LoadInt32(&toolResultsPosted) != 1 {
		t.Fatalf("expected 1 tool-results post, got %d", toolResultsPosted)
	}
}
