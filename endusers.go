package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	usagecore "github.com/modelrelay/modelrelay/billingproxy/usage"
)

// EndUserCheckoutRequest mirrors POST /end-users/checkout.
type EndUserCheckoutRequest struct {
	EndUserID string `json:"end_user_id"`
	DeviceID  string `json:"device_id,omitempty"`
	PlanID    string `json:"plan_id,omitempty"`
	Plan      string `json:"plan,omitempty"`
	Success   string `json:"success_url"`
	CancelURL string `json:"cancel_url"`
}

// EndUserRef captures the minimal identity for an end user tied to an owner.
type EndUserRef struct {
	ID         uuid.UUID `json:"id"`
	ExternalID string    `json:"external_id"`
	OwnerID    uuid.UUID `json:"owner_id"`
}

// CheckoutSession represents the Stripe checkout session metadata returned by the API.
type CheckoutSession struct {
	ID          uuid.UUID  `json:"id"`
	Plan        string     `json:"plan"`
	Status      string     `json:"status"`
	URL         string     `json:"url"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// EndUserCheckoutResponse wraps the checkout session + end user identity.
type EndUserCheckoutResponse struct {
	EndUser EndUserRef      `json:"end_user"`
	Session CheckoutSession `json:"session"`
}

// EndUserSubscription describes subscription + usage state for an end user.
type EndUserSubscription struct {
	EndUser            EndUserRef         `json:"end_user"`
	Subscription       *SubscriptionView  `json:"subscription,omitempty"`
	SubscriptionStatus string             `json:"subscription_status,omitempty"`
	Usage              *usagecore.Summary `json:"usage,omitempty"`
	Active             bool               `json:"active"`
}

// EndUserListOptions controls filters + pagination for listing end users.
type EndUserListOptions struct {
	Query    string
	Statuses []string
	Page     int
	PageSize int
}

// EndUserListItem represents a single end-user account with subscription/usage snapshot.
type EndUserListItem struct {
	ID                   uuid.UUID          `json:"id"`
	EndUserID            string             `json:"end_user_id"`
	DeviceID             *string            `json:"device_id,omitempty"`
	StripeCustomerID     *string            `json:"stripe_customer_id,omitempty"`
	StripeSubscriptionID string             `json:"stripe_subscription_id,omitempty"`
	Plan                 string             `json:"plan"`
	Status               string             `json:"status"`
	SubscriptionStatus   string             `json:"subscription_status"`
	CurrentPeriodStartAt *time.Time         `json:"current_period_start_at,omitempty"`
	CurrentPeriodEndAt   *time.Time         `json:"current_period_end_at,omitempty"`
	LastWebhookAt        *time.Time         `json:"last_webhook_at,omitempty"`
	Usage                *usagecore.Summary `json:"usage,omitempty"`
	MonthlyLimit         int64              `json:"monthly_limit,omitempty"`
	PlanID               string             `json:"plan_id,omitempty"`
	PlanName             string             `json:"plan_name,omitempty"`
	PriceID              string             `json:"price_id,omitempty"`
	TrialEndsAt          *time.Time         `json:"trial_ends_at,omitempty"`
	CreatedAt            time.Time          `json:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at"`
}

// EndUserListResponse wraps paginated list results.
type EndUserListResponse struct {
	Items              []EndUserListItem `json:"items"`
	Page               int               `json:"page"`
	PageSize           int               `json:"page_size"`
	Total              int64             `json:"total"`
	HasNext            bool              `json:"has_next"`
	StripeDashboardURL string            `json:"stripe_dashboard_url"`
}

// SubscriptionView normalizes subscription metadata for client consumption.
type SubscriptionView struct {
	ID                   uuid.UUID  `json:"id"`
	Plan                 string     `json:"plan"`
	Status               string     `json:"status"`
	CurrentPeriodStartAt *time.Time `json:"current_period_start_at,omitempty"`
	CurrentPeriodEndAt   *time.Time `json:"current_period_end_at,omitempty"`
	StripeSubscriptionID string     `json:"stripe_subscription_id,omitempty"`
}

// EndUsersClient wraps end-user subscription endpoints.
type EndUsersClient struct {
	client *Client
}

// Checkout creates a Stripe checkout session for the provided end user.
func (e *EndUsersClient) Checkout(ctx context.Context, req EndUserCheckoutRequest) (EndUserCheckoutResponse, error) {
	if e == nil || e.client == nil {
		return EndUserCheckoutResponse{}, fmt.Errorf("sdk: end-users client not initialized")
	}
	if strings.TrimSpace(req.EndUserID) == "" {
		return EndUserCheckoutResponse{}, fmt.Errorf("sdk: end_user_id required")
	}
	if strings.TrimSpace(req.Success) == "" || strings.TrimSpace(req.CancelURL) == "" {
		return EndUserCheckoutResponse{}, fmt.Errorf("sdk: success_url and cancel_url required")
	}
	httpReq, err := e.client.newJSONRequest(ctx, http.MethodPost, "/end-users/checkout", req)
	if err != nil {
		return EndUserCheckoutResponse{}, err
	}
	resp, err := e.client.send(httpReq)
	if err != nil {
		return EndUserCheckoutResponse{}, err
	}
	defer resp.Body.Close()
	var payload EndUserCheckoutResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return EndUserCheckoutResponse{}, err
	}
	return payload, nil
}

// Subscription returns subscription + usage state for an end user.
func (e *EndUsersClient) Subscription(ctx context.Context, endUserID string) (EndUserSubscription, error) {
	if e == nil || e.client == nil {
		return EndUserSubscription{}, fmt.Errorf("sdk: end-users client not initialized")
	}
	if strings.TrimSpace(endUserID) == "" {
		return EndUserSubscription{}, fmt.Errorf("sdk: end_user_id required")
	}
	values := url.Values{}
	values.Set("end_user_id", strings.TrimSpace(endUserID))
	path := "/end-users/subscription?" + values.Encode()
	httpReq, err := e.client.newJSONRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return EndUserSubscription{}, err
	}
	resp, err := e.client.send(httpReq)
	if err != nil {
		return EndUserSubscription{}, err
	}
	defer resp.Body.Close()
	var payload EndUserSubscription
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return EndUserSubscription{}, err
	}
	return payload, nil
}

// ListAccounts returns paginated end-user accounts with subscription + usage snapshots.
func (e *EndUsersClient) ListAccounts(ctx context.Context, opts EndUserListOptions) (EndUserListResponse, error) {
	if e == nil || e.client == nil {
		return EndUserListResponse{}, fmt.Errorf("sdk: end-users client not initialized")
	}
	values := url.Values{}
	if strings.TrimSpace(opts.Query) != "" {
		values.Set("q", strings.TrimSpace(opts.Query))
	}
	if len(opts.Statuses) > 0 {
		values.Set("status", strings.Join(opts.Statuses, ","))
	}
	if opts.Page > 0 {
		values.Set("page", fmt.Sprintf("%d", opts.Page))
	}
	if opts.PageSize > 0 {
		values.Set("page_size", fmt.Sprintf("%d", opts.PageSize))
	}
	path := "/end-users"
	if qs := values.Encode(); qs != "" {
		path = path + "?" + qs
	}
	req, err := e.client.newJSONRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return EndUserListResponse{}, err
	}
	resp, err := e.client.send(req)
	if err != nil {
		return EndUserListResponse{}, err
	}
	defer resp.Body.Close()
	var payload EndUserListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return EndUserListResponse{}, err
	}
	return payload, nil
}

// EndUserPlan captures the Stripe price + quota metadata for an end-user plan.
type EndUserPlan struct {
	ID            uuid.UUID `json:"id"`
	OwnerID       uuid.UUID `json:"owner_id"`
	PlanID        string    `json:"plan_id"`
	Name          string    `json:"name"`
	PriceID       string    `json:"price_id"`
	PriceAmount   int64     `json:"price_amount"`
	PriceCurrency string    `json:"price_currency"`
	PriceInterval string    `json:"price_interval"`
	ManagedPrice  bool      `json:"managed_price"`
	TrialDays     int32     `json:"trial_days"`
	MonthlyLimit  int64     `json:"monthly_limit"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// EndUserPlanRequest mirrors POST /end-users/plans.
type EndUserPlanRequest struct {
	PlanID        string `json:"plan_id"`
	Name          string `json:"name,omitempty"`
	PriceID       string `json:"price_id,omitempty"`
	PriceAmount   int64  `json:"price_amount,omitempty"`
	PriceCurrency string `json:"price_currency,omitempty"`
	PriceInterval string `json:"price_interval,omitempty"`
	ManagedPrice  bool   `json:"managed_price,omitempty"`
	TrialDays     int32  `json:"trial_days,omitempty"`
	MonthlyLimit  int64  `json:"monthly_limit,omitempty"`
}

// ListPlans returns the configured end-user plans for the authenticated owner.
func (e *EndUsersClient) ListPlans(ctx context.Context) ([]EndUserPlan, error) {
	if e == nil || e.client == nil {
		return nil, fmt.Errorf("sdk: end-users client not initialized")
	}
	req, err := e.client.newJSONRequest(ctx, http.MethodGet, "/end-users/plans", nil)
	if err != nil {
		return nil, err
	}
	resp, err := e.client.send(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		Plans []EndUserPlan `json:"plans"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Plans, nil
}

// UpsertPlan creates or updates an end-user plan for the authenticated owner.
func (e *EndUsersClient) UpsertPlan(ctx context.Context, req EndUserPlanRequest) (EndUserPlan, error) {
	if e == nil || e.client == nil {
		return EndUserPlan{}, fmt.Errorf("sdk: end-users client not initialized")
	}
	if strings.TrimSpace(req.PriceID) == "" && req.PriceAmount <= 0 && !req.ManagedPrice {
		return EndUserPlan{}, fmt.Errorf("sdk: price_id or price_amount required")
	}
	httpReq, err := e.client.newJSONRequest(ctx, http.MethodPost, "/end-users/plans", req)
	if err != nil {
		return EndUserPlan{}, err
	}
	resp, err := e.client.send(httpReq)
	if err != nil {
		return EndUserPlan{}, err
	}
	defer resp.Body.Close()
	var payload struct {
		Plan EndUserPlan `json:"plan"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return EndUserPlan{}, err
	}
	return payload.Plan, nil
}
