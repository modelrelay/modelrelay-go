package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

const defaultTokenProviderRefreshSkew = 60 * time.Second

type tokenCache struct {
	mu     sync.Mutex
	cached *CustomerToken
}

func (c *tokenCache) getReusable(skew time.Duration) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cached == nil || strings.TrimSpace(c.cached.Token) == "" || c.cached.ExpiresAt.IsZero() {
		return "", false
	}
	if time.Until(c.cached.ExpiresAt) <= skew {
		return "", false
	}
	return c.cached.Token, true
}

func (c *tokenCache) set(tok CustomerToken) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cached = &tok
}

// CustomerTokenProvider mints and caches customer-scoped bearer tokens using a secret key.
type CustomerTokenProvider struct {
	baseURL      string
	httpClient   *http.Client
	secretKey    SecretKey
	request      CustomerTokenRequest
	refreshSkew  time.Duration
	tokenCache   tokenCache
	clientHeader string
}

type CustomerTokenProviderConfig struct {
	BaseURL      string
	HTTPClient   *http.Client
	SecretKey    SecretKey
	Request      CustomerTokenRequest
	RefreshSkew  time.Duration
	ClientHeader string
}

func NewCustomerTokenProvider(cfg CustomerTokenProviderConfig) (*CustomerTokenProvider, error) {
	if strings.TrimSpace(cfg.SecretKey.String()) == "" {
		return nil, ConfigError{Reason: "secret key is required"}
	}
	if err := cfg.Request.Validate(); err != nil {
		return nil, ConfigError{Reason: err.Error()}
	}
	baseURL := cfg.BaseURL
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	normalized, err := normalizeBaseURL(baseURL)
	if err != nil {
		return nil, ConfigError{Reason: err.Error()}
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = newHTTPClient(defaultConnectTO, defaultRequestTO)
	}
	skew := cfg.RefreshSkew
	if skew <= 0 {
		skew = defaultTokenProviderRefreshSkew
	}
	return &CustomerTokenProvider{
		baseURL:      normalized,
		httpClient:   httpClient,
		secretKey:    cfg.SecretKey,
		request:      cfg.Request,
		refreshSkew:  skew,
		clientHeader: strings.TrimSpace(cfg.ClientHeader),
	}, nil
}

func (p *CustomerTokenProvider) Token(ctx context.Context) (string, error) {
	if p == nil {
		return "", errors.New("customer token provider is nil")
	}
	if tok, ok := p.tokenCache.getReusable(p.refreshSkew); ok {
		return tok, nil
	}
	tok, err := p.mint(ctx)
	if err != nil {
		return "", err
	}
	p.tokenCache.set(tok)
	return tok.Token, nil
}

func (p *CustomerTokenProvider) mint(ctx context.Context) (CustomerToken, error) {
	url := p.baseURL + routes.AuthCustomerToken
	body, err := json.Marshal(p.request)
	if err != nil {
		return CustomerToken{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return CustomerToken{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-ModelRelay-Api-Key", p.secretKey.String())
	if p.clientHeader != "" && req.Header.Get("X-ModelRelay-Client") == "" {
		req.Header.Set("X-ModelRelay-Client", p.clientHeader)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return CustomerToken{}, TransportError{
			Kind:    classifyTransportErrorKind(err),
			Message: "customer token request failed",
			Cause:   err,
		}
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return CustomerToken{}, decodeAPIError(resp, nil)
	}
	var tok CustomerToken
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return CustomerToken{}, err
	}
	return tok, nil
}
