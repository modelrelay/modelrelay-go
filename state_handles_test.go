package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStateHandlesClientCreate(t *testing.T) {
	ttl := int64(3600)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/state-handles" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var req StateHandleCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.TtlSeconds == nil || *req.TtlSeconds != ttl {
			t.Fatalf("expected ttl_seconds %d, got %+v", ttl, req.TtlSeconds)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "id": "550e8400-e29b-41d4-a716-446655440000",
		  "project_id": "11111111-2222-3333-4444-555555555555",
		  "created_at": "2025-01-15T10:30:00.000Z",
		  "expires_at": "2025-01-15T11:30:00.000Z"
		}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_state")

	resp, err := client.StateHandles.Create(context.Background(), StateHandleCreateRequest{
		TtlSeconds: &ttl,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if resp.Id.String() != "550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("unexpected id %s", resp.Id.String())
	}
	if resp.ProjectId.String() != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("unexpected project_id %s", resp.ProjectId.String())
	}
	if resp.ExpiresAt == nil {
		t.Fatalf("expected expires_at")
	}
}

func TestStateHandlesClientValidation(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	client := newTestClient(t, srv, "mr_sk_state")

	t.Run("zero_ttl", func(t *testing.T) {
		ttl := int64(0)
		_, err := client.StateHandles.Create(context.Background(), StateHandleCreateRequest{
			TtlSeconds: &ttl,
		})
		if err == nil {
			t.Fatalf("expected ttl_seconds validation error")
		}
	})

	t.Run("negative_ttl", func(t *testing.T) {
		ttl := int64(-100)
		_, err := client.StateHandles.Create(context.Background(), StateHandleCreateRequest{
			TtlSeconds: &ttl,
		})
		if err == nil {
			t.Fatalf("expected ttl_seconds validation error")
		}
	})

	t.Run("exceeds_max_ttl", func(t *testing.T) {
		ttl := MaxStateHandleTTLSeconds + 1
		_, err := client.StateHandles.Create(context.Background(), StateHandleCreateRequest{
			TtlSeconds: &ttl,
		})
		if err == nil {
			t.Fatalf("expected ttl_seconds max validation error")
		}
	})
}
