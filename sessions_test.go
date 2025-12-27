package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelrelay/modelrelay/sdk/go/generated"
)

var (
	testSessionID  = uuid.MustParse("55555555-5555-5555-5555-555555555555")
	testMessageID  = uuid.MustParse("66666666-6666-6666-6666-666666666666")
	testEndUserID  = uuid.MustParse("77777777-7777-7777-7777-777777777777")
	testSessionTS  = time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
)

func testSession() generated.SessionResponse {
	return generated.SessionResponse{
		Id:           testSessionID,
		ProjectId:    testProjectID,
		EndUserId:    &testEndUserID,
		MessageCount: 5,
		CreatedAt:    testSessionTS,
		UpdatedAt:    testSessionTS,
	}
}

func testSessionMessage() generated.SessionMessageResponse {
	return generated.SessionMessageResponse{
		Id:        testMessageID,
		Role:      "user",
		Content:   []map[string]interface{}{{"type": "text", "text": "Hello!"}},
		Seq:       1,
		CreatedAt: testSessionTS,
	}
}

func TestSessionsCreate(t *testing.T) {
	session := testSession()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var req generated.SessionCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(session)
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_sessions")

	metadata := map[string]interface{}{"project": "test"}
	resp, err := client.Sessions.Create(context.Background(), SessionCreateRequest{
		EndUserId: &testEndUserID,
		Metadata:  &metadata,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if resp.Id != testSessionID {
		t.Fatalf("unexpected session ID %s", resp.Id)
	}
}

func TestSessionsList(t *testing.T) {
	session := testSession()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions" || r.Method != http.MethodGet {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		// Check query params
		if r.URL.Query().Get("limit") != "10" {
			t.Fatalf("expected limit=10, got %s", r.URL.Query().Get("limit"))
		}
		w.Header().Set("Content-Type", "application/json")
		nextCursor := "cursor_abc"
		_ = json.NewEncoder(w).Encode(generated.SessionListResponse{
			Sessions:   []generated.SessionResponse{session},
			NextCursor: &nextCursor,
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_sessions")

	resp, err := client.Sessions.List(context.Background(), ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(resp.Sessions))
	}
	if resp.NextCursor == nil || *resp.NextCursor != "cursor_abc" {
		t.Fatalf("expected next_cursor=cursor_abc")
	}
}

func TestSessionsListWithCursor(t *testing.T) {
	session := testSession()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions" || r.Method != http.MethodGet {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		// Check cursor param
		if r.URL.Query().Get("cursor") != "cursor_abc" {
			t.Fatalf("expected cursor=cursor_abc, got %s", r.URL.Query().Get("cursor"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.SessionListResponse{
			Sessions: []generated.SessionResponse{session},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_sessions")

	resp, err := client.Sessions.List(context.Background(), ListOptions{Cursor: "cursor_abc"})
	if err != nil {
		t.Fatalf("list with cursor: %v", err)
	}
	if len(resp.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(resp.Sessions))
	}
}

func TestSessionsGet(t *testing.T) {
	msg := testSessionMessage()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/sessions/" + testSessionID.String()
		if r.URL.Path != expectedPath || r.Method != http.MethodGet {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.SessionWithMessagesResponse{
			Id:           testSessionID,
			ProjectId:    testProjectID,
			MessageCount: 1,
			Messages:     []generated.SessionMessageResponse{msg},
			CreatedAt:    testSessionTS,
			UpdatedAt:    testSessionTS,
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_sessions")

	resp, err := client.Sessions.Get(context.Background(), testSessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if resp.Id != testSessionID {
		t.Fatalf("unexpected session ID %s", resp.Id)
	}
	if len(resp.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(resp.Messages))
	}
}

func TestSessionsDelete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/sessions/" + testSessionID.String()
		if r.URL.Path != expectedPath || r.Method != http.MethodDelete {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_sessions")

	err := client.Sessions.Delete(context.Background(), testSessionID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestSessionsAddMessage(t *testing.T) {
	msg := testSessionMessage()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/sessions/" + testSessionID.String() + "/messages"
		if r.URL.Path != expectedPath || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var req generated.SessionMessageCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Role != "user" {
			t.Fatalf("expected role=user, got %s", req.Role)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(msg)
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_sessions")

	resp, err := client.Sessions.AddMessage(context.Background(), testSessionID, SessionMessageCreateRequest{
		Role:    "user",
		Content: []map[string]any{{"type": "text", "text": "Hello!"}},
	})
	if err != nil {
		t.Fatalf("add_message: %v", err)
	}
	if resp.Id != testMessageID {
		t.Fatalf("unexpected message ID %s", resp.Id)
	}
}

func TestSessionsAddMessageNormalizesRole(t *testing.T) {
	msg := testSessionMessage()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req generated.SessionMessageCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		// Verify role is trimmed before sending
		if req.Role != "user" {
			t.Fatalf("expected trimmed role 'user', got %q", req.Role)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(msg)
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_sessions")

	// Send role with whitespace
	_, err := client.Sessions.AddMessage(context.Background(), testSessionID, SessionMessageCreateRequest{
		Role:    "  user  ",
		Content: []map[string]any{{"type": "text", "text": "Hello!"}},
	})
	if err != nil {
		t.Fatalf("add_message with whitespace role: %v", err)
	}
}

func TestSessionsAddMessageValidation(t *testing.T) {
	// Use a dummy server that should never be called for validation errors
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be called for validation errors")
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_sessions")

	tests := []struct {
		name    string
		id      uuid.UUID
		req     SessionMessageCreateRequest
		wantErr string
	}{
		{
			name:    "nil session ID",
			id:      uuid.Nil,
			req:     SessionMessageCreateRequest{Role: "user", Content: []map[string]interface{}{{"type": "text"}}},
			wantErr: "session_id is required",
		},
		{
			name:    "empty role",
			id:      testSessionID,
			req:     SessionMessageCreateRequest{Role: "", Content: []map[string]interface{}{{"type": "text"}}},
			wantErr: "role is required",
		},
		{
			name:    "whitespace-only role",
			id:      testSessionID,
			req:     SessionMessageCreateRequest{Role: "   ", Content: []map[string]interface{}{{"type": "text"}}},
			wantErr: "role is required",
		},
		{
			name:    "invalid role",
			id:      testSessionID,
			req:     SessionMessageCreateRequest{Role: "invalid", Content: []map[string]interface{}{{"type": "text"}}},
			wantErr: "invalid role",
		},
		{
			name:    "empty content",
			id:      testSessionID,
			req:     SessionMessageCreateRequest{Role: "user", Content: nil},
			wantErr: "content is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.Sessions.AddMessage(context.Background(), tt.id, tt.req)
			if err == nil {
				t.Fatalf("expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestSessionsGetValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be called for validation errors")
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_sessions")

	_, err := client.Sessions.Get(context.Background(), uuid.Nil)
	if err == nil || !strings.Contains(err.Error(), "session_id is required") {
		t.Fatalf("expected session_id validation error, got %v", err)
	}
}
