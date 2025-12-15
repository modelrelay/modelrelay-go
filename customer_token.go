package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// TokenType represents the OAuth2 token type.
type TokenType string

// TokenTypeBearer is the only valid token type for customer tokens.
const TokenTypeBearer TokenType = "Bearer"

// CustomerTokenRequest mints a customer-scoped bearer token (requires secret key auth).
// Exactly one of CustomerID or CustomerExternalID is required.
type CustomerTokenRequest struct {
	ProjectID          uuid.UUID          `json:"project_id"`
	CustomerID         *uuid.UUID         `json:"customer_id,omitempty"`
	CustomerExternalID CustomerExternalID `json:"customer_external_id,omitempty"`
	TTLSeconds         int64              `json:"ttl_seconds,omitempty"`
}

func NewCustomerTokenRequestForCustomerID(projectID uuid.UUID, customerID uuid.UUID) CustomerTokenRequest {
	id := customerID
	return CustomerTokenRequest{ProjectID: projectID, CustomerID: &id}
}

func NewCustomerTokenRequestForExternalID(projectID uuid.UUID, customerExternalID CustomerExternalID) CustomerTokenRequest {
	return CustomerTokenRequest{ProjectID: projectID, CustomerExternalID: customerExternalID}
}

func (r CustomerTokenRequest) Validate() error {
	if r.ProjectID == uuid.Nil {
		return fmt.Errorf("project_id is required")
	}
	hasCustomerID := r.CustomerID != nil && *r.CustomerID != uuid.Nil
	hasExternal := !r.CustomerExternalID.IsEmpty()
	if hasCustomerID == hasExternal {
		return fmt.Errorf("provide exactly one of customer_id or customer_external_id")
	}
	if r.TTLSeconds < 0 {
		return fmt.Errorf("ttl_seconds must be non-negative")
	}
	return nil
}

// CustomerToken holds the issued bearer token for data-plane LLM calls.
type CustomerToken struct {
	Token              string             `json:"token"`
	ExpiresAt          time.Time          `json:"expires_at"`
	ExpiresIn          int                `json:"expires_in"`
	TokenType          TokenType          `json:"token_type"`
	ProjectID          uuid.UUID          `json:"project_id"`
	CustomerID         uuid.UUID          `json:"customer_id"`
	CustomerExternalID CustomerExternalID `json:"customer_external_id"`
	// TierCode is the tier code for the customer (e.g., "free", "pro", "enterprise").
	TierCode TierCode `json:"tier_code"`
}

// AuthClient wraps authentication-related endpoints.
type AuthClient struct {
	client *Client
}

// CustomerToken mints a customer-scoped bearer token (requires secret key auth).
func (a *AuthClient) CustomerToken(ctx context.Context, req CustomerTokenRequest) (CustomerToken, error) {
	if a == nil || a.client == nil {
		return CustomerToken{}, fmt.Errorf("sdk: auth client not initialized")
	}
	if err := req.Validate(); err != nil {
		return CustomerToken{}, fmt.Errorf("sdk: %w", err)
	}
	httpReq, err := a.client.newJSONRequest(ctx, http.MethodPost, "/auth/customer-token", req)
	if err != nil {
		return CustomerToken{}, err
	}
	resp, _, err := a.client.send(httpReq, nil, nil)
	if err != nil {
		return CustomerToken{}, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	var payload CustomerToken
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return CustomerToken{}, err
	}
	return payload, nil
}
