package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// APIKey describes the API key payload returned by the SaaS API.
type APIKey struct {
	ID          uuid.UUID  `json:"id"`
	Label       string     `json:"label"`
	Kind        string     `json:"kind"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	RedactedKey string     `json:"redacted_key"`
	SecretKey   string     `json:"secret_key,omitempty"`
}

// APIKeyCreateRequest mirrors POST /api-keys.
type APIKeyCreateRequest struct {
	Label     string     `json:"label"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Kind      string     `json:"kind,omitempty"`
}

// APIKeysClient wraps API key CRUD endpoints.
type APIKeysClient struct {
	client *Client
}

// List returns the API keys for the authenticated tenant.
func (a *APIKeysClient) List(ctx context.Context) ([]APIKey, error) {
	if a == nil || a.client == nil {
		return nil, fmt.Errorf("sdk: api keys client not initialized")
	}
	req, err := a.client.newJSONRequest(ctx, http.MethodGet, "/api-keys", nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.client.send(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		APIKeys []APIKey `json:"api_keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.APIKeys, nil
}

// Create issues a new API key for the authenticated user/project.
func (a *APIKeysClient) Create(ctx context.Context, req APIKeyCreateRequest) (APIKey, error) {
	if a == nil || a.client == nil {
		return APIKey{}, fmt.Errorf("sdk: api keys client not initialized")
	}
	httpReq, err := a.client.newJSONRequest(ctx, http.MethodPost, "/api-keys", req)
	if err != nil {
		return APIKey{}, err
	}
	resp, err := a.client.send(httpReq)
	if err != nil {
		return APIKey{}, err
	}
	defer resp.Body.Close()

	var envelope struct {
		APIKey APIKey `json:"api_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return APIKey{}, err
	}
	return envelope.APIKey, nil
}

// Delete revokes the API key with the provided identifier.
func (a *APIKeysClient) Delete(ctx context.Context, id uuid.UUID) error {
	if a == nil || a.client == nil {
		return fmt.Errorf("sdk: api keys client not initialized")
	}
	if id == uuid.Nil {
		return fmt.Errorf("sdk: api key id required")
	}
	path := fmt.Sprintf("/api-keys/%s", id.String())
	req, err := a.client.newJSONRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	resp, err := a.client.send(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
