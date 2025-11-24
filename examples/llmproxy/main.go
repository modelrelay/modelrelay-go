package main

import (
	"context"
	"log"
	"os"
	"time"

	llm "github.com/modelrelay/modelrelay/llmproxy"
	"github.com/modelrelay/modelrelay/sdk/go"
)

func main() {
	apiKey := os.Getenv("MODELRELAY_API_KEY")
	if apiKey == "" {
		log.Fatal("MODELRELAY_API_KEY must be set")
	}
	client, err := sdk.NewClient(sdk.Config{
		APIKey: apiKey,
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	stream, err := client.LLM.ProxyStream(ctx, sdk.ProxyRequest{
		Model:     "openai/gpt-4o-mini",
		MaxTokens: 64,
		Messages: []llm.ProxyMessage{
			{Role: "system", Content: "You are a witty assistant."},
			{Role: "user", Content: "Tell me a short joke"},
		},
		Metadata: map[string]string{"tenant": "example"},
	}, sdk.WithRequestID("example-stream-1"))
	if err != nil {
		log.Fatal(err)
	}
	defer stream.Close()

	for {
		event, ok, err := stream.Next()
		if err != nil {
			log.Fatal(err)
		}
		if !ok {
			break
		}
		log.Printf("%s: %s", event.Kind, string(event.Data))
	}
	log.Printf("request id: %s", stream.RequestID)
}
