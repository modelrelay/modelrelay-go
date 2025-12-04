package sdk

import (
	"encoding/json"
	"reflect"
	"strings"

	llm "github.com/modelrelay/modelrelay/providers"
)

// ============================================================================
// Message Factory Functions
// ============================================================================

// NewUserMessage creates a user message.
func NewUserMessage(content string) llm.ProxyMessage {
	return llm.ProxyMessage{Role: "user", Content: content}
}

// NewAssistantMessage creates an assistant message.
func NewAssistantMessage(content string) llm.ProxyMessage {
	return llm.ProxyMessage{Role: "assistant", Content: content}
}

// NewSystemMessage creates a system message.
func NewSystemMessage(content string) llm.ProxyMessage {
	return llm.ProxyMessage{Role: "system", Content: content}
}

// ============================================================================
// ToolCall Factory Functions
// ============================================================================

// NewToolCall creates a tool call with the given ID, function name, and arguments.
func NewToolCall(id, name, args string) llm.ToolCall {
	return llm.ToolCall{
		ID:       id,
		Type:     llm.ToolTypeFunction,
		Function: NewFunctionCall(name, args),
	}
}

// NewFunctionCall creates a function call with the given name and arguments.
func NewFunctionCall(name, args string) *llm.FunctionCall {
	return &llm.FunctionCall{Name: name, Arguments: args}
}

// ============================================================================
// Usage Factory Function
// ============================================================================

// NewUsage creates a Usage with the given token counts.
// If total is 0, it will be calculated as input + output.
func NewUsage(input, output, total int64) llm.Usage {
	if total == 0 {
		total = input + output
	}
	return llm.Usage{
		InputTokens:  input,
		OutputTokens: output,
		TotalTokens:  total,
	}
}

// ============================================================================
// Schema Inference from Go Types
// ============================================================================

// SchemaOptions configures JSON Schema generation from Go types.
type SchemaOptions struct {
	// IncludeSchema includes the $schema property in the output.
	IncludeSchema bool
	// SchemaVersion specifies the JSON Schema draft version.
	// Defaults to "draft-07".
	SchemaVersion string
}

// JSONSchema represents a JSON Schema.
type JSONSchema struct {
	Schema               string                 `json:"$schema,omitempty"`
	Type                 string                 `json:"type,omitempty"`
	Properties           map[string]*JSONSchema `json:"properties,omitempty"`
	Required             []string               `json:"required,omitempty"`
	Items                *JSONSchema            `json:"items,omitempty"`
	Enum                 []any                  `json:"enum,omitempty"`
	Description          string                 `json:"description,omitempty"`
	Default              any                    `json:"default,omitempty"`
	Minimum              *float64               `json:"minimum,omitempty"`
	Maximum              *float64               `json:"maximum,omitempty"`
	MinLength            *int                   `json:"minLength,omitempty"`
	MaxLength            *int                   `json:"maxLength,omitempty"`
	MinItems             *int                   `json:"minItems,omitempty"`
	MaxItems             *int                   `json:"maxItems,omitempty"`
	Pattern              string                 `json:"pattern,omitempty"`
	Format               string                 `json:"format,omitempty"`
	AdditionalProperties *JSONSchema            `json:"additionalProperties,omitempty"`
	AnyOf                []*JSONSchema          `json:"anyOf,omitempty"`
}

// TypeToJSONSchema converts a Go type to a JSON Schema.
// The type must be a struct or a pointer to a struct.
//
// Supported struct tags:
//   - json:"name"           - Field name in JSON (standard Go json tag)
//   - json:"-"              - Field is ignored
//   - json:"name,omitempty" - Field is optional
//   - description:"..."     - Field description
//   - enum:"a,b,c"          - Enum values (comma-separated)
//   - default:"value"       - Default value
//   - minimum:"N"           - Minimum value for numbers
//   - maximum:"N"           - Maximum value for numbers
//   - minLength:"N"         - Minimum length for strings
//   - maxLength:"N"         - Maximum length for strings
//   - pattern:"regex"       - Pattern for strings
//   - format:"email"        - Format hint (email, uri, uuid, date-time, etc.)
//
// Example:
//
//	type GetWeatherParams struct {
//	    Location string `json:"location" description:"City name"`
//	    Unit     string `json:"unit" enum:"celsius,fahrenheit" default:"celsius"`
//	}
//
//	schema := TypeToJSONSchema(GetWeatherParams{}, nil)
func TypeToJSONSchema(v any, opts *SchemaOptions) *JSONSchema {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	schema := typeToSchema(t, make(map[reflect.Type]bool))

	if opts != nil && opts.IncludeSchema {
		version := opts.SchemaVersion
		if version == "" {
			version = "draft-07"
		}
		switch version {
		case "draft-04":
			schema.Schema = "http://json-schema.org/draft-04/schema#"
		case "draft-2019-09":
			schema.Schema = "https://json-schema.org/draft/2019-09/schema"
		case "draft-2020-12":
			schema.Schema = "https://json-schema.org/draft/2020-12/schema"
		default:
			schema.Schema = "http://json-schema.org/draft-07/schema#"
		}
	}

	return schema
}

func typeToSchema(t reflect.Type, seen map[reflect.Type]bool) *JSONSchema {
	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		return typeToSchema(t.Elem(), seen)
	}

	// Prevent infinite recursion
	if seen[t] {
		return &JSONSchema{} // Return empty schema for recursive types
	}

	switch t.Kind() {
	case reflect.String:
		return &JSONSchema{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &JSONSchema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return &JSONSchema{Type: "number"}
	case reflect.Bool:
		return &JSONSchema{Type: "boolean"}
	case reflect.Slice, reflect.Array:
		return &JSONSchema{
			Type:  "array",
			Items: typeToSchema(t.Elem(), seen),
		}
	case reflect.Map:
		if t.Key().Kind() == reflect.String {
			return &JSONSchema{
				Type:                 "object",
				AdditionalProperties: typeToSchema(t.Elem(), seen),
			}
		}
		return &JSONSchema{Type: "object"}
	case reflect.Struct:
		// Mark as seen to prevent infinite recursion
		seen[t] = true
		defer delete(seen, t)

		return structToSchema(t, seen)
	case reflect.Interface:
		return &JSONSchema{} // Any type
	default:
		return &JSONSchema{}
	}
}

func structToSchema(t reflect.Type, seen map[reflect.Type]bool) *JSONSchema {
	schema := &JSONSchema{
		Type:       "object",
		Properties: make(map[string]*JSONSchema),
	}

	var required []string

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get JSON field name
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		fieldName := field.Name
		isOptional := false

		if jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				fieldName = parts[0]
			}
			for _, opt := range parts[1:] {
				if opt == "omitempty" {
					isOptional = true
				}
			}
		}

		// Generate schema for this field
		fieldSchema := typeToSchema(field.Type, seen)

		// Apply struct tags
		applyFieldTags(fieldSchema, field)

		// Handle pointer types as optional
		if field.Type.Kind() == reflect.Ptr {
			isOptional = true
		}

		schema.Properties[fieldName] = fieldSchema

		if !isOptional {
			required = append(required, fieldName)
		}
	}

	if len(required) > 0 {
		schema.Required = required
	}

	return schema
}

func applyFieldTags(schema *JSONSchema, field reflect.StructField) {
	// Get the underlying type (unwrap pointers)
	fieldType := field.Type
	for fieldType.Kind() == reflect.Ptr {
		fieldType = fieldType.Elem()
	}

	// Description
	if desc := field.Tag.Get("description"); desc != "" {
		schema.Description = desc
	}

	// Enum - parse values according to field type
	if enumTag := field.Tag.Get("enum"); enumTag != "" {
		values := strings.Split(enumTag, ",")
		schema.Enum = make([]any, len(values))
		for i, v := range values {
			schema.Enum[i] = parseValueForType(strings.TrimSpace(v), fieldType)
		}
	}

	// Default - parse value according to field type
	if defTag := field.Tag.Get("default"); defTag != "" {
		schema.Default = parseValueForType(defTag, fieldType)
	}

	// Minimum/Maximum for numbers
	if minTag := field.Tag.Get("minimum"); minTag != "" {
		if minVal, ok := parseFloat(minTag); ok {
			schema.Minimum = &minVal
		}
	}
	if maxTag := field.Tag.Get("maximum"); maxTag != "" {
		if maxVal, ok := parseFloat(maxTag); ok {
			schema.Maximum = &maxVal
		}
	}

	// MinLength/MaxLength for strings
	if minLenTag := field.Tag.Get("minLength"); minLenTag != "" {
		if minLen, ok := parseInt(minLenTag); ok {
			schema.MinLength = &minLen
		}
	}
	if maxLenTag := field.Tag.Get("maxLength"); maxLenTag != "" {
		if maxLen, ok := parseInt(maxLenTag); ok {
			schema.MaxLength = &maxLen
		}
	}

	// Pattern
	if pattern := field.Tag.Get("pattern"); pattern != "" {
		schema.Pattern = pattern
	}

	// Format
	if format := field.Tag.Get("format"); format != "" {
		schema.Format = format
	}
}

// parseValueForType parses a string value into the appropriate Go type
// based on the reflect.Type, for use in JSON Schema enum/default values.
func parseValueForType(s string, t reflect.Type) any {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if i, ok := parseInt(s); ok {
			return i
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if i, ok := parseInt(s); ok && i >= 0 {
			return i
		}
	case reflect.Float32, reflect.Float64:
		if f, ok := parseFloat(s); ok {
			return f
		}
	case reflect.Bool:
		if b, ok := parseBool(s); ok {
			return b
		}
	default:
		// For other types, return string as-is
	}
	// Default to string for string types or unparseable values
	return s
}

func parseBool(s string) (bool, bool) {
	switch strings.ToLower(s) {
	case "true", "1", "yes":
		return true, true
	case "false", "0", "no":
		return false, true
	}
	return false, false
}

func parseFloat(s string) (float64, bool) {
	var f float64
	err := json.Unmarshal([]byte(s), &f)
	return f, err == nil
}

func parseInt(s string) (int, bool) {
	var i int
	err := json.Unmarshal([]byte(s), &i)
	return i, err == nil
}

// FunctionToolFromType creates a function tool from a Go struct type.
// The struct fields become the tool's parameters, with struct tags
// controlling the JSON Schema generation.
//
// Example:
//
//	type GetWeatherParams struct {
//	    Location string `json:"location" description:"City name"`
//	    Unit     string `json:"unit,omitempty" enum:"celsius,fahrenheit" default:"celsius"`
//	}
//
//	tool, err := FunctionToolFromType[GetWeatherParams]("get_weather", "Get weather for a location")
func FunctionToolFromType[T any](name, description string) (llm.Tool, error) {
	var zero T
	schema := TypeToJSONSchema(zero, nil)

	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return llm.Tool{}, err
	}

	return llm.Tool{
		Type: llm.ToolTypeFunction,
		Function: &llm.FunctionTool{
			Name:        name,
			Description: description,
			Parameters:  schemaBytes,
		},
	}, nil
}

// MustFunctionToolFromType creates a function tool from a Go struct type, panicking on error.
// Useful for static tool definitions.
func MustFunctionToolFromType[T any](name, description string) llm.Tool {
	tool, err := FunctionToolFromType[T](name, description)
	if err != nil {
		panic(err)
	}
	return tool
}

// NewFunctionTool creates a function tool with the given name, description, and JSON schema.
// The schema parameter should be a JSON-encodable value (map, struct, or json.RawMessage).
func NewFunctionTool(name, description string, schema any) (llm.Tool, error) {
	var params json.RawMessage
	if schema != nil {
		switch v := schema.(type) {
		case json.RawMessage:
			params = v
		case []byte:
			params = v
		case string:
			params = json.RawMessage(v)
		default:
			data, err := json.Marshal(schema)
			if err != nil {
				return llm.Tool{}, err
			}
			params = data
		}
	}
	return llm.Tool{
		Type: llm.ToolTypeFunction,
		Function: &llm.FunctionTool{
			Name:        name,
			Description: description,
			Parameters:  params,
		},
	}, nil
}

// MustFunctionTool creates a function tool, panicking on error.
// Useful for static tool definitions.
func MustFunctionTool(name, description string, schema any) llm.Tool {
	tool, err := NewFunctionTool(name, description, schema)
	if err != nil {
		panic(err)
	}
	return tool
}

// NewWebSearchTool creates a web search tool with optional domain filters.
func NewWebSearchTool(allowedDomains, excludedDomains []string, maxUses *int) llm.Tool {
	return llm.Tool{
		Type: llm.ToolTypeWebSearch,
		WebSearch: &llm.WebSearchConfig{
			AllowedDomains:  allowedDomains,
			ExcludedDomains: excludedDomains,
			MaxUses:         maxUses,
		},
	}
}

// ToolChoiceAuto returns a ToolChoice that lets the model decide when to use tools.
func ToolChoiceAuto() *llm.ToolChoice {
	return &llm.ToolChoice{Type: llm.ToolChoiceAuto}
}

// ToolChoiceRequired returns a ToolChoice that forces the model to use a tool.
func ToolChoiceRequired() *llm.ToolChoice {
	return &llm.ToolChoice{Type: llm.ToolChoiceRequired}
}

// ToolChoiceNone returns a ToolChoice that prevents the model from using tools.
func ToolChoiceNone() *llm.ToolChoice {
	return &llm.ToolChoice{Type: llm.ToolChoiceNone}
}

// HasToolCalls returns true if the response contains tool calls.
func (r *ProxyResponse) HasToolCalls() bool {
	return len(r.ToolCalls) > 0
}

// FirstToolCall returns the first tool call, or nil if none exist.
func (r *ProxyResponse) FirstToolCall() *llm.ToolCall {
	if len(r.ToolCalls) == 0 {
		return nil
	}
	return &r.ToolCalls[0]
}

// ToolResultMessage creates a message containing the result of a tool call.
// The result parameter should be a JSON-encodable value or a string.
func ToolResultMessage(toolCallID string, result any) (llm.ProxyMessage, error) {
	var content string
	switch v := result.(type) {
	case string:
		content = v
	case []byte:
		content = string(v)
	case json.RawMessage:
		content = string(v)
	default:
		data, err := json.Marshal(result)
		if err != nil {
			return llm.ProxyMessage{}, err
		}
		content = string(data)
	}
	return llm.ProxyMessage{
		Role:       "tool",
		Content:    content,
		ToolCallID: toolCallID,
	}, nil
}

// MustToolResultMessage creates a tool result message, panicking on error.
func MustToolResultMessage(toolCallID string, result any) llm.ProxyMessage {
	msg, err := ToolResultMessage(toolCallID, result)
	if err != nil {
		panic(err)
	}
	return msg
}

// RespondToToolCall creates a tool result message from a ToolCall.
// Convenience wrapper around ToolResultMessage using the call's ID.
func RespondToToolCall(call llm.ToolCall, result any) (llm.ProxyMessage, error) {
	return ToolResultMessage(call.ID, result)
}

// AssistantMessageWithToolCalls creates an assistant message that includes tool calls.
// This is used to include the assistant's tool-calling turn in conversation history.
func AssistantMessageWithToolCalls(content string, toolCalls []llm.ToolCall) llm.ProxyMessage {
	return llm.ProxyMessage{
		Role:      "assistant",
		Content:   content,
		ToolCalls: toolCalls,
	}
}

// ToolCallAccumulator helps accumulate streaming tool call deltas into complete tool calls.
type ToolCallAccumulator struct {
	calls map[int]*llm.ToolCall
}

// NewToolCallAccumulator creates a new accumulator for streaming tool calls.
func NewToolCallAccumulator() *ToolCallAccumulator {
	return &ToolCallAccumulator{
		calls: make(map[int]*llm.ToolCall),
	}
}

// ProcessDelta processes a streaming tool call delta and returns true if this
// started a new tool call (useful for detecting tool_use_start events).
func (a *ToolCallAccumulator) ProcessDelta(delta *llm.ToolCallDelta) bool {
	if delta == nil {
		return false
	}

	existing, exists := a.calls[delta.Index]
	if !exists {
		// New tool call
		a.calls[delta.Index] = &llm.ToolCall{
			ID:   delta.ID,
			Type: llm.ToolType(delta.Type),
			Function: &llm.FunctionCall{
				Name:      "",
				Arguments: "",
			},
		}
		existing = a.calls[delta.Index]
		if delta.Function != nil {
			existing.Function.Name = delta.Function.Name
			existing.Function.Arguments = delta.Function.Arguments
		}
		return true
	}

	// Append to existing tool call
	if delta.Function != nil {
		if delta.Function.Name != "" {
			existing.Function.Name = delta.Function.Name
		}
		existing.Function.Arguments += delta.Function.Arguments
	}
	return false
}

// GetToolCalls returns all accumulated tool calls in index order.
func (a *ToolCallAccumulator) GetToolCalls() []llm.ToolCall {
	if len(a.calls) == 0 {
		return nil
	}

	// Find max index
	maxIdx := 0
	for idx := range a.calls {
		if idx > maxIdx {
			maxIdx = idx
		}
	}

	// Build result slice
	result := make([]llm.ToolCall, 0, len(a.calls))
	for i := 0; i <= maxIdx; i++ {
		if call, ok := a.calls[i]; ok {
			result = append(result, *call)
		}
	}
	return result
}

// GetToolCall returns a specific tool call by index, or nil if not found.
func (a *ToolCallAccumulator) GetToolCall(index int) *llm.ToolCall {
	if call, ok := a.calls[index]; ok {
		return call
	}
	return nil
}

// Reset clears all accumulated tool calls.
func (a *ToolCallAccumulator) Reset() {
	a.calls = make(map[int]*llm.ToolCall)
}

// ============================================================================
// Type-safe Argument Parsing
// ============================================================================

// ToolArgsError is returned when tool argument parsing or validation fails.
// Contains a descriptive message suitable for sending back to the model.
type ToolArgsError struct {
	Message      string // Human-readable error message
	ToolCallID   string // The tool call ID for correlation
	ToolName     string // The tool name that was called
	RawArguments string // The raw arguments string that failed to parse
	Cause        error  // The underlying error (JSON parse error, validation error, etc.)
}

func (e *ToolArgsError) Error() string {
	return e.Message
}

func (e *ToolArgsError) Unwrap() error {
	return e.Cause
}

// ParseToolArgs parses and unmarshals tool call arguments into the target struct.
// The target must be a pointer to a struct with json tags.
//
// Example:
//
//	type WeatherArgs struct {
//	    Location string `json:"location"`
//	    Unit     string `json:"unit"`
//	}
//
//	var args WeatherArgs
//	if err := sdk.ParseToolArgs(toolCall, &args); err != nil {
//	    // Handle error - message is suitable for sending back to model
//	}
//	fmt.Println(args.Location) // Use typed args
func ParseToolArgs(call llm.ToolCall, target any) error {
	toolName := ""
	rawArgs := ""
	if call.Function != nil {
		toolName = call.Function.Name
		rawArgs = call.Function.Arguments
	}

	// Handle empty arguments
	if rawArgs == "" {
		rawArgs = "{}"
	}

	if err := json.Unmarshal([]byte(rawArgs), target); err != nil {
		return &ToolArgsError{
			Message:      "failed to parse arguments for tool '" + toolName + "': " + err.Error(),
			ToolCallID:   call.ID,
			ToolName:     toolName,
			RawArguments: rawArgs,
			Cause:        err,
		}
	}

	return nil
}

// MustParseToolArgs is like ParseToolArgs but panics on error.
// Useful in contexts where argument parsing should never fail.
func MustParseToolArgs(call llm.ToolCall, target any) {
	if err := ParseToolArgs(call, target); err != nil {
		panic(err)
	}
}

// ParseToolArgsMap parses tool call arguments into a map[string]any.
// Useful when you don't have a predefined struct or need dynamic access.
//
// Example:
//
//	args, err := sdk.ParseToolArgsMap(toolCall)
//	if err != nil {
//	    // Handle error
//	}
//	location := args["location"].(string)
func ParseToolArgsMap(call llm.ToolCall) (map[string]any, error) {
	var args map[string]any
	if err := ParseToolArgs(call, &args); err != nil {
		return nil, err
	}
	if args == nil {
		args = make(map[string]any)
	}
	return args, nil
}

// Validator is an interface for types that can validate themselves.
// Implement this interface on your args struct for custom validation.
//
// Example:
//
//	type WeatherArgs struct {
//	    Location string `json:"location"`
//	    Unit     string `json:"unit"`
//	}
//
//	func (a WeatherArgs) Validate() error {
//	    if a.Location == "" {
//	        return errors.New("location is required")
//	    }
//	    if a.Unit != "" && a.Unit != "celsius" && a.Unit != "fahrenheit" {
//	        return fmt.Errorf("unit must be 'celsius' or 'fahrenheit', got '%s'", a.Unit)
//	    }
//	    return nil
//	}
type Validator interface {
	Validate() error
}

// ParseAndValidateToolArgs parses arguments and validates them if the target
// implements the Validator interface.
//
// Example:
//
//	var args WeatherArgs // WeatherArgs implements Validator
//	if err := sdk.ParseAndValidateToolArgs(toolCall, &args); err != nil {
//	    // err.Message is suitable for sending back to model
//	}
func ParseAndValidateToolArgs(call llm.ToolCall, target any) error {
	if err := ParseToolArgs(call, target); err != nil {
		return err
	}

	// Check if target implements Validator
	// We need to handle both pointer and value receivers
	if v, ok := target.(Validator); ok {
		if err := v.Validate(); err != nil {
			toolName := ""
			rawArgs := ""
			if call.Function != nil {
				toolName = call.Function.Name
				rawArgs = call.Function.Arguments
			}
			return &ToolArgsError{
				Message:      "invalid arguments for tool '" + toolName + "': " + err.Error(),
				ToolCallID:   call.ID,
				ToolName:     toolName,
				RawArguments: rawArgs,
				Cause:        err,
			}
		}
	}

	return nil
}

// ============================================================================
// Tool Registry
// ============================================================================

// ToolHandler is a function that handles a tool call.
// It receives the parsed arguments as a map and the original tool call.
// Returns a result (any JSON-serializable value) or an error.
type ToolHandler func(args map[string]any, call llm.ToolCall) (any, error)

// ToolExecutionResult contains the result of executing a tool call.
type ToolExecutionResult struct {
	ToolCallID string
	ToolName   string
	Result     any
	Error      error
	// IsRetryable is true if the error is due to malformed arguments (JSON parse or validation failure)
	// and the model should be given a chance to retry with corrected arguments.
	IsRetryable bool
}

// ToolRegistry maps tool names to handler functions for automatic dispatch.
//
// Example usage:
//
//	registry := sdk.NewToolRegistry().
//		Register("get_weather", func(args map[string]any, call llm.ToolCall) (any, error) {
//			location := args["location"].(string)
//			return map[string]any{"temp": 72, "unit": "fahrenheit"}, nil
//		}).
//		Register("search", func(args map[string]any, call llm.ToolCall) (any, error) {
//			query := args["query"].(string)
//			return []string{"result1", "result2"}, nil
//		})
//
//	results := registry.ExecuteAll(response.ToolCalls)
//	messages := registry.ResultsToMessages(results)
type ToolRegistry struct {
	handlers map[string]ToolHandler
}

// NewToolRegistry creates a new tool registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		handlers: make(map[string]ToolHandler),
	}
}

// Register adds a handler for the given tool name.
// Returns the registry for method chaining.
func (r *ToolRegistry) Register(name string, handler ToolHandler) *ToolRegistry {
	r.handlers[name] = handler
	return r
}

// Unregister removes the handler for the given tool name.
// Returns true if a handler was removed.
func (r *ToolRegistry) Unregister(name string) bool {
	if _, ok := r.handlers[name]; ok {
		delete(r.handlers, name)
		return true
	}
	return false
}

// Has returns true if a handler is registered for the given tool name.
func (r *ToolRegistry) Has(name string) bool {
	_, ok := r.handlers[name]
	return ok
}

// RegisteredTools returns a list of registered tool names.
func (r *ToolRegistry) RegisteredTools() []string {
	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}
	return names
}

// Execute runs the handler for a single tool call.
func (r *ToolRegistry) Execute(call llm.ToolCall) ToolExecutionResult {
	toolName := ""
	if call.Function != nil {
		toolName = call.Function.Name
	}

	handler, ok := r.handlers[toolName]
	if !ok {
		return ToolExecutionResult{
			ToolCallID: call.ID,
			ToolName:   toolName,
			Error:      &UnknownToolError{ToolName: toolName, Available: r.RegisteredTools()},
		}
	}

	// Parse arguments
	var args map[string]any
	if call.Function != nil && call.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			return ToolExecutionResult{
				ToolCallID:  call.ID,
				ToolName:    toolName,
				Error:       err,
				IsRetryable: true, // JSON parse errors are retryable
			}
		}
	}
	if args == nil {
		args = make(map[string]any)
	}

	// Execute handler
	result, err := handler(args, call)
	if err != nil {
		// Check if error is a ToolArgsError (validation failure) - these are retryable
		_, isArgsError := err.(*ToolArgsError)
		return ToolExecutionResult{
			ToolCallID:  call.ID,
			ToolName:    toolName,
			Result:      result,
			Error:       err,
			IsRetryable: isArgsError,
		}
	}
	return ToolExecutionResult{
		ToolCallID: call.ID,
		ToolName:   toolName,
		Result:     result,
	}
}

// ExecuteAll runs handlers for multiple tool calls.
// Results are returned in the same order as the input calls.
func (r *ToolRegistry) ExecuteAll(calls []llm.ToolCall) []ToolExecutionResult {
	results := make([]ToolExecutionResult, len(calls))
	for i, call := range calls {
		results[i] = r.Execute(call)
	}
	return results
}

// ResultsToMessages converts execution results to tool result messages.
// Useful for appending to the conversation history.
func (r *ToolRegistry) ResultsToMessages(results []ToolExecutionResult) []llm.ProxyMessage {
	messages := make([]llm.ProxyMessage, len(results))
	for i, res := range results {
		var content string
		if res.Error != nil {
			content = "Error: " + res.Error.Error()
		} else {
			switch v := res.Result.(type) {
			case string:
				content = v
			default:
				data, err := json.Marshal(res.Result)
				if err != nil {
					content = "Error: failed to marshal tool result"
				} else {
					content = string(data)
				}
			}
		}
		messages[i] = llm.ProxyMessage{
			Role:       "tool",
			Content:    content,
			ToolCallID: res.ToolCallID,
		}
	}
	return messages
}

// UnknownToolError is returned when a tool call references an unregistered tool.
type UnknownToolError struct {
	ToolName  string
	Available []string
}

func (e *UnknownToolError) Error() string {
	if len(e.Available) == 0 {
		return "unknown tool: '" + e.ToolName + "'. No tools registered."
	}
	return "unknown tool: '" + e.ToolName + "'. Available: " + joinStrings(e.Available, ", ")
}

func joinStrings(s []string, sep string) string {
	if len(s) == 0 {
		return ""
	}
	result := s[0]
	for i := 1; i < len(s); i++ {
		result += sep + s[i]
	}
	return result
}

// ============================================================================
// Retry Utilities
// ============================================================================

// FormatToolErrorForModel formats a tool execution error into a message suitable
// for sending back to the model. The message is designed to help the model
// understand what went wrong and correct it.
func FormatToolErrorForModel(result ToolExecutionResult) string {
	msg := "Tool call error for '" + result.ToolName + "': " + result.Error.Error()
	if result.IsRetryable {
		msg += "\n\nPlease correct the arguments and try again."
	}
	return msg
}

// HasRetryableErrors returns true if any results have retryable errors.
func HasRetryableErrors(results []ToolExecutionResult) bool {
	for _, r := range results {
		if r.Error != nil && r.IsRetryable {
			return true
		}
	}
	return false
}

// GetRetryableErrors filters results to only those with retryable errors.
func GetRetryableErrors(results []ToolExecutionResult) []ToolExecutionResult {
	var retryable []ToolExecutionResult
	for _, r := range results {
		if r.Error != nil && r.IsRetryable {
			retryable = append(retryable, r)
		}
	}
	return retryable
}

// CreateRetryMessages creates tool result messages for retryable errors,
// formatted to help the model correct them.
func CreateRetryMessages(results []ToolExecutionResult) []llm.ProxyMessage {
	retryable := GetRetryableErrors(results)
	messages := make([]llm.ProxyMessage, len(retryable))
	for i, r := range retryable {
		messages[i] = llm.ProxyMessage{
			Role:       "tool",
			Content:    FormatToolErrorForModel(r),
			ToolCallID: r.ToolCallID,
		}
	}
	return messages
}

// RetryCallback is invoked when a retryable error occurs.
// It should return new tool calls from the model's response.
// If it returns nil or empty slice, retry will stop.
type RetryCallback func(errorMessages []llm.ProxyMessage, attempt int) ([]llm.ToolCall, error)

// RetryOptions configures the retry behavior.
type RetryOptions struct {
	// MaxRetries is the maximum number of retry attempts for parse/validation errors.
	// Default: 2
	MaxRetries int
	// OnRetry is called when a retryable error occurs.
	// If nil, ExecuteWithRetry will not retry automatically.
	OnRetry RetryCallback
}

// ExecuteWithRetry executes tool calls with automatic retry on parse/validation errors.
//
// This is a higher-level utility that wraps registry.ExecuteAll with retry logic.
// When a retryable error occurs, it calls the OnRetry callback to get new tool calls
// from the model and continues execution.
//
// Result Preservation: Successful results are preserved across retries. If you
// execute multiple tool calls and only some fail, the successful results are kept
// and merged with the results from retry attempts. Results are keyed by ToolCallID,
// so if a retry returns a call with the same ID as a previous result, the newer
// result will replace it.
//
// Example:
//
//	results, err := sdk.ExecuteWithRetry(registry, toolCalls, sdk.RetryOptions{
//	    MaxRetries: 2,
//	    OnRetry: func(errorMessages []llm.ProxyMessage, attempt int) ([]llm.ToolCall, error) {
//	        // Add error messages to conversation and call the model again
//	        messages = append(messages, sdk.AssistantMessageWithToolCalls("", toolCalls))
//	        messages = append(messages, errorMessages...)
//	        resp, err := client.Chat(ctx, messages, opts)
//	        if err != nil {
//	            return nil, err
//	        }
//	        return resp.ToolCalls, nil
//	    },
//	})
func ExecuteWithRetry(registry *ToolRegistry, toolCalls []llm.ToolCall, opts RetryOptions) ([]ToolExecutionResult, error) {
	maxRetries := opts.MaxRetries
	if maxRetries == 0 {
		maxRetries = 2
	}

	currentCalls := toolCalls
	attempt := 0

	// Track successful results across retries, keyed by ToolCallID
	successfulResults := make(map[string]ToolExecutionResult)

	for attempt <= maxRetries {
		results := registry.ExecuteAll(currentCalls)

		// Store successful results (non-error or non-retryable error)
		for _, result := range results {
			if result.Error == nil || !result.IsRetryable {
				successfulResults[result.ToolCallID] = result
			}
		}

		// Check for retryable errors
		retryableResults := GetRetryableErrors(results)
		if len(retryableResults) == 0 || opts.OnRetry == nil {
			// No more retries needed - include any remaining retryable errors
			for _, result := range results {
				if result.Error != nil && result.IsRetryable {
					successfulResults[result.ToolCallID] = result
				}
			}
			return mapToSlice(successfulResults), nil
		}

		attempt++
		if attempt > maxRetries {
			// Max retries exhausted - include final failed results
			for _, result := range retryableResults {
				successfulResults[result.ToolCallID] = result
			}
			return mapToSlice(successfulResults), nil
		}

		// Create error messages and get new tool calls
		errorMessages := CreateRetryMessages(retryableResults)
		newCalls, err := opts.OnRetry(errorMessages, attempt)
		if err != nil {
			// On callback error, include final failed results
			for _, result := range retryableResults {
				successfulResults[result.ToolCallID] = result
			}
			return mapToSlice(successfulResults), err
		}

		if len(newCalls) == 0 {
			// No new calls to retry - include final failed results
			for _, result := range retryableResults {
				successfulResults[result.ToolCallID] = result
			}
			return mapToSlice(successfulResults), nil
		}

		currentCalls = newCalls
	}

	return mapToSlice(successfulResults), nil
}

// mapToSlice converts a map of results to a slice.
func mapToSlice(m map[string]ToolExecutionResult) []ToolExecutionResult {
	results := make([]ToolExecutionResult, 0, len(m))
	for _, v := range m {
		results = append(results, v)
	}
	return results
}
