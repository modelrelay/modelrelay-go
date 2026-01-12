package sdk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/uuid"
	"github.com/modelrelay/modelrelay/sdk/go/generated"
)

// StateHandleCreateRequest contains fields to create a state handle.
type StateHandleCreateRequest = generated.StateHandleCreateRequest

// StateHandleResponse represents a created state handle.
type StateHandleResponse = generated.StateHandleResponse

// StateHandleListResponse represents a paginated list of state handles.
type StateHandleListResponse = generated.StateHandleListResponse

// MaxStateHandleTTLSeconds is the maximum allowed TTL for a state handle (1 year).
const MaxStateHandleTTLSeconds int64 = 31536000

// MaxStateHandleListLimit caps list pagination.
const MaxStateHandleListLimit int32 = 100

// StateHandleListOptions controls pagination for listing state handles.
type StateHandleListOptions struct {
	Limit  int32
	Offset int32
}

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

// List returns state handles for the current project/customer.
func (c *StateHandlesClient) List(ctx context.Context, opts StateHandleListOptions) (StateHandleListResponse, error) {
	if err := c.ensureInitialized(); err != nil {
		return StateHandleListResponse{}, err
	}
	if opts.Limit < 0 || opts.Offset < 0 {
		return StateHandleListResponse{}, fmt.Errorf("sdk: limit and offset must be non-negative")
	}
	if opts.Limit > MaxStateHandleListLimit {
		return StateHandleListResponse{}, fmt.Errorf("sdk: limit exceeds maximum (%d)", MaxStateHandleListLimit)
	}
	path := "/state-handles"
	params := url.Values{}
	if opts.Limit > 0 {
		params.Set("limit", strconv.FormatInt(int64(opts.Limit), 10))
	}
	if opts.Offset > 0 {
		params.Set("offset", strconv.FormatInt(int64(opts.Offset), 10))
	}
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	var resp StateHandleListResponse
	if err := c.client.sendAndDecode(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return StateHandleListResponse{}, err
	}
	return resp, nil
}

// Delete removes a state handle by ID.
func (c *StateHandlesClient) Delete(ctx context.Context, id uuid.UUID) error {
	if err := c.ensureInitialized(); err != nil {
		return err
	}
	if id == uuid.Nil {
		return fmt.Errorf("sdk: state_id is required")
	}
	path := "/state-handles/" + id.String()
	return c.client.sendAndDecode(ctx, http.MethodDelete, path, nil, nil)
}
