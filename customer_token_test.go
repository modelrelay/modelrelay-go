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

func uuidPtr(v uuid.UUID) *uuid.UUID { return &v }

func TestCustomerToken(t *testing.T) {
	issued := CustomerToken{
		Token:              "test_token",
		ExpiresAt:          time.Now().Add(time.Hour).UTC(),
		ExpiresIn:          3600,
		TokenType:          TokenTypeBearer,
		ProjectID:          uuid.New(),
		CustomerID:         uuidPtr(uuid.New()),
		CustomerExternalID: NewCustomerExternalID("customer_123"),
		TierCode:           TierCodePtr("free"),
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

// TestCustomerTokenDecodeWithoutTierCode verifies that CustomerToken can be decoded
// when tier_code is missing from the JSON (BYOB projects without subscriptions).
func TestCustomerTokenDecodeWithoutTierCode(t *testing.T) {
	// JSON without tier_code (BYOB case)
	jsonWithoutTierCode := `{
		"token": "mr_eut_byob_token",
		"expires_at": "2025-01-01T00:00:00Z",
		"expires_in": 3600,
		"token_type": "Bearer",
		"project_id": "00000000-0000-0000-0000-000000000001",
		"customer_id": "00000000-0000-0000-0000-000000000002",
		"customer_external_id": "byob_customer_123"
	}`

	var token CustomerToken
	if err := json.Unmarshal([]byte(jsonWithoutTierCode), &token); err != nil {
		t.Fatalf("failed to decode CustomerToken without tier_code: %v", err)
	}

	if token.Token != "mr_eut_byob_token" {
		t.Fatalf("expected token 'mr_eut_byob_token', got %q", token.Token)
	}
	if token.TierCode != nil {
		t.Fatalf("expected nil TierCode for BYOB, got %v", *token.TierCode)
	}
}

// TestCustomerTokenDecodeWithoutCustomerID verifies that CustomerToken can be decoded
// when customer_id is missing from the JSON (BYOB projects without customers).
func TestCustomerTokenDecodeWithoutCustomerID(t *testing.T) {
	// JSON without customer_id (BYOB case where customers are not created)
	jsonWithoutCustomerID := `{
		"token": "mr_eut_byob_nocustomer",
		"expires_at": "2025-01-01T00:00:00Z",
		"expires_in": 3600,
		"token_type": "Bearer",
		"project_id": "00000000-0000-0000-0000-000000000001",
		"customer_external_id": "byob_external_123"
	}`

	var token CustomerToken
	if err := json.Unmarshal([]byte(jsonWithoutCustomerID), &token); err != nil {
		t.Fatalf("failed to decode CustomerToken without customer_id: %v", err)
	}

	if token.Token != "mr_eut_byob_nocustomer" {
		t.Fatalf("expected token 'mr_eut_byob_nocustomer', got %q", token.Token)
	}
	if token.CustomerID != nil {
		t.Fatalf("expected nil CustomerID for BYOB, got %v", *token.CustomerID)
	}
	if token.TierCode != nil {
		t.Fatalf("expected nil TierCode for BYOB, got %v", *token.TierCode)
	}
}

// TestCustomerTokenDecodeWithTierCode verifies that CustomerToken correctly decodes tier_code when present.
func TestCustomerTokenDecodeWithTierCode(t *testing.T) {
	// JSON with tier_code (standard subscribed customer)
	jsonWithTierCode := `{
		"token": "mr_eut_subscribed_token",
		"expires_at": "2025-01-01T00:00:00Z",
		"expires_in": 3600,
		"token_type": "Bearer",
		"project_id": "00000000-0000-0000-0000-000000000001",
		"customer_id": "00000000-0000-0000-0000-000000000002",
		"customer_external_id": "subscribed_customer_123",
		"tier_code": "pro"
	}`

	var token CustomerToken
	if err := json.Unmarshal([]byte(jsonWithTierCode), &token); err != nil {
		t.Fatalf("failed to decode CustomerToken with tier_code: %v", err)
	}

	if token.Token != "mr_eut_subscribed_token" {
		t.Fatalf("expected token 'mr_eut_subscribed_token', got %q", token.Token)
	}
	if token.TierCode == nil {
		t.Fatalf("expected TierCode to be set, got nil")
	}
	if token.TierCode.String() != "pro" {
		t.Fatalf("expected TierCode 'pro', got %q", token.TierCode.String())
	}
}

func TestCustomerTokenRequestValidation(t *testing.T) {

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

func TestGetOrCreateCustomerToken_PassesTierCode(t *testing.T) {
	issued := CustomerToken{
		Token:              "test_token",
		ExpiresAt:          time.Now().Add(time.Hour).UTC(),
		ExpiresIn:          3600,
		TokenType:          TokenTypeBearer,
		ProjectID:          uuid.New(),
		CustomerID:         uuidPtr(uuid.New()),
		CustomerExternalID: NewCustomerExternalID("customer_123"),
		TierCode:           TierCodePtr("pro"),
	}

	var receivedTierCode string
	mux := http.NewServeMux()
	mux.HandleFunc("/customers", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/auth/customer-token", func(w http.ResponseWriter, r *http.Request) {
		var req CustomerTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		receivedTierCode = string(req.TierCode)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(issued)
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client, err := NewClientWithKey(SecretKey("mr_sk_test"), WithBaseURL(ts.URL))
	if err != nil {
		t.Fatalf("NewClientWithKey: %v", err)
	}

	req := GetOrCreateCustomerTokenRequest{
		ExternalID: NewCustomerExternalID("customer_123"),
		Email:      "test@example.com",
		TierCode:   TierCode("pro"),
	}
	_, err = client.Auth.GetOrCreateCustomerToken(context.Background(), req)
	if err != nil {
		t.Fatalf("GetOrCreateCustomerToken: %v", err)
	}

	if receivedTierCode != "pro" {
		t.Fatalf("expected tier_code 'pro' to be passed to CustomerToken endpoint, got %q", receivedTierCode)
	}
}

func TestGetOrCreateCustomerTokenRequestValidation(t *testing.T) {
	tests := []struct {
		name      string
		req       GetOrCreateCustomerTokenRequest
		wantError bool
		errMsg    string
	}{
		{
			name: "valid request",
			req: GetOrCreateCustomerTokenRequest{
				ExternalID: NewCustomerExternalID("customer_123"),
				Email:      "test@example.com",
				TierCode:   TierCode("free"),
			},
			wantError: false,
		},
		{
			name: "missing external_id",
			req: GetOrCreateCustomerTokenRequest{
				Email:    "test@example.com",
				TierCode: TierCode("free"),
			},
			wantError: true,
			errMsg:    "external_id is required",
		},
		{
			name: "missing email",
			req: GetOrCreateCustomerTokenRequest{
				ExternalID: NewCustomerExternalID("customer_123"),
				TierCode:   TierCode("free"),
			},
			wantError: true,
			errMsg:    "email is required",
		},
		{
			name: "missing tier_code",
			req: GetOrCreateCustomerTokenRequest{
				ExternalID: NewCustomerExternalID("customer_123"),
				Email:      "test@example.com",
			},
			wantError: true,
			errMsg:    "tier_code is required",
		},
		{
			name: "negative ttl",
			req: GetOrCreateCustomerTokenRequest{
				ExternalID: NewCustomerExternalID("customer_123"),
				Email:      "test@example.com",
				TierCode:   TierCode("free"),
				TTLSeconds: -1,
			},
			wantError: true,
			errMsg:    "ttl_seconds must be non-negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Fatalf("expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
