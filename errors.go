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
	apiErr.Code = payload.Code
	apiErr.Message = payload.Message
	apiErr.RequestID = payload.RequestID
	apiErr.Fields = payload.Fields
	if apiErr.Message == "" {
		apiErr.Message = http.StatusText(status)
	}
	return apiErr
}
