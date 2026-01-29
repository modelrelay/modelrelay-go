package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelrelay/modelrelay/sdk/go/headers"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

// BatchStatus represents the status of an individual batch result.
type BatchStatus string

// BatchStatus constants.
const (
	BatchStatusSuccess BatchStatus = "success"
	BatchStatusError   BatchStatus = "error"
)

type BatchRequestItem struct {
	ID      string
	Request ResponseRequest
}

type BatchResponse struct {
	ID      string        `json:"id"`
	Results []BatchResult `json:"results"`
	Usage   BatchUsage    `json:"usage"`
}

type BatchResult struct {
	ID       string      `json:"id"`
	Status   BatchStatus `json:"status"`
	Response *Response   `json:"response,omitempty"`
	Error    *BatchError `json:"error,omitempty"`
}

type BatchError struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
	Code    string `json:"code,omitempty"`
}

type BatchUsage struct {
	TotalInputTokens   int64 `json:"total_input_tokens"`
	TotalOutputTokens  int64 `json:"total_output_tokens"`
	TotalRequests      int   `json:"total_requests"`
	SuccessfulRequests int   `json:"successful_requests"`
	FailedRequests     int   `json:"failed_requests"`
}

type BatchResponseOption func(*batchResponsesOptions)

type batchResponsesOptions struct {
	call          responseCallOptions
	maxConcurrent *int
	failFast      *bool
	itemTimeoutMs *int
}

func WithBatchRequestID(requestID string) BatchResponseOption {
	return func(opts *batchResponsesOptions) {
		WithRequestID(requestID)(&opts.call)
	}
}

func WithBatchCustomerID(customerID string) BatchResponseOption {
	return func(opts *batchResponsesOptions) {
		WithCustomerID(customerID)(&opts.call)
	}
}

func WithBatchHeader(key, value string) BatchResponseOption {
	return func(opts *batchResponsesOptions) {
		WithHeader(key, value)(&opts.call)
	}
}

func WithBatchHeaders(hdrs map[string]string) BatchResponseOption {
	return func(opts *batchResponsesOptions) {
		WithHeaders(hdrs)(&opts.call)
	}
}

func WithBatchTimeout(timeout time.Duration) BatchResponseOption {
	return func(opts *batchResponsesOptions) {
		WithTimeout(timeout)(&opts.call)
	}
}

func WithBatchRetry(cfg RetryConfig) BatchResponseOption {
	return func(opts *batchResponsesOptions) {
		WithRetry(cfg)(&opts.call)
	}
}

func WithBatchMaxConcurrent(maxConcurrent int) BatchResponseOption {
	return func(opts *batchResponsesOptions) {
		opts.maxConcurrent = &maxConcurrent
	}
}

func WithBatchFailFast(failFast bool) BatchResponseOption {
	return func(opts *batchResponsesOptions) {
		opts.failFast = &failFast
	}
}

func WithBatchItemTimeoutMs(timeoutMs int) BatchResponseOption {
	return func(opts *batchResponsesOptions) {
		opts.itemTimeoutMs = &timeoutMs
	}
}

func buildBatchResponsesOptions(options []BatchResponseOption) batchResponsesOptions {
	cfg := batchResponsesOptions{}
	for _, opt := range options {
		if opt == nil {
			continue
		}
		opt(&cfg)
	}
	cfg.call.headers = sanitizeHeaders(cfg.call.headers)
	return cfg
}

type responsesBatchItemPayload struct {
	ID string `json:"id"`
	responseRequestPayload
}

type responsesBatchOptionsPayload struct {
	MaxConcurrent *int  `json:"max_concurrent,omitempty"`
	FailFast      *bool `json:"fail_fast,omitempty"`
	TimeoutMs     *int  `json:"timeout_ms,omitempty"`
}

type responsesBatchRequestPayload struct {
	Requests []responsesBatchItemPayload   `json:"requests"`
	Options  *responsesBatchOptionsPayload `json:"options,omitempty"`
}

// batchValidationResult captures the pure result of batch request validation.
// This separates validation logic from I/O (HTTP calls, logging).
type batchValidationResult struct {
	// Items contains the validated and transformed request payloads.
	Items []responsesBatchItemPayload
	// Err is set if validation failed.
	Err error
}

// validateBatchRequests performs pure validation of batch requests without any I/O.
// Returns validated items on success, or an error describing the validation failure.
func validateBatchRequests(requests []BatchRequestItem, requireModel bool) batchValidationResult {
	if len(requests) == 0 {
		return batchValidationResult{Err: fmt.Errorf("requests is required")}
	}

	items := make([]responsesBatchItemPayload, 0, len(requests))
	seen := make(map[string]struct{}, len(requests))
	for i := range requests {
		id := strings.TrimSpace(requests[i].ID)
		if id == "" {
			return batchValidationResult{Err: fmt.Errorf("request id is required")}
		}
		if _, exists := seen[id]; exists {
			return batchValidationResult{Err: fmt.Errorf("request ids must be unique")}
		}
		seen[id] = struct{}{}
		if err := requests[i].Request.validate(requireModel); err != nil {
			return batchValidationResult{Err: err}
		}
		items = append(items, responsesBatchItemPayload{
			ID:                     id,
			responseRequestPayload: newResponseRequestPayload(requests[i].Request),
		})
	}

	return batchValidationResult{Items: items}
}

// BatchResponses performs a synchronous batch /responses request.
// This is the imperative shell that handles HTTP I/O and logging.
func (c *ResponsesClient) BatchResponses(ctx context.Context, requests []BatchRequestItem, options ...BatchResponseOption) (*BatchResponse, error) {
	cfg := buildBatchResponsesOptions(options)
	if cfg.call.retry == nil {
		retryCfg := c.client.retryCfg
		retryCfg.RetryPost = true
		cfg.call.retry = &retryCfg
	}

	requireModel := cfg.call.headers == nil || strings.TrimSpace(cfg.call.headers.Get(headers.CustomerID)) == ""
	if requireModel && c.client != nil && c.client.hasJWTAccessToken() {
		requireModel = false
	}

	// Pure validation - no I/O
	result := validateBatchRequests(requests, requireModel)
	if result.Err != nil {
		return nil, result.Err
	}

	payload := responsesBatchRequestPayload{Requests: result.Items}
	if cfg.maxConcurrent != nil || cfg.failFast != nil || cfg.itemTimeoutMs != nil {
		payload.Options = &responsesBatchOptionsPayload{
			MaxConcurrent: cfg.maxConcurrent,
			FailFast:      cfg.failFast,
			TimeoutMs:     cfg.itemTimeoutMs,
		}
	}

	httpReq, err := c.client.newJSONRequest(ctx, http.MethodPost, routes.ResponsesBatch, payload)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json")
	applyResponseHeaders(httpReq, cfg.call)

	resp, retryMeta, err := c.client.send(httpReq, cfg.call.timeout, cfg.call.retry)
	if err != nil {
		c.client.telemetry.log(ctx, LogLevelError, "responses_batch_failed", map[string]any{"error": err.Error(), "retries": retryMeta})
		return nil, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()

	var respPayload BatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&respPayload); err != nil {
		return nil, err
	}
	return &respPayload, nil
}
