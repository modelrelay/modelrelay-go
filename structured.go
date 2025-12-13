package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llm "github.com/modelrelay/modelrelay/providers"
	"github.com/santhosh-tekuri/jsonschema/v5"
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
	Message string            // For decode errors
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
		originalInput []llm.InputItem,
	) []llm.InputItem
}

// DefaultRetryHandler appends a simple error correction message on validation failures.
type DefaultRetryHandler struct{}

func (DefaultRetryHandler) OnValidationError(
	_ int,
	_ string,
	err StructuredErrorDetail,
	_ []llm.InputItem,
) []llm.InputItem {
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

	return []llm.InputItem{llm.NewUserText(
		fmt.Sprintf("The previous response did not match the expected schema. Error: %s. Please provide a response that matches the schema exactly.", errorMsg),
	)}
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

// Structured performs a /responses request with structured output and automatic
// schema generation from type T. It supports retry with validation error feedback.
//
// Unlike a raw /responses call, this function:
//   - Auto-generates the JSON schema from T's struct tags
//   - Sets output_format.type = "json_schema" with strict = true
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
//	result, err := sdk.Structured[Person](ctx, client.Responses, client.Responses.New().
//	    Model(sdk.NewModelID("claude-sonnet-4-20250514")).
//	    User("Extract: John, 30").
//	    Option(sdk.WithRequestID("demo-1")).
//	    Build(), sdk.StructuredOptions{MaxRetries: 2})
func Structured[T any](
	ctx context.Context,
	client *ResponsesClient,
	req ResponseRequest,
	opts StructuredOptions,
	callOpts ...ResponseOption,
) (*StructuredResult[T], error) {
	// Generate schema from type
	schemaName := opts.SchemaName
	if schemaName == "" {
		schemaName = "response"
	}

	outputFormat, err := OutputFormatFromType[T](schemaName)
	if err != nil {
		return nil, fmt.Errorf("failed to generate schema: %w", err)
	}

	// Compile schema for validation
	compiledSchema, err := compileSchema(outputFormat.JSONSchema.Schema)
	if err != nil {
		return nil, fmt.Errorf("failed to compile schema for validation: %w", err)
	}

	// Set output format on request
	req.outputFormat = outputFormat

	retryHandler := opts.RetryHandler
	if retryHandler == nil {
		retryHandler = DefaultRetryHandler{}
	}

	valueAny, attempts, requestID, err := client.createStructuredWithRetry(
		ctx,
		req,
		opts.MaxRetries,
		retryHandler,
		callOpts,
		func(rawJSON string) *StructuredErrorDetail {
			return validateAgainstSchema(compiledSchema, rawJSON)
		},
		func(rawJSON string) (any, *StructuredErrorDetail) {
			var value T
			if unmarshalErr := json.Unmarshal([]byte(rawJSON), &value); unmarshalErr != nil {
				return nil, &StructuredErrorDetail{
					Kind:    StructuredErrorKindDecode,
					Message: unmarshalErr.Error(),
				}
			}
			return value, nil
		},
	)
	if err != nil {
		return nil, err
	}

	value, ok := valueAny.(T)
	if !ok {
		return nil, fmt.Errorf("internal error: structured decoder returned unexpected type %T (this is a bug, please report it)", valueAny)
	}

	return &StructuredResult[T]{
		Value:     value,
		Attempts:  attempts,
		RequestID: requestID,
	}, nil
}

func (c *ResponsesClient) createStructuredWithRetry(
	ctx context.Context,
	req ResponseRequest,
	maxRetries int,
	retryHandler RetryHandler,
	callOpts []ResponseOption,
	validate func(rawJSON string) *StructuredErrorDetail,
	decode func(rawJSON string) (any, *StructuredErrorDetail),
) (value any, attempts int, requestID string, err error) {
	if retryHandler == nil {
		retryHandler = DefaultRetryHandler{}
	}

	var attemptRecords []AttemptRecord
	input := make([]llm.InputItem, len(req.input))
	copy(input, req.input)

	maxAttempts := maxRetries + 1

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req.input = input

		resp, err := c.Create(ctx, req, callOpts...)
		if err != nil {
			return nil, 0, "", err
		}

		rawJSON := resp.AllText()

		if errDetail := validate(rawJSON); errDetail != nil {
			attemptRecords = append(attemptRecords, AttemptRecord{
				Attempt: attempt,
				RawJSON: rawJSON,
				Error:   *errDetail,
			})

			if errDetail.Kind == StructuredErrorKindDecode && attempt == 1 && maxAttempts == 1 {
				return nil, 0, "", StructuredDecodeError{
					RawJSON: rawJSON,
					Message: errDetail.Message,
					Attempt: attempt,
				}
			}

			if attempt >= maxAttempts {
				return nil, 0, "", StructuredExhaustedError{
					LastRawJSON: rawJSON,
					AllAttempts: attemptRecords,
					FinalError:  *errDetail,
				}
			}

			retryItems := retryHandler.OnValidationError(attempt, rawJSON, *errDetail, req.input)
			if retryItems == nil {
				return nil, 0, "", StructuredExhaustedError{
					LastRawJSON: rawJSON,
					AllAttempts: attemptRecords,
					FinalError:  *errDetail,
				}
			}

			input = append(input, llm.NewAssistantText(rawJSON))
			input = append(input, retryItems...)
			continue
		}

		decoded, decodeErr := decode(rawJSON)
		if decodeErr != nil {
			attemptRecords = append(attemptRecords, AttemptRecord{
				Attempt: attempt,
				RawJSON: rawJSON,
				Error:   *decodeErr,
			})

			if attempt >= maxAttempts {
				return nil, 0, "", StructuredExhaustedError{
					LastRawJSON: rawJSON,
					AllAttempts: attemptRecords,
					FinalError:  *decodeErr,
				}
			}

			retryItems := retryHandler.OnValidationError(attempt, rawJSON, *decodeErr, req.input)
			if retryItems == nil {
				return nil, 0, "", StructuredExhaustedError{
					LastRawJSON: rawJSON,
					AllAttempts: attemptRecords,
					FinalError:  *decodeErr,
				}
			}

			input = append(input, llm.NewAssistantText(rawJSON))
			input = append(input, retryItems...)
			continue
		}

		return decoded, attempt, resp.RequestID, nil
	}

	return nil, 0, "", fmt.Errorf("internal error: structured output loop exited unexpectedly after %d attempts (this is a bug, please report it)", maxAttempts)
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
//	stream, err := sdk.StreamStructured[Person](
//	    ctx,
//	    client.Responses,
//	    client.Responses.New().Model(sdk.NewModelID("claude-sonnet-4-20250514")).User("Extract: John, 30").Build(),
//	    "person",
//	)
func StreamStructured[T any](
	ctx context.Context,
	client *ResponsesClient,
	req ResponseRequest,
	schemaName string,
	callOpts ...ResponseOption,
) (*StructuredJSONStream[T], error) {
	if schemaName == "" {
		schemaName = "response"
	}

	outputFormat, err := OutputFormatFromType[T](schemaName)
	if err != nil {
		return nil, fmt.Errorf("failed to generate schema: %w", err)
	}

	req.outputFormat = outputFormat

	return StreamJSON[T](ctx, client, req, callOpts...)
}
