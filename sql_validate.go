package sdk

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
)

// SQLValidateRequest contains inputs for SQL validation.
type SQLValidateRequest = generated.SQLValidateRequest

// SQLValidateResponse contains the SQL validation result.
type SQLValidateResponse = generated.SQLValidateResponse

// SQLPolicy defines SQL validation policy settings.
type SQLPolicy = generated.SQLPolicy

// SQLClient provides SQL-related API helpers.
type SQLClient struct {
	client *Client
}

func (c *SQLClient) ensureInitialized() error {
	if c == nil || c.client == nil {
		return fmt.Errorf("sdk: sql client not initialized")
	}
	return nil
}

// Validate validates a SQL query against a policy or profile.
func (c *SQLClient) Validate(ctx context.Context, req SQLValidateRequest) (SQLValidateResponse, error) {
	if err := c.ensureInitialized(); err != nil {
		return SQLValidateResponse{}, err
	}
	if strings.TrimSpace(req.Sql) == "" {
		return SQLValidateResponse{}, fmt.Errorf("sdk: sql is required")
	}
	var resp SQLValidateResponse
	if err := c.client.sendAndDecode(ctx, http.MethodPost, "/sql/validate", req, &resp); err != nil {
		return SQLValidateResponse{}, err
	}
	return resp, nil
}
