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

func TestTiersClientListGetCheckout(t *testing.T) {
	tierID := uuid.New()
	projectID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)

	tier := Tier{
		ID:          tierID,
		ProjectID:   projectID,
		TierCode:    NewTierCode("pro"),
		DisplayName: "Pro",
		Models: []TierModel{
			{
				ID:               uuid.New(),
				TierID:           tierID,
				ModelID:          NewModelID("gpt-4o"),
				ModelDisplayName: "GPT-4o",
				IsDefault:        true,
				CreatedAt:        now,
				UpdatedAt:        now,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/tiers" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"tiers": []Tier{tier}})
		case r.URL.Path == "/tiers/"+tierID.String() && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"tier": tier})
		case r.URL.Path == "/tiers/"+tierID.String()+"/checkout" && r.Method == http.MethodPost:
			var req TierCheckoutRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode checkout request: %v", err)
			}
			if req.SuccessURL == "" || req.CancelURL == "" {
				t.Fatalf("unexpected checkout request: %+v", req)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(TierCheckoutSession{SessionID: "sess_tier", URL: "https://stripe.example/tiers"})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_tiers")

	list, err := client.Tiers.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ID != tierID {
		t.Fatalf("unexpected list response: %+v", list)
	}

	got, err := client.Tiers.Get(context.Background(), tierID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.DisplayName != "Pro" {
		t.Fatalf("unexpected tier name %s", got.DisplayName)
	}

	checkout, err := client.Tiers.Checkout(context.Background(), tierID, TierCheckoutRequest{
		SuccessURL: "https://example.com/success",
		CancelURL:  "https://example.com/cancel",
	})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if checkout.SessionID != "sess_tier" {
		t.Fatalf("unexpected checkout session %s", checkout.SessionID)
	}
}

func TestTiersDefaultModel(t *testing.T) {
	tier := Tier{
		Models: []TierModel{{ModelID: NewModelID("gpt-4o"), IsDefault: true}},
	}
	if model, ok := tier.DefaultModel(); !ok || model.String() != "gpt-4o" {
		t.Fatalf("expected default model, got %v %v", model, ok)
	}

	tier.Models = []TierModel{{ModelID: NewModelID("gpt-4o-mini")}}
	if model, ok := tier.DefaultModel(); !ok || model.String() != "gpt-4o-mini" {
		t.Fatalf("expected fallback default model, got %v %v", model, ok)
	}

	tier.Models = []TierModel{
		{ModelID: NewModelID("m1")},
		{ModelID: NewModelID("m2")},
	}
	if _, ok := tier.DefaultModel(); ok {
		t.Fatalf("expected no default model")
	}
}

func TestTiersValidation(t *testing.T) {
	client := &Client{}
	testTierID := uuid.New()

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "Get with nil tier_id",
			fn: func() error {
				_, err := client.Tiers.Get(context.Background(), uuid.Nil)
				return err
			},
		},
		{
			name: "Checkout with empty URLs",
			fn: func() error {
				_, err := client.Tiers.Checkout(context.Background(), testTierID, TierCheckoutRequest{})
				return err
			},
		},
		{
			name: "Checkout with missing success_url",
			fn: func() error {
				_, err := client.Tiers.Checkout(context.Background(), testTierID, TierCheckoutRequest{
					CancelURL: "https://example.com/c",
				})
				return err
			},
		},
		{
			name: "Checkout with missing cancel_url",
			fn: func() error {
				_, err := client.Tiers.Checkout(context.Background(), testTierID, TierCheckoutRequest{
					SuccessURL: "https://example.com/s",
				})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestTiersCheckoutRequiresSecretKey(t *testing.T) {
	pubKey, err := ParsePublishableKey("mr_pk_test")
	if err != nil {
		t.Fatalf("parse publishable key: %v", err)
	}
	pubClient, err := NewClientWithKey(pubKey)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = pubClient.Tiers.Checkout(context.Background(), uuid.New(), TierCheckoutRequest{
		SuccessURL: "https://example.com/s",
		CancelURL:  "https://example.com/c",
	})
	if err == nil {
		t.Fatalf("expected secret key required error")
	}
}
