package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"
)

const (
	defaultBaseURL    = "https://api.modelrelay.ai/api/v1"
	stagingBaseURL    = "https://api-stg.modelrelay.ai/api/v1"
	sandboxBaseURL    = "https://api.sandbox.modelrelay.ai/api/v1"
	defaultClientHead = "modelrelay-go/dev"
)

// Environment presets for well-known API hosts.
type Environment string

const (
	EnvironmentProduction Environment = "production"
	EnvironmentStaging    Environment = "staging"
	EnvironmentSandbox    Environment = "sandbox"
)

func (e Environment) baseURL() string {
	switch e {
	case EnvironmentStaging:
		return stagingBaseURL
	case EnvironmentSandbox:
		return sandboxBaseURL
	default:
		return defaultBaseURL
	}
}

// Config wires authentication, base URL, headers/metadata defaults, and telemetry for the API client.
type Config struct {
	BaseURL         string
	Environment     Environment
	APIKey          string
	AccessToken     string
	HTTPClient      *http.Client
	Telemetry       TelemetryHooks
	UserAgent       string
	ClientHeader    string
	DefaultHeaders  http.Header
	DefaultMetadata map[string]string
}

// Client provides high-level helpers for interacting with the ModelRelay API.
type Client struct {
	baseURL         string
	httpClient      *http.Client
	auth            authChain
	telemetry       TelemetryHooks
	userAgent       string
	clientHead      string
	defaultHeaders  http.Header
	defaultMetadata map[string]string

	// Grouped service clients.
	LLM      *LLMClient
	Usage    *UsageClient
	APIKeys  *APIKeysClient
	Auth     *AuthClient
	EndUsers *EndUsersClient
}

// NewClient validates the configuration and returns a ready-to-use Client.
func NewClient(cfg Config) (*Client, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = cfg.Environment.baseURL()
	}
	normalized, err := normalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	auth := buildAuthChain(cfg)
	if len(auth) == 0 {
		return nil, errors.New("sdk: api key or access token required")
	}
	ua := strings.TrimSpace(cfg.UserAgent)
	if ua == "" {
		ua = deriveDefaultClientHeader()
	}
	clientHeader := strings.TrimSpace(cfg.ClientHeader)
	if clientHeader == "" {
		clientHeader = deriveDefaultClientHeader()
	}
	defaultHeaders := sanitizeHeaders(cfg.DefaultHeaders)
	defaultMetadata := sanitizeMetadata(cfg.DefaultMetadata)
	client := &Client{
		baseURL:         normalized,
		httpClient:      httpClient,
		auth:            auth,
		telemetry:       cfg.Telemetry,
		userAgent:       ua,
		clientHead:      clientHeader,
		defaultHeaders:  defaultHeaders,
		defaultMetadata: defaultMetadata,
	}
	client.LLM = &LLMClient{client: client}
	client.Usage = &UsageClient{client: client}
	client.APIKeys = &APIKeysClient{client: client}
	client.Auth = &AuthClient{client: client}
	client.EndUsers = &EndUsersClient{client: client}
	return client, nil
}

func normalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("sdk: base URL required")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("sdk: invalid base URL: %w", err)
	}
	if u.Scheme == "" {
		return "", errors.New("sdk: base URL missing scheme (http/https)")
	}
	if u.Host == "" {
		return "", errors.New("sdk: base URL missing host")
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	return strings.TrimSuffix(u.String(), "/"), nil
}

func buildAuthChain(cfg Config) authChain {
	var chain authChain
	if cfg.AccessToken != "" {
		token := strings.TrimSpace(cfg.AccessToken)
		if strings.HasPrefix(strings.ToLower(token), "bearer ") {
			token = strings.TrimSpace(token[7:])
		}
		chain = append(chain, bearerAuth{token: token})
	}
	if cfg.APIKey != "" {
		chain = append(chain, apiKeyAuth{key: cfg.APIKey})
	}
	return chain
}

func sanitizeHeaders(src http.Header) http.Header {
	if len(src) == 0 {
		return nil
	}
	out := make(http.Header)
	for key, values := range src {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		for _, v := range values {
			trimmedVal := strings.TrimSpace(v)
			if trimmedVal == "" {
				continue
			}
			out.Add(trimmedKey, trimmedVal)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sanitizeMetadata(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" || val == "" {
			continue
		}
		out[key] = val
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func deriveDefaultClientHeader() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		version := strings.TrimSpace(info.Main.Version)
		if version != "" && version != "(devel)" {
			return fmt.Sprintf("modelrelay-go/%s", version)
		}
	}
	return defaultClientHead
}

func (c *Client) newJSONRequest(ctx context.Context, method, path string, payload any) (*http.Request, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.buildURL(path), body)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}
	injectTraceparent(ctx, req)
	return req, nil
}

func (c *Client) prepare(req *http.Request) {
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if c.clientHead != "" && req.Header.Get("X-ModelRelay-Client") == "" {
		req.Header.Set("X-ModelRelay-Client", c.clientHead)
	}
	c.auth.Apply(req)
	for key, values := range c.defaultHeaders {
		if req.Header.Get(key) != "" {
			continue
		}
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
}

func (c *Client) send(req *http.Request) (*http.Response, error) {
	c.prepare(req)
	if c.telemetry.OnHTTPRequest != nil {
		c.telemetry.OnHTTPRequest(req.Context(), req)
	}
	c.telemetry.log(req.Context(), LogLevelInfo, "http_request", map[string]any{
		"method": req.Method,
		"url":    req.URL.String(),
	})
	start := time.Now()
	resp, err := c.httpClient.Do(req)
	if c.telemetry.OnHTTPResponse != nil {
		c.telemetry.OnHTTPResponse(req.Context(), req, resp, err, time.Since(start))
	}
	c.telemetry.metric(req.Context(), "sdk_http_request_latency_ms", float64(time.Since(start).Milliseconds()), map[string]string{
		"path": req.URL.Path,
	})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, decodeAPIError(resp)
	}
	return resp, nil
}

func (c *Client) buildURL(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.baseURL + path
}
