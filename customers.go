package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
)

// isValidEmail checks if the given string is a valid email address.
func isValidEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

// CustomerMetadata holds arbitrary customer metadata.
type CustomerMetadata map[string]any

// Customer represents a customer in a ModelRelay project.
type Customer struct {
	ID                   uuid.UUID          `json:"id"`
	ProjectID            uuid.UUID          `json:"project_id"`
	TierID               uuid.UUID          `json:"tier_id"`
	TierCode             TierCode           `json:"tier_code,omitempty"`
	ExternalID           CustomerExternalID `json:"external_id"`
	Email                string             `json:"email"`
	Metadata             CustomerMetadata   `json:"metadata,omitempty"`
	StripeCustomerID     string             `json:"stripe_customer_id,omitempty"`
	StripeSubscriptionID string             `json:"stripe_subscription_id,omitempty"`
	SubscriptionStatus   string             `json:"subscription_status,omitempty"`
	CurrentPeriodStart   *time.Time         `json:"current_period_start,omitempty"`
	CurrentPeriodEnd     *time.Time         `json:"current_period_end,omitempty"`
	CreatedAt            time.Time          `json:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at"`
}

// CustomerCreateRequest contains the fields to create a customer.
type CustomerCreateRequest struct {
	TierID     uuid.UUID          `json:"tier_id"`
	ExternalID CustomerExternalID `json:"external_id"`
	Email      string             `json:"email"`
	Metadata   CustomerMetadata   `json:"metadata,omitempty"`
}

// CustomerUpsertRequest contains the fields to upsert a customer by external_id.
type CustomerUpsertRequest struct {
	TierID     uuid.UUID          `json:"tier_id"`
	ExternalID CustomerExternalID `json:"external_id"`
	Email      string             `json:"email"`
	Metadata   CustomerMetadata   `json:"metadata,omitempty"`
}

// CustomerClaimRequest contains the fields to link an end-user identity to a customer by email.
// Used when a customer subscribes via Stripe Checkout (email only) and later authenticates to the app.
type CustomerClaimRequest struct {
	Email    string                   `json:"email"`
	Provider CustomerIdentityProvider `json:"provider"`
	Subject  CustomerIdentitySubject  `json:"subject"`
}

// CheckoutSessionRequest contains the URLs for checkout redirect.
type CheckoutSessionRequest struct {
	SuccessURL string `json:"success_url"`
	CancelURL  string `json:"cancel_url"`
}

// CheckoutSession represents a Stripe checkout session.
type CheckoutSession struct {
	SessionID string `json:"session_id"`
	URL       string `json:"url"`
}

// SubscriptionStatus represents the subscription status of a customer.
type SubscriptionStatus struct {
	Active             bool   `json:"active"`
	SubscriptionID     string `json:"subscription_id,omitempty"`
	Status             string `json:"status,omitempty"`
	CurrentPeriodStart string `json:"current_period_start,omitempty"`
	CurrentPeriodEnd   string `json:"current_period_end,omitempty"`
}

// customerListResponse wraps the customer list response.
type customerListResponse struct {
	Customers []Customer `json:"customers"`
}

// customerResponse wraps a single customer response.
type customerResponse struct {
	Customer Customer `json:"customer"`
}

// CustomersClient provides methods to manage customers in a project.
// Customer operations require a secret key (mr_sk_*) for authentication.
type CustomersClient struct {
	client *Client
}

// List returns all customers in the project.
func (c *CustomersClient) List(ctx context.Context) ([]Customer, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("sdk: customers client not initialized")
	}
	req, err := c.client.newJSONRequest(ctx, http.MethodGet, "/customers", nil)
	if err != nil {
		return nil, err
	}
	resp, _, err := c.client.send(req, nil, nil)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	var payload customerListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Customers, nil
}

// Create creates a new customer in the project.
func (c *CustomersClient) Create(ctx context.Context, req CustomerCreateRequest) (Customer, error) {
	if c == nil || c.client == nil {
		return Customer{}, fmt.Errorf("sdk: customers client not initialized")
	}
	if req.TierID == uuid.Nil {
		return Customer{}, fmt.Errorf("sdk: tier_id required")
	}
	if req.ExternalID.IsEmpty() {
		return Customer{}, fmt.Errorf("sdk: external_id required")
	}
	if strings.TrimSpace(req.Email) == "" {
		return Customer{}, fmt.Errorf("sdk: email required")
	}
	if !isValidEmail(req.Email) {
		return Customer{}, fmt.Errorf("sdk: invalid email format")
	}
	httpReq, err := c.client.newJSONRequest(ctx, http.MethodPost, "/customers", req)
	if err != nil {
		return Customer{}, err
	}
	resp, _, err := c.client.send(httpReq, nil, nil)
	if err != nil {
		return Customer{}, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	var payload customerResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Customer{}, err
	}
	return payload.Customer, nil
}

// Get retrieves a customer by ID.
func (c *CustomersClient) Get(ctx context.Context, customerID uuid.UUID) (Customer, error) {
	if c == nil || c.client == nil {
		return Customer{}, fmt.Errorf("sdk: customers client not initialized")
	}
	if customerID == uuid.Nil {
		return Customer{}, fmt.Errorf("sdk: customer_id required")
	}
	path := fmt.Sprintf("/customers/%s", customerID.String())
	req, err := c.client.newJSONRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return Customer{}, err
	}
	resp, _, err := c.client.send(req, nil, nil)
	if err != nil {
		return Customer{}, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	var payload customerResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Customer{}, err
	}
	return payload.Customer, nil
}

// Upsert creates or updates a customer by external_id.
// If a customer with the given external_id exists, it is updated.
// Otherwise, a new customer is created.
func (c *CustomersClient) Upsert(ctx context.Context, req CustomerUpsertRequest) (Customer, error) {
	if c == nil || c.client == nil {
		return Customer{}, fmt.Errorf("sdk: customers client not initialized")
	}
	if req.TierID == uuid.Nil {
		return Customer{}, fmt.Errorf("sdk: tier_id required")
	}
	if req.ExternalID.IsEmpty() {
		return Customer{}, fmt.Errorf("sdk: external_id required")
	}
	if strings.TrimSpace(req.Email) == "" {
		return Customer{}, fmt.Errorf("sdk: email required")
	}
	if !isValidEmail(req.Email) {
		return Customer{}, fmt.Errorf("sdk: invalid email format")
	}
	httpReq, err := c.client.newJSONRequest(ctx, http.MethodPut, "/customers", req)
	if err != nil {
		return Customer{}, err
	}
	resp, _, err := c.client.send(httpReq, nil, nil)
	if err != nil {
		return Customer{}, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	var payload customerResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Customer{}, err
	}
	return payload.Customer, nil
}

// Claim links an end-user identity (provider + subject) to a customer found by email.
// Used when a customer subscribes via Stripe Checkout (email only) and later authenticates to the app.
//
// This is a user self-service operation that works with publishable keys,
// allowing CLI tools and frontends to link subscriptions to user identities.
// Works with both publishable keys (mr_pk_*) and secret keys (mr_sk_*).
//
// Returns an error if the customer is not found (404) or the identity is already linked to a different customer (409).
func (c *CustomersClient) Claim(ctx context.Context, req CustomerClaimRequest) (Customer, error) {
	if c == nil || c.client == nil {
		return Customer{}, fmt.Errorf("sdk: customers client not initialized")
	}
	if strings.TrimSpace(req.Email) == "" {
		return Customer{}, fmt.Errorf("sdk: email required")
	}
	if !isValidEmail(req.Email) {
		return Customer{}, fmt.Errorf("sdk: invalid email format")
	}
	if req.Provider.IsEmpty() {
		return Customer{}, fmt.Errorf("sdk: provider required")
	}
	if req.Subject.IsEmpty() {
		return Customer{}, fmt.Errorf("sdk: subject required")
	}
	httpReq, err := c.client.newJSONRequest(ctx, http.MethodPost, "/customers/claim", req)
	if err != nil {
		return Customer{}, err
	}
	resp, _, err := c.client.send(httpReq, nil, nil)
	if err != nil {
		return Customer{}, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	var payload customerResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Customer{}, err
	}
	return payload.Customer, nil
}

// Delete removes a customer by ID.
func (c *CustomersClient) Delete(ctx context.Context, customerID uuid.UUID) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("sdk: customers client not initialized")
	}
	if customerID == uuid.Nil {
		return fmt.Errorf("sdk: customer_id required")
	}
	path := fmt.Sprintf("/customers/%s", customerID.String())
	req, err := c.client.newJSONRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	resp, _, err := c.client.send(req, nil, nil)
	if err != nil {
		return err
	}
	//nolint:errcheck // best-effort cleanup, no meaningful error to return
	_ = resp.Body.Close()
	return nil
}

// CreateCheckoutSession creates a Stripe checkout session for a customer.
func (c *CustomersClient) CreateCheckoutSession(ctx context.Context, customerID uuid.UUID, req CheckoutSessionRequest) (CheckoutSession, error) {
	if c == nil || c.client == nil {
		return CheckoutSession{}, fmt.Errorf("sdk: customers client not initialized")
	}
	if customerID == uuid.Nil {
		return CheckoutSession{}, fmt.Errorf("sdk: customer_id required")
	}
	if strings.TrimSpace(req.SuccessURL) == "" || strings.TrimSpace(req.CancelURL) == "" {
		return CheckoutSession{}, fmt.Errorf("sdk: success_url and cancel_url required")
	}
	path := fmt.Sprintf("/customers/%s/checkout", customerID.String())
	httpReq, err := c.client.newJSONRequest(ctx, http.MethodPost, path, req)
	if err != nil {
		return CheckoutSession{}, err
	}
	resp, _, err := c.client.send(httpReq, nil, nil)
	if err != nil {
		return CheckoutSession{}, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	var payload CheckoutSession
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return CheckoutSession{}, err
	}
	return payload, nil
}

// GetSubscription returns the subscription status for a customer.
func (c *CustomersClient) GetSubscription(ctx context.Context, customerID uuid.UUID) (SubscriptionStatus, error) {
	if c == nil || c.client == nil {
		return SubscriptionStatus{}, fmt.Errorf("sdk: customers client not initialized")
	}
	if customerID == uuid.Nil {
		return SubscriptionStatus{}, fmt.Errorf("sdk: customer_id required")
	}
	path := fmt.Sprintf("/customers/%s/subscription", customerID.String())
	req, err := c.client.newJSONRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return SubscriptionStatus{}, err
	}
	resp, _, err := c.client.send(req, nil, nil)
	if err != nil {
		return SubscriptionStatus{}, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	var payload SubscriptionStatus
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return SubscriptionStatus{}, err
	}
	return payload, nil
}
