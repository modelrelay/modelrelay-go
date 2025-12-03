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
	ID              uuid.UUID     `json:"id"`
	ProjectID       uuid.UUID     `json:"project_id"`
	TierCode        string        `json:"tier_code"`
	DisplayName     string        `json:"display_name"`
	SpendLimitCents int64         `json:"spend_limit_cents"` // Monthly spend limit in cents (e.g., 2000 = $20.00)
	StripePriceID   string        `json:"stripe_price_id,omitempty"`
	PriceAmount     int64         `json:"price_amount,omitempty"`
	PriceCurrency   string        `json:"price_currency,omitempty"`
	PriceInterval   PriceInterval `json:"price_interval,omitempty"`
	TrialDays       int32         `json:"trial_days,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

// tierListResponse wraps the tier list response.
type tierListResponse struct {
	Tiers []Tier `json:"tiers"`
}

// tierResponse wraps a single tier response.
type tierResponse struct {
	Tier Tier `json:"tier"`
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
	defer resp.Body.Close()
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
	defer resp.Body.Close()
	var payload tierResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Tier{}, err
	}
	return payload.Tier, nil
}
