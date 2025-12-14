package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/modelrelay/modelrelay/sdk/go"
	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

func main() {
	apiKey := os.Getenv("MODELRELAY_API_KEY")
	if apiKey == "" {
		log.Fatal("MODELRELAY_API_KEY must be set")
	}
	key, err := sdk.ParseAPIKeyAuth(apiKey)
	if err != nil {
		log.Fatal(err)
	}
	client, err := sdk.NewClientWithKey(key)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)

	req, opts, err := client.Responses.New().
		Model(sdk.NewModelID("gpt-5.1")).
		MaxOutputTokens(64).
		System("You are a witty assistant.").
		User("Tell me a short joke").
		RequestID("example-stream-1").
		Build()
	if err != nil {
		cancel()
		log.Fatal(err)
	}
	stream, err := client.Responses.Stream(ctx, req, opts...)
	if err != nil {
		cancel()
		log.Fatal(err)
	}
	defer cancel()
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = stream.Close() }()

	for {
		event, ok, err := stream.Next()
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
		if event.TextDelta != "" {
			log.Printf("delta: %s", event.TextDelta)
			continue
		}
		if event.Kind == llm.StreamEventKindMessageStop && event.Usage != nil {
			log.Printf("stop: reason=%s usage=%+v", event.StopReason, *event.Usage)
			continue
		}
		log.Printf("event=%s data=%s", event.Kind, string(event.Data))
	}
	log.Printf("request id: %s", stream.RequestID)
}
