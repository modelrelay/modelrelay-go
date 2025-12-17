package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
)

func TestPluginsClient_QuickRunServer(t *testing.T) {
	t.Parallel()

	const runID = "11111111-1111-1111-1111-111111111111"
	const planHash = "0000000000000000000000000000000000000000000000000000000000000000"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/plugins/runs":
			var req generated.PluginsRunRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode /plugins/runs request: %v", err)
			}
			if req.SourceUrl == "" || req.Command == "" || req.UserTask == "" {
				t.Fatalf("unexpected /plugins/runs request: %#v", req)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"run_id":"` + runID + `","status":"running","plan_hash":"` + planHash + `"}`))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/runs/"+runID+"/events":
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = w.Write([]byte(`{"envelope_version":"v0","run_id":"` + runID + `","seq":1,"ts":"2025-12-17T00:00:00Z","type":"run_started","plan_hash":"` + planHash + `"}` + "\n"))
			_, _ = w.Write([]byte(`{"envelope_version":"v0","run_id":"` + runID + `","seq":2,"ts":"2025-12-17T00:00:01Z","type":"run_completed","plan_hash":"` + planHash + `","outputs_artifact_key":"run_outputs.v0","outputs_info":{"bytes":2,"sha256":"00","included":false}}` + "\n"))
			return
		case r.Method == http.MethodGet && r.URL.Path == "/runs/"+runID:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
  "run_id":"` + runID + `",
  "status":"succeeded",
  "plan_hash":"` + planHash + `",
  "cost_summary":{"total_usd_cents":1,"line_items":[]},
  "outputs":{"result":"ok"}
}`))
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

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	res, err := client.Plugins().QuickRunServer(
		ctx,
		"github.com/octo/repo@main/plugins/my",
		"analyze",
		"do the thing",
		WithPluginModel("claude-opus-4-5-20251101"),
	)
	if err != nil {
		t.Fatalf("QuickRunServer: %v", err)
	}
	if res == nil || res.Status != RunStatusSucceeded {
		t.Fatalf("unexpected result: %#v", res)
	}
	var out string
	if err := json.Unmarshal(res.Outputs["result"], &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out != "ok" {
		t.Fatalf("unexpected outputs: %#v", res.Outputs)
	}
}
