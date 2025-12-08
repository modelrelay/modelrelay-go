package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// FrontendTokenRequest is the base request for fetching a frontend token for an existing customer.
// All fields are required. Use NewFrontendTokenRequest to create.
type FrontendTokenRequest struct {
	// PublishableKey (mr_pk_*) - required for authentication.
	PublishableKey string `json:"publishable_key"`
	// CustomerID - required to issue a token for this customer.
	CustomerID string `json:"customer_id"`
}

// NewFrontendTokenRequest creates a request for an existing customer.
// Both publishableKey and customerID are required.
func NewFrontendTokenRequest(publishableKey, customerID string) FrontendTokenRequest {
	return FrontendTokenRequest{
		PublishableKey: publishableKey,
		CustomerID:     customerID,
	}
}

// Validate checks that required fields are set.
func (r FrontendTokenRequest) Validate() error {
	if strings.TrimSpace(r.PublishableKey) == "" {
		return fmt.Errorf("publishable_key is required")
	}
	if strings.TrimSpace(r.CustomerID) == "" {
		return fmt.Errorf("customer_id is required")
	}
	return nil
}

// WithAutoProvision converts this to an auto-provisioning request by adding an email.
// Use this when the customer may not exist and should be created on the free tier.
func (r FrontendTokenRequest) WithAutoProvision(email string) FrontendTokenAutoProvisionRequest {
	return FrontendTokenAutoProvisionRequest{
		PublishableKey: r.PublishableKey,
		CustomerID:     r.CustomerID,
		Email:          email,
	}
}

// WithOpts adds optional configuration to this request.
func (r FrontendTokenRequest) WithOpts(opts FrontendTokenOpts) FrontendTokenRequestWithOpts {
	return FrontendTokenRequestWithOpts{
		PublishableKey: r.PublishableKey,
		CustomerID:     r.CustomerID,
		DeviceID:       opts.DeviceID,
		TTLSeconds:     opts.TTLSeconds,
	}
}

// FrontendTokenAutoProvisionRequest is used when the customer may not exist.
// The email is required for auto-provisioning on the free tier.
// All fields are required. Use NewFrontendTokenAutoProvisionRequest or
// FrontendTokenRequest.WithAutoProvision to create.
type FrontendTokenAutoProvisionRequest struct {
	PublishableKey string `json:"publishable_key"`
	CustomerID     string `json:"customer_id"`
	Email          string `json:"email"`
}

// NewFrontendTokenAutoProvisionRequest creates a request for auto-provisioning a customer.
// All parameters are required.
func NewFrontendTokenAutoProvisionRequest(publishableKey, customerID, email string) FrontendTokenAutoProvisionRequest {
	return FrontendTokenAutoProvisionRequest{
		PublishableKey: publishableKey,
		CustomerID:     customerID,
		Email:          email,
	}
}

// Validate checks that required fields are set.
func (r FrontendTokenAutoProvisionRequest) Validate() error {
	if strings.TrimSpace(r.PublishableKey) == "" {
		return fmt.Errorf("publishable_key is required")
	}
	if strings.TrimSpace(r.CustomerID) == "" {
		return fmt.Errorf("customer_id is required")
	}
	if strings.TrimSpace(r.Email) == "" {
		return fmt.Errorf("email is required for auto-provisioning")
	}
	return nil
}

// WithOpts adds optional configuration to this request.
func (r FrontendTokenAutoProvisionRequest) WithOpts(opts FrontendTokenOpts) FrontendTokenRequestWithOpts {
	return FrontendTokenRequestWithOpts{
		PublishableKey: r.PublishableKey,
		CustomerID:     r.CustomerID,
		Email:          r.Email,
		DeviceID:       opts.DeviceID,
		TTLSeconds:     opts.TTLSeconds,
	}
}

// FrontendTokenOpts contains optional configuration for frontend token requests.
// Use with WithOpts to add these to a request.
type FrontendTokenOpts struct {
	DeviceID   string
	TTLSeconds int64
}

// FrontendTokenRequestWithOpts is the wire format with all fields including options.
type FrontendTokenRequestWithOpts struct {
	PublishableKey string `json:"publishable_key"`
	CustomerID     string `json:"customer_id"`
	Email          string `json:"email,omitempty"`
	DeviceID       string `json:"device_id,omitempty"`
	TTLSeconds     int64  `json:"ttl_seconds,omitempty"`
}

// Validate checks that required fields are set.
func (r FrontendTokenRequestWithOpts) Validate() error {
	if strings.TrimSpace(r.PublishableKey) == "" {
		return fmt.Errorf("publishable_key is required")
	}
	if strings.TrimSpace(r.CustomerID) == "" {
		return fmt.Errorf("customer_id is required")
	}
	return nil
}

// FrontendToken holds the issued bearer token for client-side LLM calls.
type FrontendToken struct {
	Token      string    `json:"token"`
	ExpiresAt  time.Time `json:"expires_at"`
	ExpiresIn  int       `json:"expires_in"`
	TokenType  string    `json:"token_type"`
	KeyID      uuid.UUID `json:"key_id"`
	SessionID  uuid.UUID `json:"session_id"`
	TokenScope []string  `json:"token_scope,omitempty"`
}

// AuthClient wraps authentication-related endpoints.
type AuthClient struct {
	client *Client
}

// FrontendToken exchanges a publishable key for a short-lived bearer token for an existing customer.
func (a *AuthClient) FrontendToken(ctx context.Context, req FrontendTokenRequest) (FrontendToken, error) {
	if a == nil || a.client == nil {
		return FrontendToken{}, fmt.Errorf("sdk: auth client not initialized")
	}
	if err := req.Validate(); err != nil {
		return FrontendToken{}, fmt.Errorf("sdk: %w", err)
	}
	return a.sendFrontendTokenRequest(ctx, req)
}

// FrontendTokenAutoProvision exchanges a publishable key for a token, creating the customer if needed.
// The customer will be auto-provisioned on the project's free tier.
func (a *AuthClient) FrontendTokenAutoProvision(ctx context.Context, req FrontendTokenAutoProvisionRequest) (FrontendToken, error) {
	if a == nil || a.client == nil {
		return FrontendToken{}, fmt.Errorf("sdk: auth client not initialized")
	}
	if err := req.Validate(); err != nil {
		return FrontendToken{}, fmt.Errorf("sdk: %w", err)
	}
	return a.sendFrontendTokenRequest(ctx, req)
}

// FrontendTokenWithOpts exchanges a publishable key for a token with optional configuration.
func (a *AuthClient) FrontendTokenWithOpts(ctx context.Context, req FrontendTokenRequestWithOpts) (FrontendToken, error) {
	if a == nil || a.client == nil {
		return FrontendToken{}, fmt.Errorf("sdk: auth client not initialized")
	}
	if err := req.Validate(); err != nil {
		return FrontendToken{}, fmt.Errorf("sdk: %w", err)
	}
	return a.sendFrontendTokenRequest(ctx, req)
}

func (a *AuthClient) sendFrontendTokenRequest(ctx context.Context, req any) (FrontendToken, error) {
	httpReq, err := a.client.newJSONRequest(ctx, http.MethodPost, "/auth/frontend-token", req)
	if err != nil {
		return FrontendToken{}, err
	}
	resp, _, err := a.client.send(httpReq, nil, nil)
	if err != nil {
		return FrontendToken{}, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	var payload FrontendToken
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return FrontendToken{}, err
	}
	return payload, nil
}
