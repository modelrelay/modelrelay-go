package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunsCreate_ReturnsWorkflowValidationError(t *testing.T) {
	issuesBytes := readConformanceWorkflowV0FixtureBytes(t, "workflow_v0_invalid_duplicate_node_id.issues.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/runs" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write(issuesBytes)
	}))
	defer srv.Close()

	client, err := NewClientWithKey(
		mustSecretKey(t, "mr_sk_test_runs"),
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.Runs.Create(context.Background(), WorkflowSpecV0{Kind: WorkflowKindV0})
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
