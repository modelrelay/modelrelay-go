package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

func TestRunsCreateGetAndStream(t *testing.T) {
	runID := NewRunID()
	planHash, err := ParsePlanHash("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("parse plan hash: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case routes.Runs:
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST got %s", r.Method)
			}
			var reqPayload map[string]any
			if decodeErr := json.NewDecoder(r.Body).Decode(&reqPayload); decodeErr != nil {
				t.Fatalf("decode request: %v", decodeErr)
			}
			if _, ok := reqPayload["spec"]; !ok {
				t.Fatalf("missing spec")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(RunsCreateResponse{
				RunID:    runID,
				Status:   RunStatusRunning,
				PlanHash: planHash,
			})
		case "/runs/" + runID.String():
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(RunsGetResponse{
				RunID:       runID,
				Status:      RunStatusRunning,
				PlanHash:    planHash,
				CostSummary: RunCostSummaryV0{LineItems: []RunCostLineItemV0{}},
			})
		case "/runs/" + runID.String() + "/events":
			if r.Header.Get("Accept") != "application/x-ndjson" {
				t.Fatalf("expected NDJSON accept header")
			}
			w.Header().Set("Content-Type", "application/x-ndjson")
			ts := time.Now().UTC().Format(time.RFC3339Nano)
			_, _ = w.Write([]byte(`{"envelope_version":"v0","run_id":"` + runID.String() + `","seq":1,"ts":"` + ts + `","type":"run_started","plan_hash":"` + planHash.String() + `"}` + "\n"))
			_, _ = w.Write([]byte(`{"envelope_version":"v0","run_id":"` + runID.String() + `","seq":2,"ts":"` + ts + `","type":"run_completed","plan_hash":"` + planHash.String() + `","outputs_artifact_key":"` + ArtifactKeyRunOutputsV0 + `","outputs_info":{"bytes":17,"sha256":"` + planHash.String() + `","included":false}}` + "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: mustSecretKey(t, "mr_sk_test_runs"), HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	spec := WorkflowSpecV0{
		Kind: WorkflowKindV0,
		Nodes: []WorkflowNodeV0{
			{
				ID:   "a",
				Type: WorkflowNodeTypeJoinAll,
			},
		},
		Outputs: []WorkflowOutputRefV0{{Name: "result", From: "a"}},
	}

	created, err := client.Runs.Create(context.Background(), spec)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.RunID != runID {
		t.Fatalf("unexpected run id %s", created.RunID)
	}

	got, err := client.Runs.Get(context.Background(), runID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PlanHash != planHash {
		t.Fatalf("unexpected plan hash %s", got.PlanHash.String())
	}

	stream, err := client.Runs.StreamEvents(context.Background(), runID)
	if err != nil {
		t.Fatalf("stream events: %v", err)
	}
	defer stream.Close()

	ev1, ok, err := stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected first event ok=true err=nil got ok=%v err=%v", ok, err)
	}
	started, ok := ev1.(RunEventRunStartedV0)
	if !ok {
		t.Fatalf("expected RunEventRunStartedV0, got %T", ev1)
	}
	if started.EnvelopeVersion != RunEventEnvelopeVersionV0 {
		t.Fatalf("unexpected envelope_version %q", started.EnvelopeVersion)
	}
	if started.PlanHash != planHash {
		t.Fatalf("unexpected plan hash %s", started.PlanHash.String())
	}

	ev2, ok, err := stream.Next()
	if err != nil || !ok {
		t.Fatalf("expected second event ok=true err=nil got ok=%v err=%v", ok, err)
	}
	completed, ok := ev2.(RunEventRunCompletedV0)
	if !ok {
		t.Fatalf("expected RunEventRunCompletedV0, got %T", ev2)
	}
	if completed.EnvelopeVersion != RunEventEnvelopeVersionV0 {
		t.Fatalf("unexpected envelope_version %q", completed.EnvelopeVersion)
	}
	if completed.PlanHash != planHash {
		t.Fatalf("unexpected plan hash %s", completed.PlanHash.String())
	}
	if completed.OutputsArtifactKey != ArtifactKeyRunOutputsV0 {
		t.Fatalf("unexpected outputs_artifact_key %q", completed.OutputsArtifactKey)
	}
	if completed.OutputsInfo.Included {
		t.Fatalf("expected outputs_info.included=false")
	}
}
