package sdk

import (
	"context"
	"fmt"
	"net/http"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

// CustomerMe is the customer self-discovery payload returned by GET /customers/me.
type CustomerMe = generated.CustomerMe

// Me returns the authenticated customer from a customer-scoped bearer token.
//
// This endpoint requires a customer bearer token. API keys are not accepted.
func (c *CustomersClient) Me(ctx context.Context) (CustomerMe, error) {
	if err := c.ensureInitialized(); err != nil {
		return CustomerMe{}, err
	}

	var payload generated.CustomerMeResponse
	if err := c.client.sendAndDecode(ctx, http.MethodGet, routes.CustomersMe, nil, &payload); err != nil {
		return CustomerMe{}, err
	}
	if payload.Customer.Customer.Id == nil {
		return CustomerMe{}, fmt.Errorf("sdk: missing customer in response")
	}
	return payload.Customer, nil
}
