package sdk

import (
	"context"
	"fmt"
	"net/http"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
)

// StateHandleCreateRequest contains fields to create a state handle.
type StateHandleCreateRequest = generated.StateHandleCreateRequest

// StateHandleResponse represents a created state handle.
type StateHandleResponse = generated.StateHandleResponse

// MaxStateHandleTTLSeconds is the maximum allowed TTL for a state handle (1 year).
const MaxStateHandleTTLSeconds int64 = 31536000

// StateHandlesClient provides methods for managing state handles.
type StateHandlesClient struct {
	client *Client
}

// ensureInitialized returns an error if the client is not properly initialized.
func (c *StateHandlesClient) ensureInitialized() error {
	if c == nil || c.client == nil {
		return fmt.Errorf("sdk: state handles client not initialized")
	}
	return nil
}

// Create mints a new state handle for /responses tool persistence.
func (c *StateHandlesClient) Create(ctx context.Context, req StateHandleCreateRequest) (StateHandleResponse, error) {
	if err := c.ensureInitialized(); err != nil {
		return StateHandleResponse{}, err
	}
	if req.TtlSeconds != nil {
		if *req.TtlSeconds <= 0 {
			return StateHandleResponse{}, fmt.Errorf("sdk: ttl_seconds must be positive")
		}
		if *req.TtlSeconds > MaxStateHandleTTLSeconds {
			return StateHandleResponse{}, fmt.Errorf("sdk: ttl_seconds exceeds maximum (1 year)")
		}
	}
	var resp StateHandleResponse
	if err := c.client.sendAndDecode(ctx, http.MethodPost, "/state-handles", req, &resp); err != nil {
		return StateHandleResponse{}, err
	}
	return resp, nil
}
