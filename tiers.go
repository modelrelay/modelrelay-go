package sdk

import (
	"context"
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

// TierModel represents per-model pricing within a tier.
type TierModel struct {
	ID                         uuid.UUID         `json:"id"`
	TierID                     uuid.UUID         `json:"tier_id"`
	ModelID                    ModelID           `json:"model_id"`
	ModelDisplayName           string            `json:"model_display_name"`
	Description                string            `json:"description"`
	Capabilities               []ModelCapability `json:"capabilities"`
	ContextWindow              int32             `json:"context_window"`
	MaxOutputTokens            int32             `json:"max_output_tokens"`
	Deprecated                 bool              `json:"deprecated"`
	InputPricePerMillionCents  int64             `json:"input_price_per_million_cents"`
	OutputPricePerMillionCents int64             `json:"output_price_per_million_cents"`
	IsDefault                  bool              `json:"is_default"`
	CreatedAt                  time.Time         `json:"created_at"`
	UpdatedAt                  time.Time         `json:"updated_at"`
}

// Tier represents a pricing tier in a ModelRelay project.
type Tier struct {
	ID               uuid.UUID       `json:"id"`
	ProjectID        uuid.UUID       `json:"project_id"`
	TierCode         TierCode        `json:"tier_code"`
	DisplayName      string          `json:"display_name"`
	SpendLimitCents  int64           `json:"spend_limit_cents"` // Monthly spend limit in cents (e.g., 2000 = $20.00)
	Models           []TierModel     `json:"models"`
	BillingProvider  BillingProvider `json:"billing_provider,omitempty"`
	BillingPriceRef  string          `json:"billing_price_ref,omitempty"`
	PriceAmountCents int64           `json:"price_amount_cents,omitempty"`
	PriceCurrency    string          `json:"price_currency,omitempty"`
	PriceInterval    PriceInterval   `json:"price_interval,omitempty"`
	TrialDays        int32           `json:"trial_days,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// DefaultModel returns the tier's default model, if configured.
func (t Tier) DefaultModel() (ModelID, bool) {
	for i := range t.Models {
		if t.Models[i].IsDefault {
			return t.Models[i].ModelID, true
		}
	}
	if len(t.Models) == 1 {
		return t.Models[0].ModelID, true
	}
	return "", false
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

// ensureInitialized returns an error if the client is not properly initialized.
func (c *TiersClient) ensureInitialized() error {
	if c == nil || c.client == nil {
		return fmt.Errorf("sdk: tiers client not initialized")
	}
	return nil
}

// List returns all tiers in the project.
func (c *TiersClient) List(ctx context.Context) ([]Tier, error) {
	if err := c.ensureInitialized(); err != nil {
		return nil, err
	}
	var payload tierListResponse
	if err := c.client.sendAndDecode(ctx, http.MethodGet, "/tiers", nil, &payload); err != nil {
		return nil, err
	}
	return payload.Tiers, nil
}

// Get retrieves a tier by ID.
func (c *TiersClient) Get(ctx context.Context, tierID uuid.UUID) (Tier, error) {
	if err := c.ensureInitialized(); err != nil {
		return Tier{}, err
	}
	if tierID == uuid.Nil {
		return Tier{}, fmt.Errorf("sdk: tier_id required")
	}
	var payload tierResponse
	if err := c.client.sendAndDecode(ctx, http.MethodGet, fmt.Sprintf("/tiers/%s", tierID.String()), nil, &payload); err != nil {
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
	if err := c.ensureInitialized(); err != nil {
		return TierCheckoutSession{}, err
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
	var session TierCheckoutSession
	if err := c.client.sendAndDecode(ctx, http.MethodPost, fmt.Sprintf("/tiers/%s/checkout", tierID.String()), req, &session); err != nil {
		return TierCheckoutSession{}, err
	}
	return session, nil
}
