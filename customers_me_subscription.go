package sdk

import (
	"context"
	"net/http"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

// CustomerMeSubscription is the subscription details returned by GET /customers/me/subscription.
type CustomerMeSubscription = generated.CustomerMeSubscription

// MeSubscription returns the authenticated customer's subscription details.
//
// This endpoint requires a customer bearer token. API keys are not accepted.
func (c *CustomersClient) MeSubscription(ctx context.Context) (CustomerMeSubscription, error) {
	if err := c.ensureInitialized(); err != nil {
		return CustomerMeSubscription{}, err
	}

	var payload generated.CustomerMeSubscriptionResponse
	if err := c.client.sendAndDecode(ctx, http.MethodGet, routes.CustomersMeSubscription, nil, &payload); err != nil {
		return CustomerMeSubscription{}, err
	}
	return payload.Subscription, nil
}
