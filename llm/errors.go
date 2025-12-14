// Package llm provides a unified interface for interacting with multiple LLM providers
// including Anthropic, OpenAI, and xAI (Grok). It handles request/response normalization,
// streaming events, tool calling, and error translation across different provider APIs.
package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// StatusClientClosedRequest is the HTTP status code for client-initiated disconnection.
// This follows the nginx convention (499) and indicates the client closed the connection
// before the server finished responding. Using a 4xx status prevents marking providers
// unhealthy for client-side disconnections.
const StatusClientClosedRequest = 499

// Error codes for provider errors.
const (
	ErrorCodeClientClosed    = "CLIENT_CLOSED"
	ErrorCodeProviderTimeout = "PROVIDER_TIMEOUT"
	ErrorCodeProviderError   = "PROVIDER_ERROR"
)

// NewClientClosedError creates a ProviderError for client disconnection.
func NewClientClosedError(providerID string) ProviderError {
	return ProviderError{
		Provider: providerID,
		Status:   StatusClientClosedRequest,
		Code:     ErrorCodeClientClosed,
		Message:  "client closed request",
	}
}

// ProviderError captures normalized upstream failure metadata.
type ProviderError struct {
	Status            int
	Code              string
	Message           string
	ProviderRequestID string
	Provider          string
}

// Error satisfies the error interface.
func (e ProviderError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "provider error"
}

// ValidationError indicates the proxy rejected the request before reaching the provider.
type ValidationError struct {
	msg string
}

// Error implements error.
func (e ValidationError) Error() string {
	return e.msg
}

// NewValidationError returns a ValidationError with formatted message.
func NewValidationError(format string, args ...any) ValidationError {
	return ValidationError{msg: fmt.Sprintf(format, args...)}
}

// ClassifyProviderError maps provider errors onto structured categories.
func ClassifyProviderError(err error) ProviderError {
	defaultErr := ProviderError{
		Status:  http.StatusBadGateway,
		Code:    "PROVIDER_ERROR",
		Message: "provider request failed",
	}

	if err == nil {
		return defaultErr
	}

	var perr ProviderError
	if errors.As(err, &perr) {
		if perr.Status == 0 {
			perr.Status = defaultErr.Status
		}
		if perr.Code == "" {
			perr.Code = defaultErr.Code
		}
		if perr.Message == "" {
			perr.Message = defaultErr.Message
		}
		return perr
	}

	var verr ValidationError
	if errors.As(err, &verr) {
		return ProviderError{
			Status:  http.StatusBadRequest,
			Code:    "INVALID_REQUEST",
			Message: verr.Error(),
		}
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return ProviderError{
			Status:  http.StatusGatewayTimeout,
			Code:    "PROVIDER_TIMEOUT",
			Message: "provider request timed out",
		}
	default:
		return defaultErr
	}
}
