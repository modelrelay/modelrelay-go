package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelrelay/modelrelay/sdk/go/generated"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// Test fixtures shared across customer tests
var (
	testCustomerID     = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	testProjectID      = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	testTierID         = uuid.MustParse("33333333-3333-3333-3333-333333333333")
	testSubscriptionID = uuid.MustParse("44444444-4444-4444-4444-444444444444")
	testTimestamp      = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
)

func testCustomer() Customer {
	return Customer{
		ID:         testCustomerID,
		ProjectID:  testProjectID,
		ExternalID: NewCustomerExternalID("ext-1"),
		Email:      "user@example.com",
		Metadata:   CustomerMetadata{"plan": CustomerMetadataString("pro")},
		CreatedAt:  testTimestamp,
		UpdatedAt:  testTimestamp,
	}
}

func testSubscription() Subscription {
	return Subscription{
		ID:                 testSubscriptionID,
		ProjectID:          testProjectID,
		CustomerID:         testCustomerID,
		TierID:             testTierID,
		TierCode:           NewTierCode("pro"),
		SubscriptionStatus: SubscriptionStatusActive,
		CreatedAt:          testTimestamp,
		UpdatedAt:          testTimestamp,
	}
}

func TestCustomersList(t *testing.T) {
	customer := testCustomer()
	subscription := testSubscription()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.Customers || r.Method != http.MethodGet {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"customers": []CustomerWithSubscription{{Customer: customer, Subscription: &subscription}},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_customers")

	list, err := client.Customers.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 customer, got %d", len(list))
	}
	if list[0].Customer.ID != testCustomerID {
		t.Fatalf("unexpected customer ID %s", list[0].Customer.ID)
	}
	plan, err := list[0].Customer.Metadata.GetString("plan")
	if err != nil || plan != "pro" {
		t.Fatalf("expected metadata plan=pro, got %q err=%v", plan, err)
	}
}

func TestCustomersCreate(t *testing.T) {
	customer := testCustomer()
	subscription := testSubscription()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.Customers || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var req CustomerCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Email != customer.Email {
			t.Fatalf("unexpected email %s", req.Email)
		}
		if req.ExternalID.String() != customer.ExternalID.String() {
			t.Fatalf("unexpected external_id %s", req.ExternalID)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"customer": CustomerWithSubscription{Customer: customer, Subscription: &subscription},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_customers")

	created, err := client.Customers.Create(context.Background(), CustomerCreateRequest{
		ExternalID: customer.ExternalID,
		Email:      customer.Email,
		Metadata:   customer.Metadata,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Customer.ID != testCustomerID {
		t.Fatalf("unexpected customer ID %s", created.Customer.ID)
	}
}

func TestCustomersGet(t *testing.T) {
	customer := testCustomer()
	customerByIDPath := strings.ReplaceAll(routes.CustomersByID, "{customer_id}", url.PathEscape(testCustomerID.String()))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != customerByIDPath || r.Method != http.MethodGet {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"customer": CustomerWithSubscription{Customer: customer},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_customers")

	fetched, err := client.Customers.Get(context.Background(), testCustomerID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if fetched.Customer.Email != customer.Email {
		t.Fatalf("unexpected email %s", fetched.Customer.Email)
	}
}

func TestCustomersUpsert(t *testing.T) {
	customer := testCustomer()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.Customers || r.Method != http.MethodPut {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var req CustomerUpsertRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.ExternalID.String() != customer.ExternalID.String() {
			t.Fatalf("unexpected external_id %s", req.ExternalID)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"customer": CustomerWithSubscription{Customer: customer},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_customers")

	upserted, err := client.Customers.Upsert(context.Background(), CustomerUpsertRequest{
		ExternalID: customer.ExternalID,
		Email:      customer.Email,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if upserted.Customer.ID != testCustomerID {
		t.Fatalf("unexpected customer ID %s", upserted.Customer.ID)
	}
}

func TestCustomersClaim(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.CustomersClaim || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var req CustomerClaimRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Email != "user@example.com" {
			t.Fatalf("unexpected email %s", req.Email)
		}
		if req.Provider.String() != "oidc" {
			t.Fatalf("unexpected provider %s", req.Provider)
		}
		if req.Subject.String() != "sub-1" {
			t.Fatalf("unexpected subject %s", req.Subject)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_customers")

	err := client.Customers.Claim(context.Background(), CustomerClaimRequest{
		Email:    "user@example.com",
		Provider: NewCustomerIdentityProvider("oidc"),
		Subject:  NewCustomerIdentitySubject("sub-1"),
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
}

func TestCustomersDelete(t *testing.T) {
	customerByIDPath := strings.ReplaceAll(routes.CustomersByID, "{customer_id}", url.PathEscape(testCustomerID.String()))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != customerByIDPath || r.Method != http.MethodDelete {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_customers")

	err := client.Customers.Delete(context.Background(), testCustomerID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestCustomersSubscribe(t *testing.T) {
	subscribePath := strings.ReplaceAll(routes.CustomersSubscribe, "{customer_id}", url.PathEscape(testCustomerID.String()))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != subscribePath || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var req CustomerSubscribeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.TierID != testTierID {
			t.Fatalf("unexpected tier_id %s", req.TierID)
		}
		if req.SuccessURL == "" || req.CancelURL == "" {
			t.Fatalf("missing URLs in request")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(CheckoutSession{
			SessionID: "sess_1",
			URL:       "https://stripe.example/checkout",
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_customers")

	session, err := client.Customers.Subscribe(context.Background(), testCustomerID, CustomerSubscribeRequest{
		TierID:     testTierID,
		SuccessURL: "https://example.com/success",
		CancelURL:  "https://example.com/cancel",
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if session.SessionID != "sess_1" {
		t.Fatalf("unexpected session_id %s", session.SessionID)
	}
}

func TestCustomersGetSubscription(t *testing.T) {
	subscription := testSubscription()
	subscriptionPath := strings.ReplaceAll(routes.CustomersSubscription, "{customer_id}", url.PathEscape(testCustomerID.String()))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != subscriptionPath || r.Method != http.MethodGet {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"subscription": subscription})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_customers")

	sub, err := client.Customers.GetSubscription(context.Background(), testCustomerID)
	if err != nil {
		t.Fatalf("get subscription: %v", err)
	}
	if sub.ID != testSubscriptionID {
		t.Fatalf("unexpected subscription ID %s", sub.ID)
	}
}

func TestCustomersUnsubscribe(t *testing.T) {
	subscriptionPath := strings.ReplaceAll(routes.CustomersSubscription, "{customer_id}", url.PathEscape(testCustomerID.String()))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != subscriptionPath || r.Method != http.MethodDelete {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_customers")

	err := client.Customers.Unsubscribe(context.Background(), testCustomerID)
	if err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
}

func TestCustomersMe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.CustomersMe || r.Method != http.MethodGet {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		custID := openapi_types.UUID(testCustomerID)
		projID := openapi_types.UUID(testProjectID)
		externalID := "ext-1"
		email := openapi_types.Email("user@example.com")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.CustomerMeResponse{
			Customer: generated.CustomerMe{
				Customer: generated.Customer{
					Id:         &custID,
					ProjectId:  &projID,
					ExternalId: &externalID,
					Email:      &email,
					CreatedAt:  &testTimestamp,
					UpdatedAt:  &testTimestamp,
				},
			},
		})
	}))
	defer srv.Close()

	tokenClient, err := NewClientWithToken("header.payload.signature", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("new token client: %v", err)
	}

	me, err := tokenClient.Customers.Me(context.Background())
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	if me.Customer.Id == nil || *me.Customer.Id != testCustomerID {
		t.Fatalf("unexpected customer ID")
	}
}

func TestCustomersMeSubscription(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.CustomersMeSubscription || r.Method != http.MethodGet {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.CustomerMeSubscriptionResponse{
			Subscription: generated.CustomerMeSubscription{
				TierCode:        generated.TierCode("pro"),
				TierDisplayName: "Pro",
			},
		})
	}))
	defer srv.Close()

	tokenClient, err := NewClientWithToken("header.payload.signature", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("new token client: %v", err)
	}

	sub, err := tokenClient.Customers.MeSubscription(context.Background())
	if err != nil {
		t.Fatalf("me subscription: %v", err)
	}
	if string(sub.TierCode) != "pro" {
		t.Fatalf("unexpected tier code %s", sub.TierCode)
	}
}

func TestCustomersMeUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.CustomersMeUsage || r.Method != http.MethodGet {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.CustomerMeUsageResponse{
			Usage: generated.CustomerMeUsage{
				WindowStart: testTimestamp,
				WindowEnd:   testTimestamp.Add(time.Hour),
				Requests:    42,
				Tokens:      1000,
				Images:      5,
				Daily:       []generated.CustomerUsagePoint{},
			},
		})
	}))
	defer srv.Close()

	tokenClient, err := NewClientWithToken("header.payload.signature", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("new token client: %v", err)
	}

	usage, err := tokenClient.Customers.MeUsage(context.Background())
	if err != nil {
		t.Fatalf("me usage: %v", err)
	}
	if usage.Requests != 42 {
		t.Fatalf("unexpected requests count %d", usage.Requests)
	}
	if usage.Tokens != 1000 {
		t.Fatalf("unexpected tokens count %d", usage.Tokens)
	}
}

func TestCustomersAPIErrorHandling(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "not found error",
			status:     404,
			body:       `{"code":"NOT_FOUND","message":"customer not found"}`,
			wantStatus: 404,
			wantCode:   "NOT_FOUND",
		},
		{
			name:       "unauthorized error",
			status:     401,
			body:       `{"code":"UNAUTHORIZED","message":"invalid api key"}`,
			wantStatus: 401,
			wantCode:   "UNAUTHORIZED",
		},
		{
			name:       "rate limit error",
			status:     429,
			body:       `{"code":"RATE_LIMIT","message":"too many requests"}`,
			wantStatus: 429,
			wantCode:   "RATE_LIMIT",
		},
		{
			name:       "server error",
			status:     500,
			body:       `{"code":"INTERNAL_ERROR","message":"internal server error"}`,
			wantStatus: 500,
			wantCode:   "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			client := newTestClient(t, srv, "mr_sk_errors")

			_, err := client.Customers.List(context.Background())
			if err == nil {
				t.Fatalf("expected error")
			}
			var apiErr APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected APIError, got %T: %v", err, err)
			}
			if apiErr.Status != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, apiErr.Status)
			}
			if string(apiErr.Code) != tt.wantCode {
				t.Fatalf("expected code %s, got %s", tt.wantCode, apiErr.Code)
			}
		})
	}
}

func TestCustomersMalformedJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_malformed")

	_, err := client.Customers.List(context.Background())
	if err == nil {
		t.Fatalf("expected error for malformed JSON")
	}
}

func TestCustomersHTMLErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`<html><body>502 Bad Gateway</body></html>`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_html")

	_, err := client.Customers.List(context.Background())
	if err == nil {
		t.Fatalf("expected error for HTML response")
	}
}

func TestCustomersValidation(t *testing.T) {
	client := &Client{}

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "Create with empty request",
			fn: func() error {
				_, err := client.Customers.Create(context.Background(), CustomerCreateRequest{})
				return err
			},
		},
		{
			name: "Upsert with invalid email",
			fn: func() error {
				_, err := client.Customers.Upsert(context.Background(), CustomerUpsertRequest{
					ExternalID: NewCustomerExternalID("ext"),
					Email:      "bad",
				})
				return err
			},
		},
		{
			name: "Create with invalid metadata",
			fn: func() error {
				_, err := client.Customers.Create(context.Background(), CustomerCreateRequest{
					ExternalID: NewCustomerExternalID("ext"),
					Email:      "user@example.com",
					Metadata:   CustomerMetadata{"bad": CustomerMetadataValue{}},
				})
				return err
			},
		},
		{
			name: "Get with nil customer_id",
			fn: func() error {
				_, err := client.Customers.Get(context.Background(), uuid.Nil)
				return err
			},
		},
		{
			name: "Subscribe with nil customer_id",
			fn: func() error {
				_, err := client.Customers.Subscribe(context.Background(), uuid.Nil, CustomerSubscribeRequest{})
				return err
			},
		},
		{
			name: "Subscribe with empty URLs",
			fn: func() error {
				_, err := client.Customers.Subscribe(context.Background(), testCustomerID, CustomerSubscribeRequest{
					TierID: testTierID,
				})
				return err
			},
		},
		{
			name: "Unsubscribe with nil customer_id",
			fn: func() error {
				return client.Customers.Unsubscribe(context.Background(), uuid.Nil)
			},
		},
		{
			name: "GetSubscription with nil customer_id",
			fn: func() error {
				_, err := client.Customers.GetSubscription(context.Background(), uuid.Nil)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}
