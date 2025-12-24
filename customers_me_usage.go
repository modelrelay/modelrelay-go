package sdk

import (
	"context"
	"net/http"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

// CustomerMeUsage is the usage summary returned by GET /customers/me/usage.
type CustomerMeUsage = generated.CustomerMeUsage

// MeUsage returns the authenticated customer's usage metrics for the current billing window.
//
// This endpoint requires a customer bearer token. API keys are not accepted.
func (c *CustomersClient) MeUsage(ctx context.Context) (CustomerMeUsage, error) {
	if err := c.ensureInitialized(); err != nil {
		return CustomerMeUsage{}, err
	}

	var payload generated.CustomerMeUsageResponse
	if err := c.client.sendAndDecode(ctx, http.MethodGet, routes.CustomersMeUsage, nil, &payload); err != nil {
		return CustomerMeUsage{}, err
	}
	return payload.Usage, nil
}
