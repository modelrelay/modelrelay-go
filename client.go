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
	"sync"
	"time"
)

const (
	defaultBaseURL    = "https://api.modelrelay.ai/api/v1"
	defaultClientHead = "modelrelay-go/dev"
	defaultConnectTO  = 5 * time.Second
	defaultRequestTO  = 60 * time.Second
)

// Option configures optional settings for the SDK client.
type Option func(*clientOptions)

type clientOptions struct {
	baseURL        string
	httpClient     *http.Client
	telemetry      TelemetryHooks
	userAgent      string
	clientHeader   string
	defaultHeaders http.Header
	connectTimeout *time.Duration
	requestTimeout *time.Duration
	retry          *RetryConfig
	tokenProvider  TokenProvider
}

// WithBaseURL sets a custom API base URL.
func WithBaseURL(baseURL string) Option {
	return func(o *clientOptions) { o.baseURL = baseURL }
}

// WithHTTPClient sets a custom HTTP client for requests.
func WithHTTPClient(client *http.Client) Option {
	return func(o *clientOptions) { o.httpClient = client }
}

// WithTelemetry configures telemetry hooks for observability.
func WithTelemetry(hooks TelemetryHooks) Option {
	return func(o *clientOptions) { o.telemetry = hooks }
}

// WithUserAgent sets a custom User-Agent header.
func WithUserAgent(ua string) Option {
	return func(o *clientOptions) { o.userAgent = ua }
}

// WithClientHeader sets the X-ModelRelay-Client header for SDK identification.
func WithClientHeader(header string) Option {
	return func(o *clientOptions) { o.clientHeader = header }
}

// WithDefaultHeaders sets headers applied to every request.
func WithDefaultHeaders(headers http.Header) Option {
	return func(o *clientOptions) { o.defaultHeaders = headers }
}

// WithConnectTimeout sets the connection timeout (nil uses default, 0 disables).
func WithConnectTimeout(d time.Duration) Option {
	return func(o *clientOptions) { o.connectTimeout = &d }
}

// WithRequestTimeout sets the request timeout (nil uses default, 0 disables).
func WithRequestTimeout(d time.Duration) Option {
	return func(o *clientOptions) { o.requestTimeout = &d }
}

// WithRetryConfig sets the retry/backoff policy.
func WithRetryConfig(cfg RetryConfig) Option {
	return func(o *clientOptions) { o.retry = &cfg }
}

// WithTokenProvider configures an optional TokenProvider used to supply bearer tokens.
// This is primarily intended for data-plane endpoints like /responses and /runs.
func WithTokenProvider(provider TokenProvider) Option {
	return func(o *clientOptions) { o.tokenProvider = provider }
}

// Client provides high-level helpers for interacting with the ModelRelay API.
type Client struct {
	baseURL        string
	httpClient     *http.Client
	auth           authChain
	telemetry      TelemetryHooks
	userAgent      string
	clientHead     string
	defaultHeaders http.Header
	connectTimeout time.Duration
	requestTimeout time.Duration
	retryCfg       RetryConfig

	// Grouped service clients.
	Responses *ResponsesClient
	Workflows *WorkflowsClient
	Runs      *RunsClient
	Usage     *UsageClient
	Auth      *AuthClient
	Customers *CustomersClient
	Tiers     *TiersClient
	Models    *ModelsClient

	pluginsOnce sync.Once
	plugins     *PluginsClient
}

// NewClientWithKey creates a client authenticated with an API key.
// The key parameter is required and must be non-empty.
// Use functional options to configure additional settings.
//
// Example:
//
//	secret, err := sdk.ParseSecretKey("mr_sk_...")
//	if err != nil { /* handle */ }
//	client, err := sdk.NewClientWithKey(secret)
func NewClientWithKey(key APIKeyAuth, opts ...Option) (*Client, error) {
	if key == nil || strings.TrimSpace(key.String()) == "" {
		return nil, ConfigError{Reason: "api key is required"}
	}
	options := applyOptions(opts)
	return newClientFromOptions(key, "", options)
}

// NewClientWithToken creates a client authenticated with a bearer access token.
// The token parameter is required and must be non-empty.
// Use functional options to configure additional settings.
//
// Example:
//
//	client, err := sdk.NewClientWithToken("eyJ...")
//	client, err := sdk.NewClientWithToken(customerToken, sdk.WithBaseURL("https://custom.api.com"))
func NewClientWithToken(token string, opts ...Option) (*Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ConfigError{Reason: "access token is required"}
	}
	options := applyOptions(opts)
	return newClientFromOptions(nil, token, options)
}

// NewClientWithTokenProvider creates a client that obtains bearer tokens from a TokenProvider.
// Use functional options to configure additional settings.
func NewClientWithTokenProvider(provider TokenProvider, opts ...Option) (*Client, error) {
	if provider == nil {
		return nil, ConfigError{Reason: "token provider is required"}
	}
	opts = append(opts, WithTokenProvider(provider))
	options := applyOptions(opts)
	return newClientFromOptions(nil, "", options)
}

func applyOptions(opts []Option) clientOptions {
	var options clientOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	return options
}

func newClientFromOptions(apiKey APIKeyAuth, accessToken string, opts clientOptions) (*Client, error) {
	baseURL := opts.baseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	normalized, err := normalizeBaseURL(baseURL)
	if err != nil {
		return nil, ConfigError{Reason: err.Error()}
	}
	httpClient := opts.httpClient
	if httpClient == nil {
		httpClient = newHTTPClient(resolveConnectTimeout(opts.connectTimeout), resolveRequestTimeout(opts.requestTimeout))
	}
	var auth authChain
	if accessToken != "" {
		token := strings.TrimSpace(accessToken)
		if strings.HasPrefix(strings.ToLower(token), "bearer ") {
			token = strings.TrimSpace(token[7:])
		}
		auth = append(auth, bearerAuth{token: token})
	} else if opts.tokenProvider != nil {
		auth = append(auth, tokenProviderAuth{provider: opts.tokenProvider})
	}
	if apiKey != nil {
		auth = append(auth, apiKeyAuth{key: apiKey})
	}
	ua := strings.TrimSpace(opts.userAgent)
	if ua == "" {
		ua = deriveDefaultClientHeader()
	}
	clientHeader := strings.TrimSpace(opts.clientHeader)
	if clientHeader == "" {
		clientHeader = deriveDefaultClientHeader()
	}
	defaultHeaders := sanitizeHeaders(opts.defaultHeaders)
	retryCfg := defaultRetryConfig()
	if opts.retry != nil {
		retryCfg = opts.retry.normalized()
	}
	client := &Client{
		baseURL:        normalized,
		httpClient:     httpClient,
		auth:           auth,
		telemetry:      opts.telemetry,
		userAgent:      ua,
		clientHead:     clientHeader,
		defaultHeaders: defaultHeaders,
		connectTimeout: resolveConnectTimeout(opts.connectTimeout),
		requestTimeout: resolveRequestTimeout(opts.requestTimeout),
		retryCfg:       retryCfg,
	}
	client.Responses = &ResponsesClient{client: client}
	client.Workflows = &WorkflowsClient{client: client}
	client.Runs = &RunsClient{client: client}
	client.Usage = &UsageClient{client: client}
	client.Auth = &AuthClient{client: client}
	client.Customers = &CustomersClient{client: client}
	client.Tiers = &TiersClient{client: client}
	client.Models = &ModelsClient{client: client}
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
	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		// Fallback to default transport if type assertion fails
		return &http.Client{Timeout: 0}
	}
	cloned := transport.Clone()
	cloned.DialContext = (&net.Dialer{Timeout: connectTimeout}).DialContext
	return &http.Client{Transport: cloned, Timeout: 0}
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

// isSecretKey returns true if the client was configured with a secret key (mr_sk_*).
func (c *Client) isSecretKey() bool {
	for _, s := range c.auth {
		if ak, ok := s.(apiKeyAuth); ok {
			return ak.isSecretKey()
		}
	}
	return false
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

func (c *Client) prepare(req *http.Request) error {
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if c.clientHead != "" && req.Header.Get("X-ModelRelay-Client") == "" {
		req.Header.Set("X-ModelRelay-Client", c.clientHead)
	}
	if err := c.auth.Apply(req); err != nil {
		return err
	}
	for key, values := range c.defaultHeaders {
		if req.Header.Get(key) != "" {
			continue
		}
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	return nil
}

func (c *Client) send(req *http.Request, timeout *time.Duration, retry *RetryConfig) (*http.Response, *RetryMetadata, error) {
	req = req.Clone(req.Context())
	duration := resolveTimeout(timeout, c.requestTimeout)
	ctx := req.Context()
	var cancel context.CancelFunc
	if duration > 0 {
		if dl, ok := ctx.Deadline(); !ok || time.Until(dl) > duration {
			ctx, cancel = context.WithTimeout(ctx, duration)
			req = req.WithContext(ctx)
		}
	}
	cfg := c.retryCfg
	if retry != nil {
		cfg = retry.normalized()
	}
	resp, meta, err := c.sendWithRetry(req, cfg)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, meta, err
	}
	if resp == nil {
		if cancel != nil {
			cancel()
		}
		return nil, meta, nil
	}
	if cancel != nil && resp.Body != nil {
		resp.Body = &cancelOnCloseReadCloser{rc: resp.Body, cancel: cancel}
	}
	return resp, meta, nil
}

// sendStreaming sends a request intended for streaming responses.
//
// IMPORTANT: Streaming requests must not apply per-request timeout contexts that
// could cancel the body read after headers are received. Stream deadlines are
// enforced by stream-specific timeouts (TTFT/Idle/Total) instead.
func (c *Client) sendStreaming(req *http.Request, retry *RetryConfig) (*http.Response, *RetryMetadata, error) {
	req = req.Clone(req.Context())
	cfg := c.retryCfg
	if retry != nil {
		cfg = retry.normalized()
	}
	resp, meta, err := c.sendWithRetry(req, cfg)
	if err != nil {
		return nil, meta, err
	}
	return resp, meta, nil
}

// sendAndDecode sends a JSON request and decodes the response body into result.
// This helper reduces boilerplate for simple request/response patterns.
func (c *Client) sendAndDecode(ctx context.Context, method, path string, payload, result any) error {
	req, err := c.newJSONRequest(ctx, method, path, payload)
	if err != nil {
		return err
	}
	resp, _, err := c.send(req, nil, nil)
	if err != nil {
		return err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	return json.NewDecoder(resp.Body).Decode(result)
}

type cancelOnCloseReadCloser struct {
	rc     io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
}

func (c *cancelOnCloseReadCloser) Read(p []byte) (int, error) {
	return c.rc.Read(p)
}

func (c *cancelOnCloseReadCloser) Close() error {
	err := c.rc.Close()
	c.once.Do(c.cancel)
	return err
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
		prepErr := c.prepare(cloned)
		if prepErr != nil {
			return nil, &meta, prepErr
		}
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
			//nolint:errcheck,gocritic // best-effort cleanup on error in retry loop
			defer func() { _ = resp.Body.Close() }()
			return nil, copyRetryMeta(meta), decodeAPIError(resp, copyRetryMeta(meta))
		}

		if resp != nil {
			//nolint:errcheck // best-effort cleanup before retry
			_ = resp.Body.Close()
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
			//nolint:staticcheck // Temporary() is deprecated but still useful for detecting transient errors
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
