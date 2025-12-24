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

func TestCustomerToken(t *testing.T) {
	issued := CustomerToken{
		Token:              "test_token",
		ExpiresAt:          time.Now().Add(time.Hour).UTC(),
		ExpiresIn:          3600,
		TokenType:          TokenTypeBearer,
		ProjectID:          uuid.New(),
		CustomerID:         uuid.New(),
		CustomerExternalID: NewCustomerExternalID("customer_123"),
		TierCode:           NewTierCode("free"),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/customer-token", func(w http.ResponseWriter, r *http.Request) {
		var req CustomerTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := req.Validate(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issued)
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client, err := NewClientWithKey(SecretKey("mr_sk_test"), WithBaseURL(ts.URL))
	if err != nil {
		t.Fatalf("NewClientWithKey: %v", err)
	}

	req := NewCustomerTokenRequestForExternalID(NewCustomerExternalID("customer_123"))
	token, err := client.Auth.CustomerToken(context.Background(), req)
	if err != nil {
		t.Fatalf("CustomerToken: %v", err)
	}
	if token.Token != issued.Token {
		t.Fatalf("expected token %q, got %q", issued.Token, token.Token)
	}
}

func TestCustomerTokenRequestValidation(t *testing.T) {
	uuidPtr := func(v uuid.UUID) *uuid.UUID { return &v }

	tests := []struct {
		name      string
		req       CustomerTokenRequest
		wantError bool
	}{
		{
			name:      "valid external",
			req:       NewCustomerTokenRequestForExternalID(NewCustomerExternalID("customer_123")),
			wantError: false,
		},
		{
			name:      "valid id",
			req:       NewCustomerTokenRequestForCustomerID(uuid.New()),
			wantError: false,
		},
		{
			name:      "missing customer selector",
			req:       CustomerTokenRequest{},
			wantError: true,
		},
		{
			name:      "both selectors set",
			req:       CustomerTokenRequest{CustomerID: uuidPtr(uuid.New()), CustomerExternalID: NewCustomerExternalID("customer_123")},
			wantError: true,
		},
		{
			name:      "negative ttl",
			req:       CustomerTokenRequest{CustomerID: uuidPtr(uuid.New()), TTLSeconds: -1},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantError && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
