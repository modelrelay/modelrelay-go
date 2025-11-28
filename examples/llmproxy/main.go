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

	stream, err := client.LLM.Chat(sdk.ModelOpenAIGPT51).
		MaxTokens(64).
		System("You are a witty assistant.").
		User("Tell me a short joke").
		MetadataEntry("tenant", "example").
		RequestID("example-stream-1").
		Stream(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer stream.Close()

	for {
		chunk, ok, err := stream.Next()
		if err != nil {
			log.Fatal(err)
		}
		if !ok {
			break
		}
		if chunk.TextDelta != "" {
			log.Printf("delta: %s", chunk.TextDelta)
			continue
		}
		if chunk.Type == llm.StreamEventKindMessageStop && chunk.Usage != nil {
			log.Printf("stop: reason=%s usage=%+v", chunk.StopReason, *chunk.Usage)
			continue
		}
		log.Printf("event=%s data=%s", chunk.Type, string(chunk.Raw.Data))
	}
	log.Printf("request id: %s", stream.RequestID())
}
