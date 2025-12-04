// Package auth provides authentication helpers for the ModelRelay SDK.
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultUserAgent = "ModelRelaySDK/1"

// Config controls how the SDK auth client talks to the ModelRelay API.
type Config struct {
	BaseURL    string
	HTTPClient *http.Client
	UserAgent  string
}

// Client issues login and refresh requests against the SaaS API.
type Client struct {
	baseURL    string
	httpClient *http.Client
	userAgent  string
}

// Credentials encapsulates username/password inputs for login.
type Credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RefreshRequest wraps the token used during refresh.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// TokenResponse mirrors the control-plane response body.
type TokenResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Error conveys HTTP failures from the SaaS API.
type Error struct {
	Status int
	Body   string
}

func (e Error) Error() string {
	return fmt.Sprintf("sdk/auth: http %d: %s", e.Status, strings.TrimSpace(e.Body))
}

// NewClient constructs a Client with sane defaults.
func NewClient(cfg Config) (*Client, error) {
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" {
		return nil, errors.New("sdk/auth: base url required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	ua := cfg.UserAgent
	if ua == "" {
		ua = defaultUserAgent
	}
	return &Client{
		baseURL:    strings.TrimSuffix(base, "/"),
		httpClient: client,
		userAgent:  ua,
	}, nil
}

// Login exchanges user credentials for access/refresh tokens.
func (c *Client) Login(ctx context.Context, creds Credentials) (TokenResponse, error) {
	if strings.TrimSpace(creds.Email) == "" || strings.TrimSpace(creds.Password) == "" {
		return TokenResponse{}, errors.New("sdk/auth: email and password required")
	}
	return c.post(ctx, "/auth/login", creds)
}

// Refresh swaps a refresh token for a new token pair.
func (c *Client) Refresh(ctx context.Context, req RefreshRequest) (TokenResponse, error) {
	if strings.TrimSpace(req.RefreshToken) == "" {
		return TokenResponse{}, errors.New("sdk/auth: refresh token required")
	}
	return c.post(ctx, "/auth/refresh", req)
}

func (c *Client) post(ctx context.Context, path string, payload any) (TokenResponse, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return TokenResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return TokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TokenResponse{}, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TokenResponse{}, err
	}
	if resp.StatusCode >= 400 {
		return TokenResponse{}, Error{Status: resp.StatusCode, Body: string(body)}
	}

	var tokens TokenResponse
	if err := json.Unmarshal(body, &tokens); err != nil {
		return TokenResponse{}, err
	}
	return tokens, nil
}
