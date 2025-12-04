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

// FrontendTokenRequest represents the payload for POST /auth/frontend-token.
type FrontendTokenRequest struct {
	PublishableKey string `json:"publishable_key"`
	CustomerID     string `json:"customer_id,omitempty"`
	DeviceID       string `json:"device_id,omitempty"`
	TTLSeconds     int64  `json:"ttl_seconds,omitempty"`
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

// FrontendToken exchanges a publishable key for a short-lived bearer token safe for frontend use.
func (a *AuthClient) FrontendToken(ctx context.Context, req FrontendTokenRequest) (FrontendToken, error) {
	if a == nil || a.client == nil {
		return FrontendToken{}, fmt.Errorf("sdk: auth client not initialized")
	}
	if strings.TrimSpace(req.PublishableKey) == "" {
		return FrontendToken{}, fmt.Errorf("sdk: publishable key required")
	}
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
