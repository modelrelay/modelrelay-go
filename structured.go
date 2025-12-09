package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
	llm "github.com/modelrelay/modelrelay/providers"
)

// StructuredErrorKind identifies the type of structured output error.
type StructuredErrorKind string

const (
	// StructuredErrorKindDecode indicates JSON decoding failed.
	StructuredErrorKindDecode StructuredErrorKind = "decode"
	// StructuredErrorKindValidation indicates schema validation failed.
	StructuredErrorKindValidation StructuredErrorKind = "validation"
)

// ValidationIssue represents a single field-level validation problem.
type ValidationIssue struct {
	// Path is the JSON path to the problematic field (e.g., "person.address.city").
	// nil indicates a root-level error not associated with a specific field.
	Path *string
	// Message describes the validation failure.
	Message string
}

// AttemptRecord captures details of a single structured output attempt.
type AttemptRecord struct {
	// Attempt is the 1-based attempt number.
	Attempt int
	// RawJSON is the raw JSON returned by the model.
	RawJSON string
	// Error describes what went wrong.
	Error StructuredErrorDetail
}

// StructuredErrorDetail holds the specific error information.
type StructuredErrorDetail struct {
	Kind    StructuredErrorKind
	Message string           // For decode errors
	Issues  []ValidationIssue // For validation errors
}

// StructuredDecodeError is returned when JSON decoding fails on the first attempt.
type StructuredDecodeError struct {
	RawJSON string
	Message string
	Attempt int
}

func (e StructuredDecodeError) Error() string {
	return fmt.Sprintf("structured output decode error (attempt %d): %s", e.Attempt, e.Message)
}

// StructuredExhaustedError is returned when all retry attempts are exhausted.
type StructuredExhaustedError struct {
	LastRawJSON string
	AllAttempts []AttemptRecord
	FinalError  StructuredErrorDetail
}

func (e StructuredExhaustedError) Error() string {
	var msg string
	if e.FinalError.Kind == StructuredErrorKindDecode {
		msg = e.FinalError.Message
	} else {
		var parts []string
		for _, issue := range e.FinalError.Issues {
			if issue.Path != nil {
				parts = append(parts, fmt.Sprintf("%s: %s", *issue.Path, issue.Message))
			} else {
				parts = append(parts, issue.Message)
			}
		}
		msg = strings.Join(parts, "; ")
	}
	return fmt.Sprintf("structured output failed after %d attempts: %s", len(e.AllAttempts), msg)
}

// RetryHandler customizes retry behavior when structured output validation fails.
type RetryHandler interface {
	// OnValidationError is called when validation fails. It returns messages to
	// append to the conversation for the retry, or nil to stop retrying.
	OnValidationError(
		attempt int,
		rawJSON string,
		err StructuredErrorDetail,
		originalMessages []llm.ProxyMessage,
	) []llm.ProxyMessage
}

// DefaultRetryHandler appends a simple error correction message on validation failures.
type DefaultRetryHandler struct{}

func (DefaultRetryHandler) OnValidationError(
	_ int,
	_ string,
	err StructuredErrorDetail,
	_ []llm.ProxyMessage,
) []llm.ProxyMessage {
	var errorMsg string
	if err.Kind == StructuredErrorKindDecode {
		errorMsg = err.Message
	} else {
		var parts []string
		for _, issue := range err.Issues {
			if issue.Path != nil {
				parts = append(parts, fmt.Sprintf("%s: %s", *issue.Path, issue.Message))
			} else {
				parts = append(parts, issue.Message)
			}
		}
		errorMsg = strings.Join(parts, "; ")
	}

	return []llm.ProxyMessage{
		{
			Role:    "user",
			Content: fmt.Sprintf("The previous response did not match the expected schema. Error: %s. Please provide a response that matches the schema exactly.", errorMsg),
		},
	}
}

// StructuredOptions configures structured output behavior.
type StructuredOptions struct {
	// MaxRetries is the number of retry attempts on validation failure (default: 0).
	MaxRetries int
	// RetryHandler customizes retry messages. Uses DefaultRetryHandler if nil.
	RetryHandler RetryHandler
	// SchemaName overrides the schema name in response_format (default: "response").
	SchemaName string
}

// validateAgainstSchema validates JSON data against a compiled JSON Schema.
// Returns nil if valid, or a StructuredErrorDetail with validation issues.
func validateAgainstSchema(schema *jsonschema.Schema, rawJSON string) *StructuredErrorDetail {
	var data any
	if err := json.Unmarshal([]byte(rawJSON), &data); err != nil {
		return &StructuredErrorDetail{
			Kind:    StructuredErrorKindDecode,
			Message: err.Error(),
		}
	}

	err := schema.Validate(data)
	if err == nil {
		return nil
	}

	// Convert jsonschema validation errors to our format
	var issues []ValidationIssue
	if validationErr, ok := err.(*jsonschema.ValidationError); ok {
		issues = extractValidationIssues(validationErr)
	} else {
		// Fallback for unexpected error types
		issues = []ValidationIssue{{Message: err.Error()}}
	}

	return &StructuredErrorDetail{
		Kind:   StructuredErrorKindValidation,
		Issues: issues,
	}
}

// extractValidationIssues recursively extracts validation issues from jsonschema errors.
func extractValidationIssues(err *jsonschema.ValidationError) []ValidationIssue {
	var issues []ValidationIssue

	// Add the current error if it has a message
	if err.Message != "" {
		path := err.InstanceLocation
		var pathPtr *string
		if path != "" && path != "#" {
			// Convert JSON pointer format to dot notation for readability
			// InstanceLocation is typically "#/field/subfield" (JSON Pointer with fragment)
			// e.g., "#/name" -> "name", "#/address/city" -> "address.city"
			cleanPath := strings.TrimPrefix(path, "#")
			cleanPath = strings.TrimPrefix(cleanPath, "/")
			cleanPath = strings.ReplaceAll(cleanPath, "/", ".")
			if cleanPath != "" {
				pathPtr = &cleanPath
			}
		}
		issues = append(issues, ValidationIssue{
			Path:    pathPtr,
			Message: err.Message,
		})
	}

	// Recursively collect issues from nested errors
	for _, cause := range err.Causes {
		issues = append(issues, extractValidationIssues(cause)...)
	}

	return issues
}

// compileSchema compiles a JSON Schema for validation.
func compileSchema(schemaJSON json.RawMessage) (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	// Use draft 2020-12 which is the most recent and compatible with OpenAI's schema format
	compiler.Draft = jsonschema.Draft2020

	if err := compiler.AddResource("schema.json", strings.NewReader(string(schemaJSON))); err != nil {
		return nil, fmt.Errorf("failed to add schema resource: %w", err)
	}

	schema, err := compiler.Compile("schema.json")
	if err != nil {
		return nil, fmt.Errorf("failed to compile schema: %w", err)
	}

	return schema, nil
}

// StructuredResult holds a successful structured output result.
type StructuredResult[T any] struct {
	// Value is the parsed, validated result.
	Value T
	// Attempts is the number of attempts made (1 = first attempt succeeded).
	Attempts int
	// RequestID is the server request ID (from the final successful request).
	RequestID string
}

// Structured performs a chat completion with structured output and automatic
// schema generation from type T. It supports retry with validation error feedback.
//
// Unlike ProxyMessage, this function:
//   - Auto-generates the JSON schema from T's struct tags
//   - Sets response_format.type = "json_schema" with strict = true
//   - Automatically decodes and validates the response into T
//   - Optionally retries with error feedback on validation failures
//
// Example:
//
//	type Person struct {
//	    Name string `json:"name"`
//	    Age  int    `json:"age"`
//	}
//
//	result, err := sdk.Structured[Person](ctx, client.LLM, sdk.ProxyRequest{
//	    Model:    sdk.NewModelID("claude-sonnet-4-20250514"),
//	    Messages: []llm.ProxyMessage{{Role: "user", Content: "Extract: John, 30"}},
//	}, sdk.StructuredOptions{MaxRetries: 2})
func Structured[T any](
	ctx context.Context,
	client *LLMClient,
	req ProxyRequest,
	opts StructuredOptions,
	proxyOpts ...ProxyOption,
) (*StructuredResult[T], error) {
	// Generate schema from type
	schemaName := opts.SchemaName
	if schemaName == "" {
		schemaName = "response"
	}

	responseFormat, err := ResponseFormatFromType[T](schemaName)
	if err != nil {
		return nil, fmt.Errorf("failed to generate schema: %w", err)
	}

	// Compile schema for validation
	compiledSchema, err := compileSchema(responseFormat.JSONSchema.Schema)
	if err != nil {
		return nil, fmt.Errorf("failed to compile schema for validation: %w", err)
	}

	// Set response format on request
	req.ResponseFormat = responseFormat

	retryHandler := opts.RetryHandler
	if retryHandler == nil {
		retryHandler = DefaultRetryHandler{}
	}

	var attempts []AttemptRecord
	messages := make([]llm.ProxyMessage, len(req.Messages))
	copy(messages, req.Messages)

	maxAttempts := opts.MaxRetries + 1

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req.Messages = messages

		resp, err := client.ProxyMessage(ctx, req, proxyOpts...)
		if err != nil {
			return nil, err
		}

		// Extract JSON content
		rawJSON := strings.Join(resp.Content, "")

		// Step 1: Validate against JSON Schema first
		if errDetail := validateAgainstSchema(compiledSchema, rawJSON); errDetail != nil {
			attempts = append(attempts, AttemptRecord{
				Attempt: attempt,
				RawJSON: rawJSON,
				Error:   *errDetail,
			})

			// For decode errors on the first attempt with no retries, return StructuredDecodeError
			// This allows callers to type-assert on first-attempt decode failures
			if errDetail.Kind == StructuredErrorKindDecode && attempt == 1 && maxAttempts == 1 {
				return nil, StructuredDecodeError{
					RawJSON: rawJSON,
					Message: errDetail.Message,
					Attempt: attempt,
				}
			}

			// If this was the last attempt, return exhausted error
			if attempt >= maxAttempts {
				return nil, StructuredExhaustedError{
					LastRawJSON: rawJSON,
					AllAttempts: attempts,
					FinalError:  *errDetail,
				}
			}

			// Get retry messages
			retryMsgs := retryHandler.OnValidationError(attempt, rawJSON, *errDetail, req.Messages)
			if retryMsgs == nil {
				return nil, StructuredExhaustedError{
					LastRawJSON: rawJSON,
					AllAttempts: attempts,
					FinalError:  *errDetail,
				}
			}

			// Build messages for retry: original + assistant response + retry feedback
			messages = append(messages, llm.ProxyMessage{Role: "assistant", Content: rawJSON})
			messages = append(messages, retryMsgs...)
			continue
		}

		// Step 2: Decode into target type (should succeed after schema validation)
		var value T
		if err := json.Unmarshal([]byte(rawJSON), &value); err != nil {
			// This should rarely happen if schema validation passed, but handle it
			errDetail := StructuredErrorDetail{
				Kind:    StructuredErrorKindDecode,
				Message: err.Error(),
			}
			attempts = append(attempts, AttemptRecord{
				Attempt: attempt,
				RawJSON: rawJSON,
				Error:   errDetail,
			})

			if attempt >= maxAttempts {
				return nil, StructuredExhaustedError{
					LastRawJSON: rawJSON,
					AllAttempts: attempts,
					FinalError:  errDetail,
				}
			}

			retryMsgs := retryHandler.OnValidationError(attempt, rawJSON, errDetail, req.Messages)
			if retryMsgs == nil {
				return nil, StructuredExhaustedError{
					LastRawJSON: rawJSON,
					AllAttempts: attempts,
					FinalError:  errDetail,
				}
			}

			messages = append(messages, llm.ProxyMessage{Role: "assistant", Content: rawJSON})
			messages = append(messages, retryMsgs...)
			continue
		}

		// Success - both schema validation and type decoding passed
		return &StructuredResult[T]{
			Value:     value,
			Attempts:  attempt,
			RequestID: resp.RequestID,
		}, nil
	}

	// This should be unreachable - if we get here, there's a logic bug in the retry loop
	return nil, fmt.Errorf("internal error: structured output loop exited unexpectedly after %d attempts (this is a bug, please report it)", maxAttempts)
}

// StreamStructured opens a streaming connection for structured JSON outputs.
// Unlike Structured, this does not support retries - validation errors are
// surfaced only on stream completion.
//
// Example:
//
//	type Person struct {
//	    Name string `json:"name"`
//	    Age  int    `json:"age"`
//	}
//
//	stream, err := sdk.StreamStructured[Person](ctx, client.LLM, sdk.ProxyRequest{
//	    Model:    sdk.NewModelID("claude-sonnet-4-20250514"),
//	    Messages: []llm.ProxyMessage{{Role: "user", Content: "Extract: John, 30"}},
//	}, "person")
func StreamStructured[T any](
	ctx context.Context,
	client *LLMClient,
	req ProxyRequest,
	schemaName string,
	proxyOpts ...ProxyOption,
) (*StructuredJSONStream[T], error) {
	if schemaName == "" {
		schemaName = "response"
	}

	responseFormat, err := ResponseFormatFromType[T](schemaName)
	if err != nil {
		return nil, fmt.Errorf("failed to generate schema: %w", err)
	}

	req.ResponseFormat = responseFormat

	return ProxyStreamJSON[T](ctx, client, req, proxyOpts...)
}
