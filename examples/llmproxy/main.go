package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	llm "github.com/modelrelay/modelrelay/providers"
	"github.com/modelrelay/modelrelay/sdk/go"
)

func main() {
	apiKey := os.Getenv("MODELRELAY_API_KEY")
	if apiKey == "" {
		log.Fatal("MODELRELAY_API_KEY must be set")
	}
	client, err := sdk.NewClientWithKey(apiKey)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)

	stream, err := client.LLM.Chat(sdk.ModelGPT51).
		MaxTokens(64).
		System("You are a witty assistant.").
		User("Tell me a short joke").
		MetadataEntry("tenant", "example").
		RequestID("example-stream-1").
		Stream(ctx)
	if err != nil {
		cancel()
		log.Fatal(err)
	}
	defer cancel()
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = stream.Close() }()

	for {
		chunk, ok, err := stream.Next()
		if err != nil {
			//nolint:errcheck // best-effort cleanup before exit
			_ = stream.Close()
			cancel()
			fmt.Fprintf(os.Stderr, "stream error: %v\n", err)
			//nolint:gocritic // os.Exit required for CLI error handling
			os.Exit(1)
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
