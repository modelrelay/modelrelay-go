package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkflowsCompileV0_ValidationErrorRoundTrip(t *testing.T) {
	issuesBytes := readConformanceWorkflowV0FixtureBytes(t, "workflow_v0_invalid_edge_unknown_node.issues.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/workflows/compile" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(issuesBytes)
	}))
	defer srv.Close()

	client, err := NewClientWithKey(
		mustSecretKey(t, "mr_sk_test_workflows"),
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.Workflows.CompileV0(context.Background(), WorkflowSpecV0{Kind: WorkflowKindV0})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	verr, ok := err.(WorkflowValidationError)
	if !ok {
		t.Fatalf("expected WorkflowValidationError, got %T", err)
	}
	gotRaw, marshalErr := json.Marshal(verr)
	if marshalErr != nil {
		t.Fatalf("marshal error: %v", marshalErr)
	}
	if string(canonicalJSON(t, gotRaw)) != string(canonicalJSON(t, issuesBytes)) {
		t.Fatalf("validation error mismatch")
	}
}

func TestWorkflowsCompileV0_SuccessRoundTrip(t *testing.T) {
	planJSON := readConformanceWorkflowV0FixtureBytes(t, "workflow_v0_parallel_agents.plan.json")
	hash := PlanHash("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/workflows/compile" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(WorkflowsCompileResponseV0{
			PlanJSON: planJSON,
			PlanHash: hash,
		})
	}))
	defer srv.Close()

	client, err := NewClientWithKey(
		mustSecretKey(t, "mr_sk_test_workflows"),
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	resp, err := client.Workflows.CompileV0(context.Background(), WorkflowSpecV0{Kind: WorkflowKindV0})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if resp.PlanHash != hash {
		t.Fatalf("unexpected plan_hash %s", resp.PlanHash)
	}
	if string(canonicalJSON(t, resp.PlanJSON)) != string(canonicalJSON(t, planJSON)) {
		t.Fatalf("plan_json mismatch")
	}
}
