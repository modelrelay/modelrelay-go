package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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
	defaultConnectTO  = 5 * time.Second
	defaultRequestTO  = 60 * time.Second
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
	// Optional timeouts (nil => defaults). Set to 0 to disable.
	ConnectTimeout *time.Duration
	RequestTimeout *time.Duration
	// Optional retry/backoff policy (nil => defaults).
	Retry *RetryConfig
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
	connectTimeout  time.Duration
	requestTimeout  time.Duration
	retryCfg        RetryConfig

	// Grouped service clients.
	LLM          *LLMClient
	Usage        *UsageClient
	APIKeys      *APIKeysClient
	Auth         *AuthClient
	EndUsers     *EndUsersClient
	Projects     *ProjectsClient
	RequestPlans *RequestPlansClient
}

// NewClient validates the configuration and returns a ready-to-use Client.
func NewClient(cfg Config) (*Client, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = cfg.Environment.baseURL()
	}
	normalized, err := normalizeBaseURL(baseURL)
	if err != nil {
		return nil, ConfigError{Reason: err.Error()}
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = newHTTPClient(resolveConnectTimeout(cfg.ConnectTimeout), resolveRequestTimeout(cfg.RequestTimeout))
	}
	auth := buildAuthChain(cfg)
	if len(auth) == 0 {
		return nil, ConfigError{Reason: "api key or access token required"}
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
	retryCfg := defaultRetryConfig()
	if cfg.Retry != nil {
		retryCfg = cfg.Retry.normalized()
	}
	client := &Client{
		baseURL:         normalized,
		httpClient:      httpClient,
		auth:            auth,
		telemetry:       cfg.Telemetry,
		userAgent:       ua,
		clientHead:      clientHeader,
		defaultHeaders:  defaultHeaders,
		defaultMetadata: defaultMetadata,
		connectTimeout:  resolveConnectTimeout(cfg.ConnectTimeout),
		requestTimeout:  resolveRequestTimeout(cfg.RequestTimeout),
		retryCfg:        retryCfg,
	}
	client.LLM = &LLMClient{client: client}
	client.Usage = &UsageClient{client: client}
	client.APIKeys = &APIKeysClient{client: client}
	client.Auth = &AuthClient{client: client}
	client.EndUsers = &EndUsersClient{client: client}
	client.Projects = &ProjectsClient{client: client}
	client.RequestPlans = &RequestPlansClient{client: client}
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

func resolveConnectTimeout(val *time.Duration) time.Duration {
	if val == nil {
		return defaultConnectTO
	}
	return *val
}

func resolveRequestTimeout(val *time.Duration) time.Duration {
	if val == nil {
		return defaultRequestTO
	}
	return *val
}

func resolveTimeout(override *time.Duration, def time.Duration) time.Duration {
	if override != nil {
		return *override
	}
	return def
}

func newHTTPClient(connectTimeout, requestTimeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: connectTimeout}).DialContext
	return &http.Client{Transport: transport, Timeout: 0}
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
	var getBody func() (io.ReadCloser, error)
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(encoded)
		buf := encoded
		getBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(buf)), nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.buildURL(path), body)
	if err != nil {
		return nil, err
	}
	req.GetBody = getBody
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

func (c *Client) send(req *http.Request, timeout *time.Duration, retry *RetryConfig) (*http.Response, *RetryMetadata, error) {
	req = req.Clone(req.Context())
	duration := resolveTimeout(timeout, c.requestTimeout)
	ctx := req.Context()
	if duration > 0 {
		if dl, ok := ctx.Deadline(); !ok || time.Until(dl) > duration {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, duration)
			req = req.WithContext(ctx)
			defer cancel()
		}
	}
	cfg := c.retryCfg
	if retry != nil {
		cfg = retry.normalized()
	}
	resp, meta, err := c.sendWithRetry(req, cfg)
	return resp, meta, err
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

func (c *Client) sendWithRetry(req *http.Request, cfg RetryConfig) (*http.Response, *RetryMetadata, error) {
	cfg = cfg.normalized()
	var meta RetryMetadata
	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		cloned, err := cloneRequest(req)
		if err != nil {
			return nil, &meta, TransportError{Message: "request not rewindable", Cause: err, Retry: &meta}
		}
		c.prepare(cloned)
		if c.telemetry.OnHTTPRequest != nil {
			c.telemetry.OnHTTPRequest(cloned.Context(), cloned)
		}
		start := time.Now()
		resp, err := c.httpClient.Do(cloned)
		latency := time.Since(start)
		if c.telemetry.OnHTTPResponse != nil {
			c.telemetry.OnHTTPResponse(cloned.Context(), cloned, resp, err, latency)
		}
		c.telemetry.metric(cloned.Context(), "sdk_http_request_latency_ms", float64(latency.Milliseconds()), map[string]string{
			"path": cloned.URL.Path,
		})

		retriable, status, reason := shouldRetry(cloned, resp, err, cfg)
		meta.Attempts = attempt
		meta.MaxAttempts = cfg.MaxAttempts
		meta.LastStatus = status
		meta.LastError = reason

		if err == nil && resp != nil && resp.StatusCode < 400 {
			return resp, copyRetryMeta(meta), nil
		}
		if !retriable || attempt == cfg.MaxAttempts {
			if err != nil {
				return nil, copyRetryMeta(meta), TransportError{Message: reason, Cause: err, Retry: copyRetryMeta(meta)}
			}
			defer resp.Body.Close()
			return nil, copyRetryMeta(meta), decodeAPIError(resp, copyRetryMeta(meta))
		}

		if resp != nil {
			resp.Body.Close()
		}
		backoff := cfg.backoffDelay(attempt)
		meta.LastBackoff = backoff
		c.telemetry.log(cloned.Context(), LogLevelInfo, "sdk_retry", map[string]any{
			"attempt":      attempt,
			"max_attempts": cfg.MaxAttempts,
			"backoff_ms":   backoff.Milliseconds(),
			"reason":       reason,
			"status":       status,
			"path":         cloned.URL.Path,
			"method":       cloned.Method,
			"retry_post":   cfg.RetryPost,
		})
		time.Sleep(backoff)
		lastErr = err
	}
	return nil, copyRetryMeta(meta), lastErr
}

func copyRetryMeta(meta RetryMetadata) *RetryMetadata {
	m := meta
	return &m
}

func cloneRequest(req *http.Request) (*http.Request, error) {
	cloned := req.Clone(req.Context())
	if req.Body == nil {
		return cloned, nil
	}
	if req.GetBody == nil {
		return nil, errors.New("sdk: request body is not rewindable")
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	cloned.Body = body
	return cloned, nil
}

func shouldRetry(req *http.Request, resp *http.Response, err error, cfg RetryConfig) (bool, int, string) {
	isSafeMethod := req.Method == http.MethodGet || req.Method == http.MethodHead || req.Method == http.MethodOptions
	allowNonSafe := cfg.RetryPost
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false, 0, err.Error()
		}
		if !isSafeMethod && !allowNonSafe {
			return false, 0, err.Error()
		}
		if ne, ok := err.(net.Error); ok {
			if ne.Timeout() || ne.Temporary() {
				return true, 0, err.Error()
			}
		}
		return true, 0, err.Error()
	}
	if resp == nil {
		return false, 0, "nil response"
	}
	status := resp.StatusCode
	if status == http.StatusTooManyRequests || status >= 500 {
		if !isSafeMethod && !allowNonSafe {
			return false, status, resp.Status
		}
		return true, status, resp.Status
	}
	return false, status, resp.Status
}
