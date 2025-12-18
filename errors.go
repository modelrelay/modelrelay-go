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

// TokenProviderError indicates the SDK could not obtain a bearer token from a TokenProvider.
type TokenProviderError struct {
	Cause error
}

func (e TokenProviderError) Error() string {
	if e.Cause == nil {
		return "sdk token provider: failed"
	}
	return "sdk token provider: " + e.Cause.Error()
}

func (e TokenProviderError) Unwrap() error { return e.Cause }

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

// ProtocolError indicates the server returned a syntactically valid response that violates
// the SDK's expected wire contract (e.g., invalid run event envelopes).
type ProtocolError struct {
	Message string
	Cause   error
}

func (e ProtocolError) Error() string {
	if e.Message == "" {
		return "protocol error"
	}
	return "protocol error: " + e.Message
}

func (e ProtocolError) Unwrap() error { return e.Cause }

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

// APIErrorCode is a strongly-typed error code returned by the API.
// Using a dedicated type instead of raw strings enables compile-time checking
// and prevents typos in error code comparisons.
type APIErrorCode string

// API error codes returned by the server.
// These constants can be used for programmatic error handling.
const (
	ErrCodeNotFound         APIErrorCode = "NOT_FOUND"
	ErrCodeValidation       APIErrorCode = "VALIDATION_ERROR"
	ErrCodeRateLimit        APIErrorCode = "RATE_LIMIT"
	ErrCodeUnauthorized     APIErrorCode = "UNAUTHORIZED"
	ErrCodeForbidden        APIErrorCode = "FORBIDDEN"
	ErrCodeConflict         APIErrorCode = "CONFLICT"
	ErrCodeInternal         APIErrorCode = "INTERNAL_ERROR"
	ErrCodeUnavailable      APIErrorCode = "SERVICE_UNAVAILABLE"
	ErrCodeInvalidInput     APIErrorCode = "INVALID_INPUT"
	ErrCodePaymentRequired  APIErrorCode = "PAYMENT_REQUIRED"
	ErrCodeMethodNotAllowed APIErrorCode = "METHOD_NOT_ALLOWED"

	// OIDC exchange error codes
	ErrCodeEmailRequired              APIErrorCode = "EMAIL_REQUIRED"
	ErrCodeIdentityRequired           APIErrorCode = "IDENTITY_REQUIRED"
	ErrCodeAutoProvisionDisabled      APIErrorCode = "AUTO_PROVISION_DISABLED"
	ErrCodeAutoProvisionMisconfigured APIErrorCode = "AUTO_PROVISION_MISCONFIGURED"

	// Workflow / model capability validation
	ErrCodeModelCapabilityUnsupported APIErrorCode = "MODEL_CAPABILITY_UNSUPPORTED"
)

// APIError captures structured SaaS error metadata.
type APIError struct {
	Status    int
	Code      APIErrorCode
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
	code := e.Code
	if code == "" {
		code = "UNKNOWN"
	}
	msg := e.Message
	if msg == "" {
		msg = fmt.Sprintf("%s (%d)", code, e.Status)
	}
	return fmt.Sprintf("%s: %s", code, msg)
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

// Package-level helper functions for checking error types.

func decodeAPIError(resp *http.Response, retry *RetryMetadata) error {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return APIError{Status: resp.StatusCode, Message: fmt.Sprintf("[failed to read response body: %v]", err), Retry: retry}
	}
	return decodeAPIErrorFromBytes(resp.StatusCode, data, retry)
}

func decodeAPIErrorFromBytes(status int, data []byte, retry *RetryMetadata) error {
	// Some endpoints intentionally return a bare workflow validation error shape
	// (no {code,message} envelope). Surface it as a typed error for callers.
	if status == http.StatusBadRequest {
		var wfErr WorkflowValidationError
		if err := json.Unmarshal(data, &wfErr); err == nil && len(wfErr.Issues) > 0 {
			return wfErr
		}
	}

	apiErr := APIError{Status: status, Retry: retry}
	if len(data) == 0 {
		apiErr.Message = http.StatusText(status)
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
	apiErr.Code = APIErrorCode(payload.Code)
	apiErr.Message = payload.Message
	apiErr.RequestID = payload.RequestID
	apiErr.Fields = payload.Fields
	if apiErr.Message == "" {
		apiErr.Message = http.StatusText(status)
	}
	return apiErr
}
