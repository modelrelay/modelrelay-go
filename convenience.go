package sdk

import (
	"context"
	"strings"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

// ============================================================================
// Convenience Methods
// ============================================================================

// ChatOptions configures optional settings for Chat and Ask.
type ChatOptions struct {
	// System prompt to prepend to the conversation.
	System string
	// CustomerID for attributed requests (if set, model can be omitted).
	CustomerID string
}

// Chat performs a simple chat completion and returns the full Response.
//
// This is the most ergonomic way to get a response when you need access
// to metadata like usage, model, or stop reason.
//
// Example:
//
//	resp, err := client.Chat(ctx, "claude-sonnet-4-5", "What is 2 + 2?", nil)
//	if err != nil { /* handle */ }
//	fmt.Println(resp.AssistantText())
//	fmt.Println(resp.Usage)
func (c *Client) Chat(ctx context.Context, model string, prompt string, opts *ChatOptions) (*Response, error) {
	if opts == nil {
		opts = &ChatOptions{}
	}

	builder := c.Responses.New().Model(NewModelID(model))

	if opts.CustomerID != "" {
		builder = builder.CustomerID(opts.CustomerID)
	}

	if opts.System != "" {
		builder = builder.System(opts.System)
	}

	builder = builder.User(prompt)

	req, callOpts, err := builder.Build()
	if err != nil {
		return nil, err
	}

	return c.Responses.Create(ctx, req, callOpts...)
}

// Ask performs a simple prompt and returns just the text response.
//
// This is the most ergonomic way to get a quick answer.
//
// Example:
//
//	answer, err := client.Ask(ctx, "claude-sonnet-4-5", "What is 2 + 2?", nil)
//	if err != nil { /* handle */ }
//	fmt.Println(answer) // "4"
func (c *Client) Ask(ctx context.Context, model string, prompt string, opts *ChatOptions) (string, error) {
	resp, err := c.Chat(ctx, model, prompt, opts)
	if err != nil {
		return "", err
	}

	text := resp.AssistantText()
	if strings.TrimSpace(text) == "" {
		return "", TransportError{
			Kind:    TransportErrorEmptyResponse,
			Message: "response contained no assistant text output",
		}
	}

	return text, nil
}

// Agent turn limit constants.
const (
	// DefaultMaxTurns is the default limit for agent turns (100).
	DefaultMaxTurns = 100
	// NoTurnLimit disables the turn limit. Use with caution as this
	// can lead to infinite loops and runaway API costs.
	NoTurnLimit = -1
)

// AgentOptions configures the Agent method.
type AgentOptions struct {
	// Tools is the ToolBuilder containing both tool definitions and handlers.
	// Use NewToolBuilder() and AddFunc() to create tools with type-safe handlers.
	Tools *ToolBuilder
	// Prompt is the user's input.
	Prompt string
	// System is an optional system prompt.
	System string
	// MaxTurns limits the number of LLM calls.
	// Default is 100. Set to NoTurnLimit (-1) for unlimited turns.
	MaxTurns int
}

// AgentResult contains the result of an agent run.
type AgentResult struct {
	// Output is the final text response (if any).
	Output string
	// Usage summarizes token and call counts.
	Usage AgentUsage
	// Response is the final response from the model.
	Response *Response
}

// AgentUsage tracks usage across an agent run.
type AgentUsage struct {
	InputTokens  int64
	OutputTokens int64
	TotalTokens  int64
	LLMCalls     int
	ToolCalls    int
}

// Agent runs an agentic tool loop to completion.
//
// Creates requests with the provided tools and runs until the model
// stops calling tools or MaxTurns is reached.
//
// Example:
//
//	type ReadFileArgs struct {
//		Path string `json:"path" description:"File path to read"`
//	}
//
//	tools := sdk.NewToolBuilder()
//	sdk.AddFunc(tools, "read_file", "Read a file", func(args ReadFileArgs) (any, error) {
//		return os.ReadFile(args.Path)
//	})
//
//	result, err := client.Agent(ctx, "claude-sonnet-4-5", sdk.AgentOptions{
//		Tools:  tools,
//		Prompt: "Read config.json and summarize it",
//	})
func (c *Client) Agent(ctx context.Context, model string, opts AgentOptions) (*AgentResult, error) {
	if opts.Tools == nil {
		return nil, ConfigError{Reason: "Tools (ToolBuilder) is required for Agent"}
	}
	if opts.Prompt == "" {
		return nil, ConfigError{Reason: "Prompt is required for Agent"}
	}

	// Extract definitions and registry from ToolBuilder
	toolDefinitions, toolRegistry := opts.Tools.Build()

	var usage AgentUsage
	var input []llm.InputItem

	// Build initial input
	if opts.System != "" {
		input = append(input, llm.InputItem{
			Type:    llm.InputItemTypeMessage,
			Role:    llm.RoleSystem,
			Content: []llm.ContentPart{llm.TextPart(opts.System)},
		})
	}
	input = append(input, llm.InputItem{
		Type:    llm.InputItemTypeMessage,
		Role:    llm.RoleUser,
		Content: []llm.ContentPart{llm.TextPart(opts.Prompt)},
	})

	modelID := NewModelID(model)
	maxTurns := opts.MaxTurns
	if maxTurns == 0 {
		maxTurns = DefaultMaxTurns
	} else if maxTurns < 0 {
		maxTurns = int(^uint(0) >> 1) // max int - effectively no limit
	}

	var lastResponse *Response

	for turn := 0; turn < maxTurns; turn++ {
		// Build request
		builder := c.Responses.New().
			Model(modelID).
			Input(input)

		if len(toolDefinitions) > 0 {
			builder = builder.Tools(toolDefinitions)
		}

		req, callOpts, err := builder.Build()
		if err != nil {
			return nil, err
		}

		// Make request
		resp, err := c.Responses.Create(ctx, req, callOpts...)
		if err != nil {
			return nil, err
		}

		lastResponse = resp
		usage.LLMCalls++

		// Accumulate usage
		usage.InputTokens += resp.Usage.InputTokens
		usage.OutputTokens += resp.Usage.OutputTokens
		usage.TotalTokens += resp.Usage.TotalTokens

		// Check for tool calls
		toolCalls := resp.ToolCalls()
		if len(toolCalls) == 0 {
			// No tool calls, we're done
			return &AgentResult{
				Output:   resp.AssistantText(),
				Usage:    usage,
				Response: resp,
			}, nil
		}

		// Execute tool calls
		usage.ToolCalls += len(toolCalls)

		// Add assistant message with tool calls to history
		input = append(input, AssistantMessageWithToolCalls(resp.AssistantText(), toolCalls))

		// Execute tools and add results
		results := toolRegistry.ExecuteAll(toolCalls)
		for _, result := range results {
			resultValue := result.Result
			if result.Error != nil {
				resultValue = "Error: " + result.Error.Error()
			}
			msg, err := ToolResultMessage(result.ToolCallID, resultValue)
			if err != nil {
				return nil, err
			}
			input = append(input, msg)
		}
	}

	// Hit max turns without completion - this is an error
	return nil, AgentMaxTurnsError{
		MaxTurns:     maxTurns,
		LastResponse: lastResponse,
		Usage:        usage,
	}
}
