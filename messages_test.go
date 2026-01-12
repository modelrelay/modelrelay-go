package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestMessagesClientSend(t *testing.T) {
	messageID := uuid.New()
	projectID := uuid.New()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var req MessageSendRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.To != "agent:reviewer@"+projectID.String() {
			t.Fatalf("unexpected to %q", req.To)
		}
		if req.Subject != "Review" {
			t.Fatalf("unexpected subject %q", req.Subject)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "id": "` + messageID.String() + `",
		  "project_id": "` + projectID.String() + `",
		  "from": "agent:sender@` + projectID.String() + `",
		  "to": "agent:reviewer@` + projectID.String() + `",
		  "subject": "Review",
		  "body": {"ok": true},
		  "thread_id": "` + messageID.String() + `",
		  "read": false,
		  "created_at": "2025-01-15T10:30:00.000Z"
		}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_messages")

	resp, err := client.Messages.Send(context.Background(), MessageSendRequest{
		To:      "agent:reviewer@" + projectID.String(),
		Subject: "Review",
		Body:    map[string]interface{}{"ok": true},
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if resp.Id.String() != messageID.String() {
		t.Fatalf("unexpected message id %s", resp.Id.String())
	}
}

func TestMessagesClientListGetRead(t *testing.T) {
	messageID := "550e8400-e29b-41d4-a716-446655440000"
	projectID := "11111111-2222-3333-4444-555555555555"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/messages" && r.Method == http.MethodGet:
			q := r.URL.Query()
			if q.Get("to") != "agent:reviewer@"+projectID {
				t.Fatalf("unexpected to: %s", q.Get("to"))
			}
			if q.Get("unread") != "true" {
				t.Fatalf("unexpected unread: %s", q.Get("unread"))
			}
			if q.Get("limit") != "1" {
				t.Fatalf("unexpected limit: %s", q.Get("limit"))
			}
			if q.Get("offset") != "5" {
				t.Fatalf("unexpected offset: %s", q.Get("offset"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
			  "messages": [
			    {
			      "id": "` + messageID + `",
			      "project_id": "` + projectID + `",
			      "from": "agent:sender@` + projectID + `",
			      "to": "agent:reviewer@` + projectID + `",
			      "subject": "Review",
			      "body": {"ok": true},
			      "thread_id": "` + messageID + `",
			      "read": false,
			      "created_at": "2025-01-15T10:30:00.000Z"
			    }
			  ],
			  "next_cursor": "6"
			}`))
		case r.URL.Path == "/messages/"+messageID && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
			  "id": "` + messageID + `",
			  "project_id": "` + projectID + `",
			  "from": "agent:sender@` + projectID + `",
			  "to": "agent:reviewer@` + projectID + `",
			  "subject": "Review",
			  "body": {"ok": true},
			  "thread_id": "` + messageID + `",
			  "read": false,
			  "created_at": "2025-01-15T10:30:00.000Z"
			}`))
		case r.URL.Path == "/messages/"+messageID+"/read" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_messages")
	unread := true
	resp, err := client.Messages.List(context.Background(), MessageListOptions{
		To:       "agent:reviewer@" + projectID,
		Unread:   &unread,
		Limit:    1,
		Offset:   5,
		ThreadID: nil,
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(resp.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(resp.Messages))
	}
	if resp.Messages[0].ProjectId.String() != projectID {
		t.Fatalf("unexpected project id %s", resp.Messages[0].ProjectId.String())
	}

	parsedID, err := uuid.Parse(resp.Messages[0].Id.String())
	if err != nil {
		t.Fatalf("parse message id: %v", err)
	}
	if _, err := client.Messages.Get(context.Background(), parsedID); err != nil {
		t.Fatalf("get: %v", err)
	}
	if err := client.Messages.MarkRead(context.Background(), parsedID); err != nil {
		t.Fatalf("mark read: %v", err)
	}
}

func TestMessagesClientValidation(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	client := newTestClient(t, srv, "mr_sk_messages")

	if _, err := client.Messages.Send(context.Background(), MessageSendRequest{}); err == nil {
		t.Fatalf("expected send validation error")
	}
	if _, err := client.Messages.List(context.Background(), MessageListOptions{}); err == nil {
		t.Fatalf("expected list validation error")
	}
	if _, err := client.Messages.List(context.Background(), MessageListOptions{Limit: -1, To: "agent:reviewer@proj"}); err == nil {
		t.Fatalf("expected list limit validation error")
	}
	if _, err := client.Messages.Get(context.Background(), uuid.Nil); err == nil {
		t.Fatalf("expected get validation error")
	}
	if err := client.Messages.MarkRead(context.Background(), uuid.Nil); err == nil {
		t.Fatalf("expected mark read validation error")
	}
}
