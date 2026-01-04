package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

type staticTokenProvider struct {
	token string
	calls atomic.Int64
}

func (p *staticTokenProvider) Token(ctx context.Context) (string, error) {
	p.calls.Add(1)
	return p.token, nil
}

func TestClientWithTokenProviderUsesBearerHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.Responses {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tp-123" {
			t.Fatalf("expected Authorization header got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llm.Response{
			ID:    "resp_tp",
			Model: "demo",
			Output: []llm.OutputItem{{
				Type:    llm.OutputItemTypeMessage,
				Role:    llm.RoleAssistant,
				Content: []llm.ContentPart{llm.TextPart("OK")},
			}},
			Usage: llm.Usage{TotalTokens: 1},
		})
	}))
	defer srv.Close()

	provider := &staticTokenProvider{token: "tp-123"}
	client, err := NewClientWithTokenProvider(provider, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	text, err := client.Responses.Text(context.Background(), NewModelID("demo"), "sys", "user")
	if err != nil {
		t.Fatalf("text: %v", err)
	}
	if text != "OK" {
		t.Fatalf("unexpected text %q", text)
	}
	if got := provider.calls.Load(); got != 1 {
		t.Fatalf("expected 1 token call, got %d", got)
	}
}

func TestCustomerTokenProviderMintsAndCaches(t *testing.T) {
	var mintCalls atomic.Int64
	secret := mustSecretKey(t, "mr_sk_test_customer_token")
	expiresAt := time.Now().Add(10 * time.Minute).UTC()
	projectID := uuid.New()
	customerID := uuid.New()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case routes.AuthCustomerToken:
			mintCalls.Add(1)
			if got := r.Header.Get("X-ModelRelay-Api-Key"); got != secret.String() {
				t.Fatalf("expected api key header got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(CustomerToken{
				Token:              "customer-token-2",
				ExpiresAt:          expiresAt,
				ExpiresIn:          600,
				TokenType:          TokenTypeBearer,
				ProjectID:          projectID,
				CustomerID:         &customerID,
				CustomerExternalID: NewCustomerExternalID("ext_2"),
				TierCode:           TierCodePtr("pro"),
			})
		case routes.Responses:
			if got := r.Header.Get("Authorization"); got != "Bearer customer-token-2" {
				t.Fatalf("expected Authorization header got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(llm.Response{
				ID:    "resp_customer_token",
				Model: "demo",
				Output: []llm.OutputItem{{
					Type:    llm.OutputItemTypeMessage,
					Role:    llm.RoleAssistant,
					Content: []llm.ContentPart{llm.TextPart("OK")},
				}},
				Usage: llm.Usage{TotalTokens: 1},
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	req := NewCustomerTokenRequestForCustomerID(customerID)
	req.TTLSeconds = 0
	provider, err := NewCustomerTokenProvider(CustomerTokenProviderConfig{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
		SecretKey:  secret,
		Request:    req,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	client, err := NewClientWithTokenProvider(provider, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, err = client.Responses.Text(context.Background(), NewModelID("demo"), "sys", "user")
	if err != nil {
		t.Fatalf("text: %v", err)
	}
	_, err = client.Responses.Text(context.Background(), NewModelID("demo"), "sys", "user")
	if err != nil {
		t.Fatalf("text (second): %v", err)
	}

	if got := mintCalls.Load(); got != 1 {
		t.Fatalf("expected 1 mint call, got %d", got)
	}
}
