package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

// OIDCExchangeRequest verifies an OIDC id_token and exchanges it for a customer bearer token.
type OIDCExchangeRequest struct {
	IDToken   string     `json:"id_token"`
	ProjectID *uuid.UUID `json:"project_id,omitempty"`
}

func (r OIDCExchangeRequest) Validate() error {
	if strings.TrimSpace(r.IDToken) == "" {
		return fmt.Errorf("id_token is required")
	}
	if r.ProjectID != nil && *r.ProjectID == uuid.Nil {
		return fmt.Errorf("project_id must be non-nil when provided")
	}
	return nil
}

// OIDCExchange verifies an OIDC id_token and exchanges it for a customer bearer token.
// This endpoint accepts either bearer tokens or API keys; typical usage is API key auth.
func (a *AuthClient) OIDCExchange(ctx context.Context, req OIDCExchangeRequest) (CustomerToken, error) {
	if a == nil || a.client == nil {
		return CustomerToken{}, fmt.Errorf("sdk: auth client not initialized")
	}
	if err := req.Validate(); err != nil {
		return CustomerToken{}, fmt.Errorf("sdk: %w", err)
	}
	httpReq, err := a.client.newJSONRequest(ctx, http.MethodPost, routes.AuthOIDCExchange, req)
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

