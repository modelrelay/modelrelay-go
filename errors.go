package sdk

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrorCategory tags SDK errors for callers.
type ErrorCategory string

const (
	ErrorCategoryConfig    ErrorCategory = "config"
	ErrorCategoryTransport ErrorCategory = "transport"
	ErrorCategoryAPI       ErrorCategory = "api"
)

// ConfigError indicates invalid caller configuration.
type ConfigError struct {
	Reason string
}

func (e ConfigError) Error() string { return "sdk config: " + e.Reason }

// TransportError wraps network/timeout failures.
type TransportError struct {
	Message string
	Cause   error
	Retry   *RetryMetadata
}

func (e TransportError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return "transport error"
}

func (e TransportError) Unwrap() error { return e.Cause }

// StreamProtocolError indicates the server did not return the expected NDJSON stream.
// This typically happens when an upstream proxy returns HTML (e.g., 504 pages).
type StreamProtocolError struct {
	ExpectedContentType string
	ReceivedContentType string
	Status              int
}

func (e StreamProtocolError) Error() string {
	got := e.ReceivedContentType
	if got == "" {
		got = "<missing>"
	}
	return fmt.Sprintf("expected NDJSON stream (%s), got Content-Type %s", e.ExpectedContentType, got)
}

// StreamTimeoutKind indicates which streaming timeout triggered.
type StreamTimeoutKind string

const (
	StreamTimeoutTTFT  StreamTimeoutKind = "ttft"
	StreamTimeoutIdle  StreamTimeoutKind = "idle"
	StreamTimeoutTotal StreamTimeoutKind = "total"
)

// StreamTimeoutError indicates a streaming timeout (ttft, idle, or total).
type StreamTimeoutError struct {
	Kind    StreamTimeoutKind
	Timeout time.Duration
}

func (e StreamTimeoutError) Error() string {
	switch e.Kind {
	case StreamTimeoutTTFT:
		return fmt.Sprintf("stream TTFT timeout after %s", e.Timeout)
	case StreamTimeoutIdle:
		return fmt.Sprintf("stream idle timeout after %s", e.Timeout)
	case StreamTimeoutTotal:
		return fmt.Sprintf("stream total timeout after %s", e.Timeout)
	default:
		return fmt.Sprintf("stream timeout after %s", e.Timeout)
	}
}

// API error codes returned by the server.
// These constants can be used for programmatic error handling.
const (
	ErrCodeNotFound         = "NOT_FOUND"
	ErrCodeValidation       = "VALIDATION_ERROR"
	ErrCodeRateLimit        = "RATE_LIMIT"
	ErrCodeUnauthorized     = "UNAUTHORIZED"
	ErrCodeForbidden        = "FORBIDDEN"
	ErrCodeConflict         = "CONFLICT"
	ErrCodeInternal         = "INTERNAL_ERROR"
	ErrCodeUnavailable      = "SERVICE_UNAVAILABLE"
	ErrCodeInvalidInput     = "INVALID_INPUT"
	ErrCodePaymentRequired  = "PAYMENT_REQUIRED"
	ErrCodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	// ErrCodeNoTiers indicates no tiers are configured for the project.
	// To resolve: create at least one tier in your project dashboard.
	ErrCodeNoTiers = "NO_TIERS"
	// ErrCodeNoFreeTier indicates no free tier is available for auto-provisioning.
	// To resolve: either create a free tier for automatic customer creation,
	// or use the checkout flow to create paying customers first.
	ErrCodeNoFreeTier = "NO_FREE_TIER"
	// ErrCodeEmailRequired indicates email is required for auto-provisioning a new customer.
	// To resolve: provide the 'email' field in FrontendTokenRequest, or create the
	// customer via the dashboard/API before requesting a frontend token.
	ErrCodeEmailRequired = "EMAIL_REQUIRED"
)

// APIError captures structured SaaS error metadata.
type APIError struct {
	Status    int
	Code      string
	Message   string
	RequestID string
	Fields    []FieldError
	Retry     *RetryMetadata
}

// FieldError represents a validation failure for a single field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error implements the error interface.
func (e APIError) Error() string {
	if e.Code == "" {
		e.Code = "UNKNOWN"
	}
	if e.Message == "" {
		e.Message = fmt.Sprintf("%s (%d)", e.Code, e.Status)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// IsNotFound returns true if the error is a not found error.
func (e APIError) IsNotFound() bool { return e.Code == ErrCodeNotFound }

// IsValidation returns true if the error is a validation error.
func (e APIError) IsValidation() bool {
	return e.Code == ErrCodeValidation || e.Code == ErrCodeInvalidInput
}

// IsRateLimit returns true if the error is a rate limit error.
func (e APIError) IsRateLimit() bool { return e.Code == ErrCodeRateLimit }

// IsUnauthorized returns true if the error is an unauthorized error.
func (e APIError) IsUnauthorized() bool { return e.Code == ErrCodeUnauthorized }

// IsForbidden returns true if the error is a forbidden error.
func (e APIError) IsForbidden() bool { return e.Code == ErrCodeForbidden }

// IsUnavailable returns true if the error is a service unavailable error.
func (e APIError) IsUnavailable() bool { return e.Code == ErrCodeUnavailable }

// IsNoTiers returns true if no tiers are configured for the project.
// To resolve: create at least one tier in your project dashboard.
func (e APIError) IsNoTiers() bool { return e.Code == ErrCodeNoTiers }

// IsNoFreeTier returns true if no free tier is available for auto-provisioning.
// To resolve: either create a free tier or use the checkout flow.
func (e APIError) IsNoFreeTier() bool { return e.Code == ErrCodeNoFreeTier }

// IsEmailRequired returns true if email is required for auto-provisioning.
// To resolve: provide the 'email' field in FrontendTokenRequest.
func (e APIError) IsEmailRequired() bool { return e.Code == ErrCodeEmailRequired }

// IsProvisioningError returns true if this is a customer provisioning error.
// These errors occur when calling FrontendToken with a customer that doesn't exist
// and automatic provisioning cannot complete.
func (e APIError) IsProvisioningError() bool {
	return e.IsNoTiers() || e.IsNoFreeTier() || e.IsEmailRequired()
}

// Package-level helper functions for checking error types.

// IsEmailRequired returns true if the error indicates email is required for auto-provisioning.
func IsEmailRequired(err error) bool {
	if apiErr, ok := err.(APIError); ok {
		return apiErr.IsEmailRequired()
	}
	return false
}

// IsNoFreeTier returns true if the error indicates no free tier is available.
func IsNoFreeTier(err error) bool {
	if apiErr, ok := err.(APIError); ok {
		return apiErr.IsNoFreeTier()
	}
	return false
}

// IsNoTiers returns true if the error indicates no tiers are configured.
func IsNoTiers(err error) bool {
	if apiErr, ok := err.(APIError); ok {
		return apiErr.IsNoTiers()
	}
	return false
}

// IsProvisioningError returns true if the error is a customer provisioning error.
func IsProvisioningError(err error) bool {
	if apiErr, ok := err.(APIError); ok {
		return apiErr.IsProvisioningError()
	}
	return false
}

func decodeAPIError(resp *http.Response, retry *RetryMetadata) error {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		data = []byte(fmt.Sprintf("[failed to read response body: %v]", err))
	}
	apiErr := APIError{Status: resp.StatusCode, Retry: retry}
	if len(data) == 0 {
		apiErr.Message = resp.Status
		return apiErr
	}

	// Parse structured error: {"error": "...", "code": "...", "message": "..."}
	var payload struct {
		Error     string       `json:"error"`
		Code      string       `json:"code"`
		Message   string       `json:"message"`
		RequestID string       `json:"request_id"`
		Fields    []FieldError `json:"fields"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		apiErr.Message = string(data)
		return apiErr
	}
	apiErr.Code = payload.Code
	apiErr.Message = payload.Message
	apiErr.RequestID = payload.RequestID
	apiErr.Fields = payload.Fields
	if apiErr.Message == "" {
		apiErr.Message = resp.Status
	}
	return apiErr
}
