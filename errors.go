package sdk

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// APIError captures structured SaaS error metadata.
type APIError struct {
	Status    int
	Code      string
	Message   string
	RequestID string
	Fields    []FieldError
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

func decodeAPIError(resp *http.Response) error {
	data, _ := io.ReadAll(resp.Body)
	apiErr := APIError{Status: resp.StatusCode}
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
