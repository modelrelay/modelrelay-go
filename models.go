package sdk

import (
	"context"
	"net/http"
	"net/url"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

// CatalogModel is the model catalog entry returned by GET /models.
type CatalogModel = generated.Model

// ListModelsParams configures optional filtering for GET /models.
type ListModelsParams struct {
	Provider   ProviderID
	Capability ModelCapability
}

// ModelsClient provides methods to list active models with rich metadata.
type ModelsClient struct {
	client *Client
}

func (c *ModelsClient) ensureInitialized() error {
	if c == nil || c.client == nil {
		return ConfigError{Reason: "models client not initialized"}
	}
	return nil
}

// List returns active models with rich metadata.
// The underlying endpoint is public (no auth required).
func (c *ModelsClient) List(ctx context.Context, params *ListModelsParams) ([]CatalogModel, error) {
	if err := c.ensureInitialized(); err != nil {
		return nil, err
	}

	path := routes.Models
	if params != nil {
		q := url.Values{}
		if !params.Provider.IsEmpty() {
			q.Set("provider", params.Provider.String())
		}
		if params.Capability != "" {
			q.Set("capability", string(params.Capability))
		}
		if encoded := q.Encode(); encoded != "" {
			path = path + "?" + encoded
		}
	}

	var payload generated.ModelsResponse
	if err := c.client.sendAndDecode(ctx, http.MethodGet, path, nil, &payload); err != nil {
		return nil, err
	}
	if payload.Models == nil {
		return []CatalogModel{}, nil
	}
	return payload.Models, nil
}
