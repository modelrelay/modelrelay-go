package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

func TestOIDCExchangeRequestValidate(t *testing.T) {
	if err := (OIDCExchangeRequest{}).Validate(); err == nil {
		t.Fatalf("expected missing id_token error")
	}

	badID := uuid.Nil
	if err := (OIDCExchangeRequest{IDToken: "tok", ProjectID: &badID}).Validate(); err == nil {
		t.Fatalf("expected project_id validation error")
	}
}

func TestAuthClientOIDCExchange(t *testing.T) {
	expiresAt := time.Now().Add(10 * time.Minute).UTC()
	projectID := uuid.New()
	customerID := uuid.New()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.AuthOIDCExchange {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		var req OIDCExchangeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.IDToken != "id-token" {
			t.Fatalf("unexpected id token")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CustomerToken{
			Token:              "customer-token",
			ExpiresAt:          expiresAt,
			ExpiresIn:          600,
			TokenType:          TokenTypeBearer,
			ProjectID:          projectID,
			CustomerID:         customerID,
			CustomerExternalID: NewCustomerExternalID("ext_1"),
			TierCode:           NewTierCode("free"),
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_oidc")

	resp, err := client.Auth.OIDCExchange(context.Background(), OIDCExchangeRequest{IDToken: "id-token"})
	if err != nil {
		t.Fatalf("oidc exchange: %v", err)
	}
	if resp.Token != "customer-token" || resp.CustomerID != customerID {
		t.Fatalf("unexpected token response: %+v", resp)
	}
}
