package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	llm "github.com/modelrelay/modelrelay/providers"
	"github.com/modelrelay/modelrelay/sdk/go"
)

func main() {
	baseURL := flag.String("base-url", "", "ModelRelay API base URL override")
	model := flag.String("model", "openai/gpt-5.1", "LLM model identifier")
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

	// Build client options
	opts := []sdk.Option{
		sdk.WithClientHeader("modelrelay-cli/1.0"),
		sdk.WithDefaultMetadata(map[string]string{"cli": "true"}),
		sdk.WithDefaultHeaders(http.Header{"X-Debug": []string{"cli-default"}}),
	}
	if *baseURL != "" {
		opts = append(opts, sdk.WithBaseURL(*baseURL))
	}

	client, err := sdk.NewClientWithKey(apiKey, opts...)
	if err != nil {
		log.Fatalf("new client: %v", err)
	}

	proxyOpts := make([]sdk.ProxyOption, 0, 3)
	if *requestID != "" {
		proxyOpts = append(proxyOpts, sdk.WithRequestID(*requestID))
	}
	proxyOpts = append(proxyOpts,
		sdk.WithHeader("X-Debug", "cli-run"),
		sdk.WithMetadataEntry("prompt_length", fmt.Sprintf("%d", len(prompt))))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)

	stream, err := client.LLM.ProxyStream(ctx, sdk.ProxyRequest{
		Model:     sdk.ParseModelID(*model),
		MaxTokens: int64(*maxTokens),
		Messages: []llm.ProxyMessage{{
			Role:    "user",
			Content: prompt,
		}},
		Metadata: map[string]string{"source": "cli"},
	}, proxyOpts...)
	if err != nil {
		cancel()
		log.Fatalf("proxy stream: %v", err)
	}
	defer cancel()
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = stream.Close() }()

	fmt.Printf("Streaming response for request %s...\n", stream.RequestID)
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
		if len(event.Data) > 0 {
			fmt.Printf("[%s] %s\n", event.Kind, event.Data)
		}
	}

	fmt.Println("Done.")
}

func readStdin() string {
	stat, err := os.Stdin.Stat()
	if err != nil || stat.Mode()&os.ModeCharDevice != 0 {
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
