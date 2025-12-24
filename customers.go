package sdk

import (
	"context"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

// isValidEmail checks if the given string is a valid email address.
func isValidEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

// validateEmail validates an email address and returns a descriptive error if invalid.
func validateEmail(email string) error {
	if strings.TrimSpace(email) == "" {
		return fmt.Errorf("sdk: email required")
	}
	if !isValidEmail(email) {
		return fmt.Errorf("sdk: invalid email format")
	}
	return nil
}

// Customer represents a customer in a ModelRelay project.
type Customer struct {
	ID         uuid.UUID         `json:"id"`
	ProjectID  uuid.UUID         `json:"project_id"`
	ExternalID CustomerExternalID `json:"external_id"`
	Email      string            `json:"email"`
	Metadata   CustomerMetadata   `json:"metadata,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// Subscription represents billing state for a customer.
type Subscription struct {
	ID                   uuid.UUID              `json:"id"`
	ProjectID            uuid.UUID              `json:"project_id"`
	CustomerID           uuid.UUID              `json:"customer_id"`
	TierID               uuid.UUID              `json:"tier_id"`
	TierCode             TierCode               `json:"tier_code,omitempty"`
	StripeCustomerID     string                 `json:"stripe_customer_id,omitempty"`
	StripeSubscriptionID string                 `json:"stripe_subscription_id,omitempty"`
	SubscriptionStatus   SubscriptionStatusKind `json:"subscription_status,omitempty"`
	CurrentPeriodStart   *time.Time             `json:"current_period_start,omitempty"`
	CurrentPeriodEnd     *time.Time             `json:"current_period_end,omitempty"`
	CreatedAt            time.Time              `json:"created_at"`
	UpdatedAt            time.Time              `json:"updated_at"`
}

// CustomerWithSubscription bundles customer identity with optional subscription state.
type CustomerWithSubscription struct {
	Customer      Customer       `json:"customer"`
	Subscription *Subscription `json:"subscription,omitempty"`
}

// CustomerCreateRequest contains the fields to create a customer.
type CustomerCreateRequest struct {
	ExternalID CustomerExternalID `json:"external_id"`
	Email      string            `json:"email"`
	Metadata   CustomerMetadata   `json:"metadata,omitempty"`
}

// CustomerUpsertRequest contains the fields to upsert a customer by external_id.
type CustomerUpsertRequest struct {
	ExternalID CustomerExternalID `json:"external_id"`
	Email      string            `json:"email"`
	Metadata   CustomerMetadata   `json:"metadata,omitempty"`
}

// CustomerClaimRequest contains the fields to link a customer identity to a customer by email.
// Used when a customer subscribes via Stripe Checkout (email only) and later authenticates to the app.
type CustomerClaimRequest struct {
	Email    string                  `json:"email"`
	Provider CustomerIdentityProvider `json:"provider"`
	Subject  CustomerIdentitySubject  `json:"subject"`
}

// CustomerSubscribeRequest contains the checkout parameters for a customer subscription.
type CustomerSubscribeRequest struct {
	TierID     uuid.UUID `json:"tier_id"`
	SuccessURL string    `json:"success_url"`
	CancelURL  string    `json:"cancel_url"`
}

// CheckoutSession represents a Stripe checkout session.
type CheckoutSession struct {
	SessionID string `json:"session_id"`
	URL       string `json:"url"`
}

type customerListResponse struct {
	Customers []CustomerWithSubscription `json:"customers"`
}

type customerResponse struct {
	Customer CustomerWithSubscription `json:"customer"`
}

type customerSubscriptionResponse struct {
	Subscription Subscription `json:"subscription"`
}

// CustomersClient provides methods to manage customers in a project.
// Customer operations require a secret key (mr_sk_*) for authentication.
type CustomersClient struct {
	client *Client
}

// ensureInitialized returns an error if the client is not properly initialized.
func (c *CustomersClient) ensureInitialized() error {
	if c == nil || c.client == nil {
		return fmt.Errorf("sdk: customers client not initialized")
	}
	return nil
}

// List returns all customers in the project.
func (c *CustomersClient) List(ctx context.Context) ([]CustomerWithSubscription, error) {
	if err := c.ensureInitialized(); err != nil {
		return nil, err
	}
	var payload customerListResponse
	if err := c.client.sendAndDecode(ctx, http.MethodGet, routes.Customers, nil, &payload); err != nil {
		return nil, err
	}
	return payload.Customers, nil
}

// Create creates a new customer in the project.
func (c *CustomersClient) Create(ctx context.Context, req CustomerCreateRequest) (CustomerWithSubscription, error) {
	if err := c.ensureInitialized(); err != nil {
		return CustomerWithSubscription{}, err
	}
	if req.ExternalID.IsEmpty() {
		return CustomerWithSubscription{}, fmt.Errorf("sdk: external_id required")
	}
	if err := validateEmail(req.Email); err != nil {
		return CustomerWithSubscription{}, err
	}
	if err := req.Metadata.Validate(); err != nil {
		return CustomerWithSubscription{}, err
	}
	var payload customerResponse
	if err := c.client.sendAndDecode(ctx, http.MethodPost, routes.Customers, req, &payload); err != nil {
		return CustomerWithSubscription{}, err
	}
	return payload.Customer, nil
}

// Get retrieves a customer by ID.
func (c *CustomersClient) Get(ctx context.Context, customerID uuid.UUID) (CustomerWithSubscription, error) {
	if err := c.ensureInitialized(); err != nil {
		return CustomerWithSubscription{}, err
	}
	if customerID == uuid.Nil {
		return CustomerWithSubscription{}, fmt.Errorf("sdk: customer_id required")
	}
	var payload customerResponse
	path := strings.ReplaceAll(routes.CustomersByID, "{customer_id}", url.PathEscape(customerID.String()))
	if err := c.client.sendAndDecode(ctx, http.MethodGet, path, nil, &payload); err != nil {
		return CustomerWithSubscription{}, err
	}
	return payload.Customer, nil
}

// Upsert creates or updates a customer by external_id.
func (c *CustomersClient) Upsert(ctx context.Context, req CustomerUpsertRequest) (CustomerWithSubscription, error) {
	if err := c.ensureInitialized(); err != nil {
		return CustomerWithSubscription{}, err
	}
	if req.ExternalID.IsEmpty() {
		return CustomerWithSubscription{}, fmt.Errorf("sdk: external_id required")
	}
	if err := validateEmail(req.Email); err != nil {
		return CustomerWithSubscription{}, err
	}
	if err := req.Metadata.Validate(); err != nil {
		return CustomerWithSubscription{}, err
	}
	var payload customerResponse
	if err := c.client.sendAndDecode(ctx, http.MethodPut, routes.Customers, req, &payload); err != nil {
		return CustomerWithSubscription{}, err
	}
	return payload.Customer, nil
}

// Claim links a customer identity (provider + subject) to a customer found by email.
// This is a user self-service operation that works with publishable keys.
func (c *CustomersClient) Claim(ctx context.Context, req CustomerClaimRequest) error {
	if err := c.ensureInitialized(); err != nil {
		return err
	}
	if err := validateEmail(req.Email); err != nil {
		return err
	}
	if req.Provider.IsEmpty() {
		return fmt.Errorf("sdk: provider required")
	}
	if req.Subject.IsEmpty() {
		return fmt.Errorf("sdk: subject required")
	}
	return c.client.sendAndDecode(ctx, http.MethodPost, routes.CustomersClaim, req, nil)
}

// Delete removes a customer by ID.
func (c *CustomersClient) Delete(ctx context.Context, customerID uuid.UUID) error {
	if err := c.ensureInitialized(); err != nil {
		return err
	}
	if customerID == uuid.Nil {
		return fmt.Errorf("sdk: customer_id required")
	}
	path := strings.ReplaceAll(routes.CustomersByID, "{customer_id}", url.PathEscape(customerID.String()))
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

// Subscribe creates a Stripe checkout session for a customer.
func (c *CustomersClient) Subscribe(ctx context.Context, customerID uuid.UUID, req CustomerSubscribeRequest) (CheckoutSession, error) {
	if err := c.ensureInitialized(); err != nil {
		return CheckoutSession{}, err
	}
	if customerID == uuid.Nil {
		return CheckoutSession{}, fmt.Errorf("sdk: customer_id required")
	}
	if req.TierID == uuid.Nil {
		return CheckoutSession{}, fmt.Errorf("sdk: tier_id required")
	}
	if strings.TrimSpace(req.SuccessURL) == "" || strings.TrimSpace(req.CancelURL) == "" {
		return CheckoutSession{}, fmt.Errorf("sdk: success_url and cancel_url required")
	}
	var payload CheckoutSession
	path := strings.ReplaceAll(routes.CustomersSubscribe, "{customer_id}", url.PathEscape(customerID.String()))
	if err := c.client.sendAndDecode(ctx, http.MethodPost, path, req, &payload); err != nil {
		return CheckoutSession{}, err
	}
	return payload, nil
}

// GetSubscription returns the subscription details for a customer.
func (c *CustomersClient) GetSubscription(ctx context.Context, customerID uuid.UUID) (Subscription, error) {
	if err := c.ensureInitialized(); err != nil {
		return Subscription{}, err
	}
	if customerID == uuid.Nil {
		return Subscription{}, fmt.Errorf("sdk: customer_id required")
	}
	var payload customerSubscriptionResponse
	path := strings.ReplaceAll(routes.CustomersSubscription, "{customer_id}", url.PathEscape(customerID.String()))
	if err := c.client.sendAndDecode(ctx, http.MethodGet, path, nil, &payload); err != nil {
		return Subscription{}, err
	}
	return payload.Subscription, nil
}

// Unsubscribe cancels a customer's subscription at period end.
func (c *CustomersClient) Unsubscribe(ctx context.Context, customerID uuid.UUID) error {
	if err := c.ensureInitialized(); err != nil {
		return err
	}
	if customerID == uuid.Nil {
		return fmt.Errorf("sdk: customer_id required")
	}
	path := strings.ReplaceAll(routes.CustomersSubscription, "{customer_id}", url.PathEscape(customerID.String()))
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
