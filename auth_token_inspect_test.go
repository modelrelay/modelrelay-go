package sdk

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

type failingTokenProvider struct{}

func (f failingTokenProvider) Token(ctx context.Context) (string, error) {
	return "", errors.New("boom")
}

type emptyTokenProvider struct{}

func (e emptyTokenProvider) Token(ctx context.Context) (string, error) {
	return "   ", nil
}

func TestAuthTokenInspectHelpers(t *testing.T) {
	if isAPIKeyToken(" ") {
		t.Fatalf("expected blank token to be false")
	}
	if !isAPIKeyToken("mr_sk_test") {
		t.Fatalf("expected secret key to be api key")
	}
	if isAPIKeyToken("header.payload.signature") {
		t.Fatalf("expected jwt-like token to not be api key")
	}
	if isJWTLikeToken(" ") {
		t.Fatalf("expected blank jwt to be false")
	}
	if !isJWTLikeToken("header.payload.signature") {
		t.Fatalf("expected jwt-like token to be true")
	}
}

func TestHasJWTAccessToken(t *testing.T) {
	client, err := NewClientWithToken("header.payload.signature")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if !client.hasJWTAccessToken() {
		t.Fatalf("expected JWT token detection")
	}

	client, err = NewClientWithToken("mr_sk_test")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if client.hasJWTAccessToken() {
		t.Fatalf("expected api key token to be false")
	}
}

func TestAuthChainAppliesHeaders(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	chain := authChain{bearerAuth{token: "tok"}, apiKeyAuth{key: SecretKey("mr_sk_1")}}
	if err := chain.Apply(req); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer tok" {
		t.Fatalf("unexpected authorization header %q", got)
	}
	if got := req.Header.Get("X-ModelRelay-Api-Key"); got != "mr_sk_1" {
		t.Fatalf("unexpected api key header %q", got)
	}
}

func TestTokenProviderAuthErrors(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)

	if err := (tokenProviderAuth{provider: failingTokenProvider{}}).Apply(req); err == nil {
		t.Fatalf("expected token provider error")
	}

	if err := (tokenProviderAuth{provider: emptyTokenProvider{}}).Apply(req); err == nil {
		t.Fatalf("expected empty token error")
	}

	provider := tokenProviderAuth{provider: TokenProviderFunc(func(ctx context.Context) (string, error) {
		return "Bearer tok2", nil
	})}
	if err := provider.Apply(req); err != nil {
		t.Fatalf("unexpected token provider error: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer tok2" {
		t.Fatalf("unexpected authorization header %q", got)
	}
}

type TokenProviderFunc func(ctx context.Context) (string, error)

func (f TokenProviderFunc) Token(ctx context.Context) (string, error) {
	return f(ctx)
}
