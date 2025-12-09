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
		Token:              "eyJhbGciOiJI",
		ExpiresAt:          time.Now().Add(30 * time.Minute).UTC(),
		ExpiresIn:          1800,
		TokenType:          TokenTypeBearer,
		KeyID:              uuid.New(),
		SessionID:          uuid.New(),
		ProjectID:          uuid.New(),
		CustomerID:         uuid.New(),
		CustomerExternalID: "cust_123",
		TierCode:           "free",
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

	req := NewFrontendTokenRequest("mr_pk_demo", "customer_123")
	token, err := client.Auth.FrontendToken(context.Background(), req)
	if err != nil {
		t.Fatalf("frontend token: %v", err)
	}
	if token.Token != issued.Token || token.KeyID != issued.KeyID || token.SessionID != issued.SessionID {
		t.Fatalf("unexpected token payload: %#v", token)
	}
}

func TestFrontendTokenRequestValidation(t *testing.T) {
	tests := []struct {
		name      string
		req       FrontendTokenRequest
		wantError bool
		errorMsg  string
	}{
		{
			name:      "valid request",
			req:       NewFrontendTokenRequest("mr_pk_test", "customer_123"),
			wantError: false,
		},
		{
			name:      "empty publishable key",
			req:       FrontendTokenRequest{CustomerID: "customer_123"},
			wantError: true,
			errorMsg:  "publishable_key is required",
		},
		{
			name:      "whitespace-only publishable key",
			req:       FrontendTokenRequest{PublishableKey: "   ", CustomerID: "customer_123"},
			wantError: true,
			errorMsg:  "publishable_key is required",
		},
		{
			name:      "empty customer ID",
			req:       FrontendTokenRequest{PublishableKey: "mr_pk_test"},
			wantError: true,
			errorMsg:  "customer_id is required",
		},
		{
			name:      "whitespace-only customer ID",
			req:       FrontendTokenRequest{PublishableKey: "mr_pk_test", CustomerID: "   "},
			wantError: true,
			errorMsg:  "customer_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantError && err == nil {
				t.Errorf("expected error containing %q, got nil", tt.errorMsg)
			}
			if !tt.wantError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
			if tt.wantError && err != nil && tt.errorMsg != "" {
				if err.Error() != tt.errorMsg {
					t.Errorf("expected error %q, got %q", tt.errorMsg, err.Error())
				}
			}
		})
	}
}

func TestFrontendTokenAutoProvisionRequestValidation(t *testing.T) {
	tests := []struct {
		name      string
		req       FrontendTokenAutoProvisionRequest
		wantError bool
		errorMsg  string
	}{
		{
			name:      "valid request",
			req:       NewFrontendTokenAutoProvisionRequest("mr_pk_test", "customer_123", "test@example.com"),
			wantError: false,
		},
		{
			name:      "empty email",
			req:       FrontendTokenAutoProvisionRequest{PublishableKey: "mr_pk_test", CustomerID: "customer_123"},
			wantError: true,
			errorMsg:  "email is required for auto-provisioning",
		},
		{
			name:      "whitespace-only email",
			req:       FrontendTokenAutoProvisionRequest{PublishableKey: "mr_pk_test", CustomerID: "customer_123", Email: "   "},
			wantError: true,
			errorMsg:  "email is required for auto-provisioning",
		},
		{
			name:      "empty publishable key",
			req:       FrontendTokenAutoProvisionRequest{CustomerID: "customer_123", Email: "test@example.com"},
			wantError: true,
			errorMsg:  "publishable_key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantError && err == nil {
				t.Errorf("expected error containing %q, got nil", tt.errorMsg)
			}
			if !tt.wantError && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
			if tt.wantError && err != nil && tt.errorMsg != "" {
				if err.Error() != tt.errorMsg {
					t.Errorf("expected error %q, got %q", tt.errorMsg, err.Error())
				}
			}
		})
	}
}

func TestFrontendTokenRequestBuilders(t *testing.T) {
	t.Run("WithAutoProvision creates correct type", func(t *testing.T) {
		base := NewFrontendTokenRequest("mr_pk_test", "customer_123")
		autoProvision := base.WithAutoProvision("user@example.com")

		if autoProvision.PublishableKey != "mr_pk_test" {
			t.Errorf("expected PublishableKey 'mr_pk_test', got %q", autoProvision.PublishableKey)
		}
		if autoProvision.CustomerID != "customer_123" {
			t.Errorf("expected CustomerID 'customer_123', got %q", autoProvision.CustomerID)
		}
		if autoProvision.Email != "user@example.com" {
			t.Errorf("expected Email 'user@example.com', got %q", autoProvision.Email)
		}
	})

	t.Run("WithOpts adds optional fields", func(t *testing.T) {
		base := NewFrontendTokenRequest("mr_pk_test", "customer_123")
		withOpts := base.WithOpts(FrontendTokenOpts{
			DeviceID:   "device_abc",
			TTLSeconds: 3600,
		})

		if withOpts.PublishableKey != "mr_pk_test" {
			t.Errorf("expected PublishableKey 'mr_pk_test', got %q", withOpts.PublishableKey)
		}
		if withOpts.DeviceID != "device_abc" {
			t.Errorf("expected DeviceID 'device_abc', got %q", withOpts.DeviceID)
		}
		if withOpts.TTLSeconds != 3600 {
			t.Errorf("expected TTLSeconds 3600, got %d", withOpts.TTLSeconds)
		}
	})

	t.Run("AutoProvision.WithOpts preserves email", func(t *testing.T) {
		base := NewFrontendTokenRequest("mr_pk_test", "customer_123")
		withOpts := base.WithAutoProvision("user@example.com").WithOpts(FrontendTokenOpts{
			DeviceID: "device_xyz",
		})

		if withOpts.Email != "user@example.com" {
			t.Errorf("expected Email 'user@example.com', got %q", withOpts.Email)
		}
		if withOpts.DeviceID != "device_xyz" {
			t.Errorf("expected DeviceID 'device_xyz', got %q", withOpts.DeviceID)
		}
	})
}

func TestFrontendTokenAutoProvision(t *testing.T) {
	issued := FrontendToken{
		Token:              "eyJhbGciOiJI",
		ExpiresAt:          time.Now().Add(30 * time.Minute).UTC(),
		ExpiresIn:          1800,
		TokenType:          TokenTypeBearer,
		KeyID:              uuid.New(),
		SessionID:          uuid.New(),
		ProjectID:          uuid.New(),
		CustomerID:         uuid.New(),
		CustomerExternalID: "cust_123",
		TierCode:           "free",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/frontend-token", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		// Verify email is included in the request
		if req["email"] == nil || req["email"].(string) == "" {
			http.Error(w, `{"error":"email_required","code":"EMAIL_REQUIRED","message":"Email required"}`, http.StatusBadRequest)
			return
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

	req := NewFrontendTokenAutoProvisionRequest("mr_pk_demo", "customer_123", "test@example.com")
	token, err := client.Auth.FrontendTokenAutoProvision(context.Background(), req)
	if err != nil {
		t.Fatalf("frontend token auto provision: %v", err)
	}
	if token.Token != issued.Token {
		t.Fatalf("unexpected token: %#v", token)
	}
}
