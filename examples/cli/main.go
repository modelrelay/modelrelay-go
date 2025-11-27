package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	llm "github.com/modelrelay/modelrelay/llmproxy"
	"github.com/modelrelay/modelrelay/sdk/go"
)

func main() {
	baseURL := flag.String("base-url", "", "ModelRelay API base URL override")
	model := flag.String("model", "openai/gpt-4o-mini", "LLM model identifier")
	maxTokens := flag.Int("max-tokens", 256, "Maximum tokens to request")
	requestID := flag.String("request-id", "", "Optional X-ModelRelay-Chat-Request-Id value")
	flag.Parse()

	prompt := strings.Join(flag.Args(), " ")
	if prompt == "" {
		prompt = readStdin()
	}
	if strings.TrimSpace(prompt) == "" {
		log.Fatal("provide a prompt via CLI args or stdin")
	}

	apiKey := os.Getenv("MODELRELAY_API_KEY")
	if apiKey == "" {
		log.Fatal("MODELRELAY_API_KEY is not set")
	}

	cfg := sdk.Config{APIKey: apiKey}
	if *baseURL != "" {
		cfg.BaseURL = *baseURL
	}

	client, err := sdk.NewClient(cfg)
	if err != nil {
		log.Fatalf("new client: %v", err)
	}

	opts := make([]sdk.ProxyOption, 0, 1)
	if *requestID != "" {
		opts = append(opts, sdk.WithRequestID(*requestID))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	stream, err := client.LLM.ProxyStream(ctx, sdk.ProxyRequest{
		Model:     sdk.ParseModelID(*model),
		MaxTokens: int64(*maxTokens),
		Messages: []llm.ProxyMessage{{
			Role:    "user",
			Content: prompt,
		}},
		Metadata: map[string]string{"cli": "true"},
	}, opts...)
	if err != nil {
		log.Fatalf("proxy stream: %v", err)
	}
	defer stream.Close()

	fmt.Printf("Streaming response for request %s...\n", stream.RequestID)
	for {
		event, ok, err := stream.Next()
		if err != nil {
			log.Fatalf("stream error: %v", err)
		}
		if !ok {
			break
		}
		if len(event.Data) > 0 {
			fmt.Printf("[%s] %s\n", event.Kind, event.Data)
		}
	}

	fmt.Println("Done.")
}

func readStdin() string {
	stat, _ := os.Stdin.Stat()
	if stat.Mode()&os.ModeCharDevice != 0 {
		return ""
	}
	scanner := bufio.NewScanner(os.Stdin)
	var builder strings.Builder
	for scanner.Scan() {
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(scanner.Text())
	}
	return builder.String()
}
