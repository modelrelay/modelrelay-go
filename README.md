# ModelRelay SDK

The `sdk` module is a thin Go client around the ModelRelay API. It composes the
shared primitives from `billingproxy` and `llmproxy` into a high-level interface for:

- Authentication (login, refresh, API key management).
- End-user billing (checkout sessions, subscriptions, usage tracking).
- LLM proxying (streaming chat completions, unified types).

## Installation

```bash
go get github.com/modelrelay/modelrelay/sdk/go
```

## Usage

Construct a client with your credentials. The client handles token refresh automatically
if initialized with user credentials, or uses the provided API key for server-to-server
access.

```go
import (
    "errors"
    "net/http"
    "os"
    "time"

    llm "github.com/modelrelay/modelrelay/llmproxy"
    "github.com/modelrelay/modelrelay/sdk/go"
)

func main() {
    client, err := sdk.NewClient(sdk.Config{
        APIKey:          os.Getenv("MODELRELAY_API_KEY"),
        Environment:     sdk.EnvironmentStaging,                // production, staging, or sandbox presets
        ClientHeader:    "my-service/1.0",                     // sets X-ModelRelay-Client
        DefaultMetadata: map[string]string{"env": "stg"},    // merged into every chat request
        DefaultHeaders:  http.Header{"X-Debug": []string{"sdk-demo"}},
        ConnectTimeout:  sdk.DurationPtr(5 * time.Second),      // defaults: 5s connect / 60s overall
        RequestTimeout:  sdk.DurationPtr(60 * time.Second),
        Retry:           &sdk.RetryConfig{MaxAttempts: 3},      // exponential backoff + jitter
    })
    // ...
}
```

## Environment Variables

The SDK and CLI examples respect standard environment variables:

- `MODELRELAY_API_KEY` - Secret key for server-side access.
- `MODELRELAY_PUBLISHABLE_KEY` - Public key for frontend token exchange.
- `MODELRELAY_BASE_URL` - API endpoint (defaults to `https://api.modelrelay.ai/api/v1`).

## Examples

See `examples/` for runnable code:

- `examples/cli`: Interactive chat CLI using the streaming proxy.
- `examples/apikeys`: API key management.

### Typed models, providers, and stop reasons

The Go SDK now surfaces typed identifiers for common providers/models and stop
reasons. Unknown strings are preserved and marked as `Other` so you can pass
custom IDs safely:

```go
req := sdk.ProxyRequest{
    Provider: sdk.ProviderOpenAI,
    Model:    sdk.ModelOpenAIGPT51,
    Messages: []llm.ProxyMessage{{Role: "user", Content: "hi"}},
}
resp, _ := client.LLM.ProxyMessage(ctx, req)
if resp.StopReason.IsOther() {
    log.Printf("provider returned custom stop reason %q", resp.StopReason)
}

// Per-call overrides: metadata takes precedence over config defaults, and
// headers set here override defaults on the underlying HTTP request.
resp, _ = client.LLM.ProxyMessage(ctx, req,
	sdk.WithMetadata(map[string]string{"user": "alice", "env": "sandbox"}),
	sdk.WithHeader("X-Debug", "call-123"),
	sdk.WithRequestID("req-123"),
	sdk.WithTimeout(30*time.Second),
	sdk.DisableRetry(),
)

// Error typing: distinguish config vs transport vs API failures.
var apiErr sdk.APIError
var transportErr sdk.TransportError
switch {
case errors.As(err, &apiErr):
	log.Printf("api error status=%d code=%s retry=%+v", apiErr.Status, apiErr.Code, apiErr.Retry)
case errors.As(err, &transportErr):
	log.Printf("transport error: %v (retry=%+v)", transportErr, transportErr.Retry)
}
```

`Usage` now includes optional `cached_tokens` / `reasoning_tokens` fields when
providers emit them, and backfills `total_tokens` if the provider omits it.

### Structured outputs (`response_format`)

Ask providers to return structured JSON instead of free-form text:

```go
schema := json.RawMessage(`{"type":"object","properties":{"headline":{"type":"string"}}}`)
strict := true
req := sdk.ProxyRequest{
    Provider: sdk.ProviderOpenAI,
    Model:    sdk.ModelOpenAIGPT4oMini,
    Messages: []llm.ProxyMessage{{Role: "user", Content: "Summarize ModelRelay"}},
    ResponseFormat: &llm.ResponseFormat{
        Type: llm.ResponseFormatTypeJSONSchema,
        JSONSchema: &llm.JSONSchemaFormat{Name: "summary", Schema: schema, Strict: &strict},
    },
}
resp, _ := client.LLM.ProxyMessage(ctx, req)
fmt.Println(resp.Content[0]) // JSON string matching your schema
```

Providers that don’t support structured outputs are automatically skipped during
routing, and the request falls back to text if `response_format` is omitted.

The CLI in `examples/apikeys` uses the same calls. Provide `MODELRELAY_EMAIL` and
`MODELRELAY_PASSWORD` in the environment to log in, then run `go run ./examples/apikeys`
to create a project key.

### Chat builders & streaming adapter

Fluent builders mirror the TS/Rust ergonomics for both blocking and streaming
calls while keeping access to the underlying SSE events when needed:

```go
// Blocking request with stop sequences for fence suppression
resp, err := client.LLM.Chat(sdk.ModelOpenAIGPT51).
    Provider(sdk.ProviderOpenAI).
    System("You are a concise assistant.").
    User("Explain retrieval-augmented generation in one sentence").
    StopSequences("\n\n```").
    Metadata(map[string]string{"trace_id": "abc123"}).
    RequestID("req-chat-123").
    Send(ctx)

// Streaming adapter emits text deltas and the final usage/stop_reason,
// while chunk.Raw exposes the original StreamEvent payload.
stream, _ := client.LLM.Chat(sdk.ModelOpenAIGPT51).
    User("Tell me a joke about Go SDKs").
    MaxTokens(64).
    RequestID("req-stream-1").
    Stream(ctx)
defer stream.Close()

for {
    chunk, ok, err := stream.Next()
    if err != nil || !ok {
        break
    }
    if chunk.TextDelta != "" {
        fmt.Print(chunk.TextDelta)
        continue
    }
    if chunk.Type == llm.StreamEventKindMessageStop && chunk.Usage != nil {
        fmt.Printf("\nstop=%s usage=%+v\n", chunk.StopReason, *chunk.Usage)
    }
}
```

### Frontend Flow

To issue tokens for a browser/mobile app:

```go
client, _ := sdk.NewClient(sdk.Config{
    APIKey: os.Getenv("MODELRELAY_PUBLISHABLE_KEY"), // mr_pk_xxx
})

// Exchange publishable key + user ID for a short-lived bearer token
token, err := client.Auth.FrontendToken(context.Background(), sdk.FrontendTokenRequest{
    PublishableKey: os.Getenv("MODELRELAY_PUBLISHABLE_KEY"),
    UserID:         "user-123",
})
