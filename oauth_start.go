package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

// OAuthProvider specifies which OAuth provider to use for customer authentication.
type OAuthProvider string

const (
	// OAuthProviderGitHub uses GitHub OAuth.
	OAuthProviderGitHub OAuthProvider = "github"
	// OAuthProviderGoogle uses Google OAuth.
	OAuthProviderGoogle OAuthProvider = "google"
)

// OAuthStartRequest options for starting an OAuth flow for customer authentication.
type OAuthStartRequest struct {
	// ProjectID is the project to authenticate against.
	ProjectID uuid.UUID
	// Provider is the OAuth provider: "github" or "google".
	Provider OAuthProvider
	// RedirectURI is where to redirect after OAuth completion.
	// Must be in the project's whitelist.
	RedirectURI string
}

// OAuthStartResponse contains the URL to redirect users to for OAuth authentication.
type OAuthStartResponse struct {
	// RedirectURL is the URL to redirect the user to for OAuth authentication.
	RedirectURL string
}

// OAuthStart initiates an OAuth flow for customer authentication.
//
// This starts the OAuth redirect flow where users authenticate with
// GitHub or Google and are redirected back to your application with a
// customer token.
//
// Example:
//
//	resp, err := client.Auth.OAuthStart(ctx, OAuthStartRequest{
//	    ProjectID:   projectID,
//	    Provider:    OAuthProviderGitHub,
//	    RedirectURI: "https://your-app.com/auth/callback",
//	})
//	if err != nil {
//	    return err
//	}
//	// Redirect user to resp.RedirectURL
//	// After OAuth, your callback receives a POST with:
//	// token, token_type, expires_at, expires_in, project_id, customer_id, customer_external_id, tier_code
func (a *AuthClient) OAuthStart(ctx context.Context, req OAuthStartRequest) (OAuthStartResponse, error) {
	if a == nil || a.client == nil {
		return OAuthStartResponse{}, fmt.Errorf("sdk: auth client not initialized")
	}
	if req.ProjectID == uuid.Nil {
		return OAuthStartResponse{}, fmt.Errorf("sdk: project_id is required")
	}
	if req.Provider == "" {
		return OAuthStartResponse{}, fmt.Errorf("sdk: provider is required")
	}
	if req.RedirectURI == "" {
		return OAuthStartResponse{}, fmt.Errorf("sdk: redirect_uri is required")
	}

	body := map[string]string{
		"project_id":   req.ProjectID.String(),
		"provider":     string(req.Provider),
		"redirect_uri": req.RedirectURI,
	}

	httpReq, err := a.client.newJSONRequest(ctx, http.MethodPost, routes.AuthCustomerOAuthStart, body)
	if err != nil {
		return OAuthStartResponse{}, err
	}

	// OAuth start doesn't require authentication - it's a public endpoint
	resp, _, err := a.client.send(httpReq, nil, nil)
	if err != nil {
		return OAuthStartResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	var payload struct {
		RedirectURL string `json:"redirect_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return OAuthStartResponse{}, err
	}

	return OAuthStartResponse{
		RedirectURL: payload.RedirectURL,
	}, nil
}
