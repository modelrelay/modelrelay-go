package sdk

import (
	"encoding/json"
	"fmt"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

// ============================================================================
// ToolBuilder
// ============================================================================

// ToolBuilder provides a fluent interface for defining tools with both
// JSON Schema definitions (for the API) and handler functions (for execution).
//
// This provides a single source of truth for tool definitions, ensuring that
// the schema and handler stay in sync.
//
// Example:
//
//	type WeatherArgs struct {
//		Location string `json:"location" description:"City name"`
//	}
//
//	tools := sdk.NewToolBuilder().
//		AddFunc("get_weather", "Get current weather", func(args WeatherArgs) (any, error) {
//			return map[string]any{"temp": 72, "unit": "fahrenheit"}, nil
//		}).
//		AddFunc("read_file", "Read a file", func(args struct {
//			Path string `json:"path" description:"File path"`
//		}) (any, error) {
//			return os.ReadFile(args.Path)
//		})
//
//	// Get tool definitions for ResponseBuilder
//	defs := tools.Definitions()
//
//	// Get registry for executing tool calls
//	registry := tools.Registry()
//
//	// Or get both at once
//	defs, registry := tools.Build()
type ToolBuilder struct {
	entries []toolEntry
}

type toolEntry struct {
	name        ToolName
	description string
	tool        llm.Tool
	handler     ToolHandler
}

// NewToolBuilder creates a new empty ToolBuilder.
func NewToolBuilder() *ToolBuilder {
	return &ToolBuilder{}
}

// AddFunc adds a tool with a typed handler function.
//
// The handler function must have the signature:
//
//	func(args T) (any, error)
//	func(args T, call llm.ToolCall) (any, error)
//
// where T is a struct type used to generate the JSON Schema and parse arguments.
//
// Example:
//
//	type SearchArgs struct {
//		Query      string `json:"query" description:"Search query"`
//		MaxResults int    `json:"max_results,omitempty" description:"Max results" default:"10"`
//	}
//
//	builder.AddFunc("search", "Search the web", func(args SearchArgs) (any, error) {
//		return performSearch(args.Query, args.MaxResults), nil
//	})
func AddFunc[T any](b *ToolBuilder, name ToolName, description string, handler func(T) (any, error)) *ToolBuilder {
	tool, err := FunctionToolFromType[T](name, description)
	if err != nil {
		// This should rarely happen since we're generating from a valid Go type.
		// If it does, we'll panic since this is a programming error.
		panic(fmt.Sprintf("failed to create tool schema for %s: %v", name, err))
	}

	// Wrap the typed handler to match ToolHandler signature
	wrappedHandler := func(args map[string]any, call llm.ToolCall) (any, error) {
		// Parse and validate arguments into the typed struct
		var typedArgs T
		argsJSON, err := json.Marshal(args)
		if err != nil {
			return nil, &ToolArgsError{
				Message:      fmt.Sprintf("failed to marshal args: %v", err),
				ToolCallID:   call.ID,
				ToolName:     name,
				RawArguments: call.Function.Arguments,
				Cause:        err,
			}
		}
		if err := json.Unmarshal(argsJSON, &typedArgs); err != nil {
			return nil, &ToolArgsError{
				Message:      fmt.Sprintf("failed to parse args: %v", err),
				ToolCallID:   call.ID,
				ToolName:     name,
				RawArguments: call.Function.Arguments,
				Cause:        err,
			}
		}

		// Validate if the type implements Validator
		if v, ok := any(&typedArgs).(Validator); ok {
			if err := v.Validate(); err != nil {
				return nil, &ToolArgsError{
					Message:      fmt.Sprintf("validation failed: %v", err),
					ToolCallID:   call.ID,
					ToolName:     name,
					RawArguments: call.Function.Arguments,
					Cause:        err,
				}
			}
		}

		return handler(typedArgs)
	}

	b.entries = append(b.entries, toolEntry{
		name:        name,
		description: description,
		tool:        tool,
		handler:     wrappedHandler,
	})
	return b
}

// AddFuncWithCall adds a tool with a typed handler that also receives the ToolCall.
//
// Use this when you need access to the tool call ID or other metadata.
//
// Example:
//
//	builder.AddFuncWithCall("process", "Process data", func(args ProcessArgs, call llm.ToolCall) (any, error) {
//		log.Printf("Processing tool call %s", call.ID)
//		return process(args), nil
//	})
func AddFuncWithCall[T any](b *ToolBuilder, name ToolName, description string, handler func(T, llm.ToolCall) (any, error)) *ToolBuilder {
	tool, err := FunctionToolFromType[T](name, description)
	if err != nil {
		// This should rarely happen since we're generating from a valid Go type.
		// If it does, we'll panic since this is a programming error.
		panic(fmt.Sprintf("failed to create tool schema for %s: %v", name, err))
	}

	// Wrap the typed handler to match ToolHandler signature
	wrappedHandler := func(args map[string]any, call llm.ToolCall) (any, error) {
		// Parse and validate arguments into the typed struct
		var typedArgs T
		argsJSON, err := json.Marshal(args)
		if err != nil {
			return nil, &ToolArgsError{
				Message:      fmt.Sprintf("failed to marshal args: %v", err),
				ToolCallID:   call.ID,
				ToolName:     name,
				RawArguments: call.Function.Arguments,
				Cause:        err,
			}
		}
		if err := json.Unmarshal(argsJSON, &typedArgs); err != nil {
			return nil, &ToolArgsError{
				Message:      fmt.Sprintf("failed to parse args: %v", err),
				ToolCallID:   call.ID,
				ToolName:     name,
				RawArguments: call.Function.Arguments,
				Cause:        err,
			}
		}

		// Validate if the type implements Validator
		if v, ok := any(&typedArgs).(Validator); ok {
			if err := v.Validate(); err != nil {
				return nil, &ToolArgsError{
					Message:      fmt.Sprintf("validation failed: %v", err),
					ToolCallID:   call.ID,
					ToolName:     name,
					RawArguments: call.Function.Arguments,
					Cause:        err,
				}
			}
		}

		return handler(typedArgs, call)
	}

	b.entries = append(b.entries, toolEntry{
		name:        name,
		description: description,
		tool:        tool,
		handler:     wrappedHandler,
	})
	return b
}

// Add adds a tool with a raw ToolHandler.
//
// Use this when you don't want automatic argument parsing, or when you
// need to handle arguments manually.
//
// Example:
//
//	builder.Add("echo", "Echo the input", schema, func(args map[string]any, call llm.ToolCall) (any, error) {
//		return args["message"], nil
//	})
func (b *ToolBuilder) Add(name ToolName, description string, schema json.RawMessage, handler ToolHandler) *ToolBuilder {
	b.entries = append(b.entries, toolEntry{
		name:        name,
		description: description,
		tool: llm.Tool{
			Type: llm.ToolTypeFunction,
			Function: &llm.FunctionTool{
				Name:        name,
				Description: description,
				Parameters:  schema,
			},
		},
		handler: handler,
	})
	return b
}

// Definitions returns the tool definitions for use with ResponseBuilder.Tools().
//
// Example:
//
//	req, opts, err := client.Responses.New().
//		Model("claude-sonnet-4-5").
//		Tools(tools.Definitions()).
//		User("What's the weather in Paris?").
//		Build()
func (b *ToolBuilder) Definitions() []llm.Tool {
	defs := make([]llm.Tool, len(b.entries))
	for i, e := range b.entries {
		defs[i] = e.tool
	}
	return defs
}

// Registry returns a ToolRegistry with all handlers registered.
//
// Example:
//
//	registry := tools.Registry()
//	results := registry.ExecuteAll(response.ToolCalls())
func (b *ToolBuilder) Registry() *ToolRegistry {
	reg := NewToolRegistry()
	for _, e := range b.entries {
		reg.Register(e.name, e.handler)
	}
	return reg
}

// Build returns both definitions and registry.
//
// Example:
//
//	defs, registry := tools.Build()
func (b *ToolBuilder) Build() ([]llm.Tool, *ToolRegistry) {
	return b.Definitions(), b.Registry()
}

// Size returns the number of tools defined.
func (b *ToolBuilder) Size() int {
	return len(b.entries)
}

// Has returns true if a tool with the given name is defined.
func (b *ToolBuilder) Has(name ToolName) bool {
	for _, e := range b.entries {
		if e.name == name {
			return true
		}
	}
	return false
}
