package sdk

import (
	"context"
	"fmt"
	"net/http"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
)

// CustomerMe is the customer profile returned by GET /customers/me.
type CustomerMe = generated.CustomerMe

// CustomerMeUsage is the usage metrics for the current billing window.
type CustomerMeUsage = generated.CustomerMeUsage

// CustomerMeSubscription is the subscription details.
type CustomerMeSubscription = generated.CustomerMeSubscription

// CustomerBalanceResponse is the credit balance for PAYGO subscriptions.
type CustomerBalanceResponse = generated.CustomerBalanceResponse

// CustomerLedgerEntry is a single transaction in the balance history.
type CustomerLedgerEntry = generated.CustomerLedgerEntry

// CustomerLedgerResponse contains the balance transaction history.
type CustomerLedgerResponse = generated.CustomerLedgerResponse

// CustomerTopupRequest is the request to create a top-up checkout session.
type CustomerTopupRequest = generated.CustomerTopupRequest

// CustomerTopupResponse is the response from top-up checkout session creation.
type CustomerTopupResponse = generated.CustomerTopupResponse

// ChangeTierRequest is the request to change subscription tier.
type ChangeTierRequest = generated.ChangeTierRequest

// CustomerMeCheckoutRequest is the request to create a subscription checkout session.
type CustomerMeCheckoutRequest = generated.CustomerMeCheckoutRequest

// CheckoutSessionResponse is the response from checkout session creation.
type CheckoutSessionResponse = generated.CheckoutSessionResponse

// BillingClient provides methods for customer self-service billing operations.
//
// These endpoints require a customer bearer token (from OIDC exchange).
// API keys are not accepted.
//
// Example:
//
//	// Get customer info
//	me, err := client.Billing.Me(ctx)
//
//	// Get usage metrics
//	usage, err := client.Billing.Usage(ctx)
//
//	// Get subscription details
//	sub, err := client.Billing.Subscription(ctx)
type BillingClient struct {
	client *Client
}

// ensureInitialized returns an error if the client is not properly initialized.
func (c *BillingClient) ensureInitialized() error {
	if c == nil || c.client == nil {
		return fmt.Errorf("sdk: billing client not initialized")
	}
	return nil
}

// Me returns the authenticated customer's profile.
//
// Returns customer details including ID, email, external ID, and metadata.
//
// Example:
//
//	me, err := client.Billing.Me(ctx)
//	fmt.Println("Customer ID:", me.Customer.Id)
//	fmt.Println("Tier:", me.Tier.Code)
func (c *BillingClient) Me(ctx context.Context) (CustomerMe, error) {
	if err := c.ensureInitialized(); err != nil {
		return CustomerMe{}, err
	}

	var payload generated.CustomerMeResponse
	if err := c.client.sendAndDecode(ctx, http.MethodGet, "/customers/me", nil, &payload); err != nil {
		return CustomerMe{}, err
	}
	return payload.Customer, nil
}

// Subscription returns the authenticated customer's subscription details.
//
// Returns subscription status, tier information, and billing provider.
//
// Example:
//
//	sub, err := client.Billing.Subscription(ctx)
//	fmt.Println("Tier:", sub.TierCode)
//	fmt.Println("Status:", sub.SubscriptionStatus)
func (c *BillingClient) Subscription(ctx context.Context) (CustomerMeSubscription, error) {
	if err := c.ensureInitialized(); err != nil {
		return CustomerMeSubscription{}, err
	}

	var payload generated.CustomerMeSubscriptionResponse
	if err := c.client.sendAndDecode(ctx, http.MethodGet, "/customers/me/subscription", nil, &payload); err != nil {
		return CustomerMeSubscription{}, err
	}
	return payload.Subscription, nil
}

// Usage returns the authenticated customer's usage metrics.
//
// Returns token usage, request counts, and cost for the current billing window.
//
// Example:
//
//	usage, err := client.Billing.Usage(ctx)
//	fmt.Println("Total tokens:", usage.TotalTokens)
//	fmt.Println("Total requests:", usage.TotalRequests)
func (c *BillingClient) Usage(ctx context.Context) (CustomerMeUsage, error) {
	if err := c.ensureInitialized(); err != nil {
		return CustomerMeUsage{}, err
	}

	var payload generated.CustomerMeUsageResponse
	if err := c.client.sendAndDecode(ctx, http.MethodGet, "/customers/me/usage", nil, &payload); err != nil {
		return CustomerMeUsage{}, err
	}
	return payload.Usage, nil
}

// Balance returns the authenticated customer's credit balance.
//
// For PAYGO (pay-as-you-go) subscriptions, returns the current balance
// and reserved amount.
//
// Example:
//
//	balance, err := client.Billing.Balance(ctx)
//	fmt.Println("Balance:", balance.BalanceCents, "cents")
//	fmt.Println("Reserved:", balance.ReservedCents, "cents")
func (c *BillingClient) Balance(ctx context.Context) (CustomerBalanceResponse, error) {
	if err := c.ensureInitialized(); err != nil {
		return CustomerBalanceResponse{}, err
	}

	var payload CustomerBalanceResponse
	if err := c.client.sendAndDecode(ctx, http.MethodGet, "/customers/me/balance", nil, &payload); err != nil {
		return CustomerBalanceResponse{}, err
	}
	return payload, nil
}

// BalanceHistory returns the authenticated customer's balance transaction history.
//
// Returns a list of ledger entries showing credits and debits.
//
// Example:
//
//	history, err := client.Billing.BalanceHistory(ctx)
//	for _, entry := range history.Entries {
//	    fmt.Printf("%s: %d cents (%s)\n", entry.OccurredAt, entry.AmountCents, entry.Reason)
//	}
func (c *BillingClient) BalanceHistory(ctx context.Context) (CustomerLedgerResponse, error) {
	if err := c.ensureInitialized(); err != nil {
		return CustomerLedgerResponse{}, err
	}

	var payload CustomerLedgerResponse
	if err := c.client.sendAndDecode(ctx, http.MethodGet, "/customers/me/balance/history", nil, &payload); err != nil {
		return CustomerLedgerResponse{}, err
	}
	return payload, nil
}

// Topup creates a top-up checkout session.
//
// For PAYGO subscriptions, creates a Stripe Checkout session to add credits.
//
// Example:
//
//	session, err := client.Billing.Topup(ctx, sdk.CustomerTopupRequest{
//	    CreditAmountCents: 1000, // $10.00
//	    SuccessUrl:        "https://myapp.com/billing/success",
//	    CancelUrl:         "https://myapp.com/billing/cancel",
//	})
//	fmt.Println("Checkout URL:", session.CheckoutUrl)
func (c *BillingClient) Topup(ctx context.Context, req CustomerTopupRequest) (CustomerTopupResponse, error) {
	if err := c.ensureInitialized(); err != nil {
		return CustomerTopupResponse{}, err
	}

	var payload CustomerTopupResponse
	if err := c.client.sendAndDecode(ctx, http.MethodPost, "/customers/me/topup", req, &payload); err != nil {
		return CustomerTopupResponse{}, err
	}
	return payload, nil
}

// ChangeTier changes the authenticated customer's subscription tier.
//
// Switches to a different tier within the same project.
//
// Example:
//
//	sub, err := client.Billing.ChangeTier(ctx, "pro")
//	fmt.Println("New tier:", sub.TierCode)
func (c *BillingClient) ChangeTier(ctx context.Context, tierCode string) (CustomerMeSubscription, error) {
	if err := c.ensureInitialized(); err != nil {
		return CustomerMeSubscription{}, err
	}

	req := ChangeTierRequest{TierCode: tierCode}
	var payload generated.CustomerMeSubscriptionResponse
	if err := c.client.sendAndDecode(ctx, http.MethodPost, "/customers/me/change-tier", req, &payload); err != nil {
		return CustomerMeSubscription{}, err
	}
	return payload.Subscription, nil
}

// Checkout creates a subscription checkout session.
//
// Creates a Stripe Checkout session for subscribing to a tier.
//
// Example:
//
//	session, err := client.Billing.Checkout(ctx, sdk.CustomerMeCheckoutRequest{
//	    TierCode:   "pro",
//	    SuccessUrl: "https://myapp.com/billing/success",
//	    CancelUrl:  "https://myapp.com/billing/cancel",
//	})
//	fmt.Println("Checkout URL:", session.Url)
func (c *BillingClient) Checkout(ctx context.Context, req CustomerMeCheckoutRequest) (CheckoutSessionResponse, error) {
	if err := c.ensureInitialized(); err != nil {
		return CheckoutSessionResponse{}, err
	}

	var payload CheckoutSessionResponse
	if err := c.client.sendAndDecode(ctx, http.MethodPost, "/customers/me/checkout", req, &payload); err != nil {
		return CheckoutSessionResponse{}, err
	}
	return payload, nil
}
