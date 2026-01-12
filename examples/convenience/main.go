// Example demonstrating the convenience API for quick prototyping
// and simple use cases.
//
// Run with:
//
//	MODELRELAY_API_KEY=mr_sk_... go run ./examples/convenience
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/modelrelay/modelrelay/sdk/go"
)

func main() {
	apiKey := os.Getenv("MODELRELAY_API_KEY")
	if apiKey == "" {
		log.Fatal("MODELRELAY_API_KEY must be set")
	}

	client, err := sdk.NewClientFromAPIKey(apiKey)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

	// Example 1: Ask — Get a quick answer
	fmt.Println("=== Ask Example ===")
	answer, err := client.Ask(ctx, "claude-sonnet-4-5", "What is 2 + 2?", nil)
	if err != nil {
		cancel()
		log.Fatal(err)
	}
	fmt.Println("Answer:", answer)
	fmt.Println()

	// Example 2: Chat — Full response with metadata
	fmt.Println("=== Chat Example ===")
	response, err := client.Chat(ctx, "claude-sonnet-4-5", "Explain the concept of recursion in one sentence.", &sdk.ChatOptions{
		System: "You are a computer science teacher. Be concise.",
	})
	if err != nil {
		cancel()
		log.Fatal(err)
	}
	fmt.Println("Response:", response.AssistantText())
	fmt.Printf("Tokens: %d input, %d output, %d total\n",
		response.Usage.InputTokens,
		response.Usage.OutputTokens,
		response.Usage.TotalTokens)
	fmt.Println()

	// Example 3: Agent — Agentic tool loop
	fmt.Println("=== Agent Example ===")

	// Define a simple calculator tool
	type CalculateArgs struct {
		Expression string `json:"expression" description:"Math expression to evaluate (e.g., '2 + 2')"`
	}

	tools := sdk.NewToolBuilder()
	sdk.AddFunc(tools, "calculate", "Evaluate a simple math expression", func(args CalculateArgs) (any, error) {
		// In a real app, you'd use a proper expression parser
		// This is just a demo returning a mock result
		return map[string]any{
			"expression": args.Expression,
			"result":     "42",
			"note":       "Demo calculator - always returns 42",
		}, nil
	})

	result, err := client.Agent(ctx, "claude-sonnet-4-5", sdk.AgentOptions{
		Tools:  tools,
		Prompt: "Calculate 6 * 7 using the calculate tool.",
		System: "You are a helpful math assistant. Use the calculate tool when asked to compute expressions.",
	})
	if err != nil {
		cancel()
		log.Fatal(err)
	}

	fmt.Println("Agent output:", result.Output)
	fmt.Printf("Agent usage: %d LLM calls, %d tool calls\n",
		result.Usage.LLMCalls,
		result.Usage.ToolCalls)

	cancel()
}
