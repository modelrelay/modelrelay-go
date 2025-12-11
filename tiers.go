package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// PriceInterval encodes the billing frequency for a tier.
type PriceInterval string

const (
	PriceIntervalMonth PriceInterval = "month"
	PriceIntervalYear  PriceInterval = "year"
)

// Tier represents a pricing tier in a ModelRelay project.
type Tier struct {
	ID                         uuid.UUID     `json:"id"`
	ProjectID                  uuid.UUID     `json:"project_id"`
	TierCode                   TierCode      `json:"tier_code"`
	DisplayName                string        `json:"display_name"`
	SpendLimitCents            int64         `json:"spend_limit_cents"`             // Monthly spend limit in cents (e.g., 2000 = $20.00)
	InputPricePerMillionCents  int64         `json:"input_price_per_million_cents"` // Input token price in cents per million (e.g., 300 = $3.00/1M)
	OutputPricePerMillionCents int64         `json:"output_price_per_million_cents"`
	StripePriceID              string        `json:"stripe_price_id,omitempty"`
	PriceAmountCents           int64         `json:"price_amount_cents,omitempty"`
	PriceCurrency              string        `json:"price_currency,omitempty"`
	PriceInterval              PriceInterval `json:"price_interval,omitempty"`
	TrialDays                  int32         `json:"trial_days,omitempty"`
	CreatedAt                  time.Time     `json:"created_at"`
	UpdatedAt                  time.Time     `json:"updated_at"`
}

// tierListResponse wraps the tier list response.
type tierListResponse struct {
	Tiers []Tier `json:"tiers"`
}

// tierResponse wraps a single tier response.
type tierResponse struct {
	Tier Tier `json:"tier"`
}

// TierCheckoutRequest contains parameters for creating a tier checkout session.
// Stripe collects the customer's email during checkout.
type TierCheckoutRequest struct {
	SuccessURL string `json:"success_url"`
	CancelURL  string `json:"cancel_url"`
}

// TierCheckoutSession represents a checkout session created from a tier.
type TierCheckoutSession struct {
	SessionID string `json:"session_id"`
	URL       string `json:"url"`
}

// TiersClient provides methods to query tiers in a project.
// Works with both publishable keys (mr_pk_*) and secret keys (mr_sk_*).
type TiersClient struct {
	client *Client
}

// List returns all tiers in the project.
func (c *TiersClient) List(ctx context.Context) ([]Tier, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("sdk: tiers client not initialized")
	}
	req, err := c.client.newJSONRequest(ctx, http.MethodGet, "/tiers", nil)
	if err != nil {
		return nil, err
	}
	resp, _, err := c.client.send(req, nil, nil)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	var payload tierListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Tiers, nil
}

// Get retrieves a tier by ID.
func (c *TiersClient) Get(ctx context.Context, tierID uuid.UUID) (Tier, error) {
	if c == nil || c.client == nil {
		return Tier{}, fmt.Errorf("sdk: tiers client not initialized")
	}
	if tierID == uuid.Nil {
		return Tier{}, fmt.Errorf("sdk: tier_id required")
	}
	path := fmt.Sprintf("/tiers/%s", tierID.String())
	req, err := c.client.newJSONRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return Tier{}, err
	}
	resp, _, err := c.client.send(req, nil, nil)
	if err != nil {
		return Tier{}, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	var payload tierResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Tier{}, err
	}
	return payload.Tier, nil
}

// Checkout creates a Stripe checkout session for a tier (Stripe-first flow).
// This enables users to subscribe before authenticating. Stripe collects the
// customer's email during checkout. After checkout completes, a customer record
// is created with the email from Stripe. The customer can later be linked to
// an identity via CustomersClient.Claim.
//
// Requires a secret key (mr_sk_*).
func (c *TiersClient) Checkout(ctx context.Context, tierID uuid.UUID, req TierCheckoutRequest) (TierCheckoutSession, error) {
	if c == nil || c.client == nil {
		return TierCheckoutSession{}, fmt.Errorf("sdk: tiers client not initialized")
	}
	if !c.client.isSecretKey() {
		return TierCheckoutSession{}, fmt.Errorf("sdk: checkout requires secret key (mr_sk_*)")
	}
	if tierID == uuid.Nil {
		return TierCheckoutSession{}, fmt.Errorf("sdk: tier_id required")
	}
	if req.SuccessURL == "" || req.CancelURL == "" {
		return TierCheckoutSession{}, fmt.Errorf("sdk: success_url and cancel_url required")
	}

	path := fmt.Sprintf("/tiers/%s/checkout", tierID.String())
	httpReq, err := c.client.newJSONRequest(ctx, http.MethodPost, path, req)
	if err != nil {
		return TierCheckoutSession{}, err
	}
	resp, _, err := c.client.send(httpReq, nil, nil)
	if err != nil {
		return TierCheckoutSession{}, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()

	var session TierCheckoutSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return TierCheckoutSession{}, err
	}
	return session, nil
}
