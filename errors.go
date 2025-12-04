package sdk

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

func decodeAPIError(resp *http.Response, retry *RetryMetadata) error {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return APIError{Status: resp.StatusCode, Message: "failed to read response body", Retry: retry}
	}
	apiErr := APIError{Status: resp.StatusCode}
	apiErr.Retry = retry
	if len(data) == 0 {
		apiErr.Message = resp.Status
		return apiErr
	}
	var payload struct {
		Error struct {
			Code    string       `json:"code"`
			Message string       `json:"message"`
			Status  int          `json:"status"`
			Fields  []FieldError `json:"fields"`
		} `json:"error"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		apiErr.Message = string(data)
		return apiErr
	}
	apiErr.Code = payload.Error.Code
	apiErr.Message = payload.Error.Message
	if payload.Error.Status != 0 {
		apiErr.Status = payload.Error.Status
	}
	apiErr.Fields = payload.Error.Fields
	apiErr.RequestID = payload.RequestID
	if apiErr.Message == "" {
		apiErr.Message = resp.Status
	}
	return apiErr
}
