package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	var eventsRequests int32
	var toolsDone atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/runs":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"run_id":"` + runID + `","status":"running","plan_hash":"` + planHash + `"}`))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/runs/"+runID+"/events":
			if q.Get("wait") == "0" {
				t.Fatalf("expected streaming events (wait!=0), got wait=0")
			}
			w.Header().Set("Content-Type", "application/x-ndjson")
			atomic.AddInt32(&eventsRequests, 1)

			afterSeq := int64(0)
			if raw := q.Get("after_seq"); raw != "" {
				v, err := strconv.ParseInt(raw, 10, 64)
				if err != nil {
					t.Fatalf("parse after_seq: %v", err)
				}
				afterSeq = v
			}

			// First stream: emit waiting. After tool-results submission: emit run_completed.
			if !toolsDone.Load() {
				if afterSeq != 0 {
					t.Fatalf("expected initial after_seq=0, got %d", afterSeq)
				}
				_, _ = w.Write([]byte(`{"envelope_version":"v0","run_id":"` + runID + `","seq":1,"ts":"2025-12-17T00:00:00Z","type":"node_waiting","node_id":"n1","waiting":{"step":0,"request_id":"req_1","pending_tool_calls":[{"tool_call_id":"tc_1","name":"bash","arguments":"{\"command\":\"echo hi\"}"},{"tool_call_id":"tc_2","name":"write_file","arguments":"{\"path\":\"x.txt\",\"contents\":\"hi\"}"}],"reason":"client_tool_execution"}}` + "\n"))
				return
			}
			if afterSeq != 1 {
				t.Fatalf("expected resume after_seq=1, got %d", afterSeq)
			}
			_, _ = w.Write([]byte(`{"envelope_version":"v0","run_id":"` + runID + `","seq":2,"ts":"2025-12-17T00:00:01Z","type":"run_completed","plan_hash":"` + planHash + `","outputs_artifact_key":"run_outputs.v0","outputs_info":{"bytes":2,"sha256":"00","included":false}}` + "\n"))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/runs/"+runID:
			w.Header().Set("Content-Type", "application/json")
			if !toolsDone.Load() {
				_, _ = w.Write([]byte(`{
  "run_id":"` + runID + `",
  "status":"running",
  "plan_hash":"` + planHash + `",
  "cost_summary":{"total_usd_cents":1,"line_items":[]}
}`))
				return
			}
			_, _ = w.Write([]byte(`{
  "run_id":"` + runID + `",
  "status":"succeeded",
  "plan_hash":"` + planHash + `",
  "cost_summary":{"total_usd_cents":1,"line_items":[]},
  "outputs":{"result":"\"ok\""}
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
			if len(payload.Results) != 2 {
				t.Fatalf("unexpected tool-results: %#v", payload.Results)
			}
			if !strings.Contains(payload.Results[0].Output, "ok") || !strings.Contains(payload.Results[1].Output, "ok") {
				t.Fatalf("unexpected tool-results: %#v", payload.Results)
			}
			atomic.AddInt32(&toolResultsPosted, 1)
			toolsDone.Store(true)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"accepted":1,"status":"running"}`))
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
	}).Register("write_file", func(args map[string]any, call llm.ToolCall) (any, error) {
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

	res, err := runner.Run(ctx, spec, PluginRunConfig{ToolHandler: registry})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if res == nil || res.Status != RunStatusSucceeded {
		t.Fatalf("unexpected result: %#v", res)
	}
	if atomic.LoadInt32(&toolResultsPosted) != 1 {
		t.Fatalf("expected 1 tool-results post, got %d", toolResultsPosted)
	}
	if atomic.LoadInt32(&eventsRequests) != 2 {
		t.Fatalf("expected 2 event stream requests, got %d", eventsRequests)
	}
}
