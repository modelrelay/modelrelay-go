package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestFrontendToken(t *testing.T) {
	issued := FrontendToken{
		Token:      "eyJhbGciOiJI",
		ExpiresAt:  time.Now().Add(30 * time.Minute).UTC(),
		ExpiresIn:  1800,
		TokenType:  "Bearer",
		KeyID:      uuid.New(),
		SessionID:  uuid.New(),
		TokenScope: []string{"frontend"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/frontend-token", func(w http.ResponseWriter, r *http.Request) {
		var req FrontendTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.PublishableKey == "" {
			t.Fatalf("expected publishable key")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issued)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "mr_pk_demo"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	token, err := client.Auth.FrontendToken(context.Background(), FrontendTokenRequest{PublishableKey: "mr_pk_demo"})
	if err != nil {
		t.Fatalf("frontend token: %v", err)
	}
	if token.Token != issued.Token || token.KeyID != issued.KeyID || token.SessionID != issued.SessionID {
		t.Fatalf("unexpected token payload: %#v", token)
	}
}
