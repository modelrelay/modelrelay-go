# ModelRelay SDK

The `sdk` module is a thin Go client around the ModelRelay API for **consuming** LLM and usage endpoints.

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

    llm "github.com/modelrelay/modelrelay/providers"
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

### Typed models and stop reasons

The Go SDK now surfaces typed identifiers for common stop
reasons, while models are plain strings wrapped in a `ModelID` type:

```go
req := sdk.ProxyRequest{
    Model:    sdk.NewModelID("gpt-5.1"),
    Messages: []llm.ProxyMessage{{Role: "user", Content: "hi"}},
}
resp, _ := client.LLM.ProxyMessage(ctx, req)
if resp.StopReason.IsOther() {
    log.Printf("backend returned custom stop reason %q", resp.StopReason)
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
the backend emits them, and backfills `total_tokens` if it is omitted.

### Structured outputs (`response_format`)

Ask the backend to return structured JSON instead of free-form text:

```go
schema := json.RawMessage(`{"type":"object","properties":{"headline":{"type":"string"}}}`)
strict := true
req := sdk.ProxyRequest{
    Model:    sdk.NewModelID("gpt-4o-mini"),
    Messages: []llm.ProxyMessage{{Role: "user", Content: "Summarize ModelRelay"}},
    ResponseFormat: &llm.ResponseFormat{
        Type: llm.ResponseFormatTypeJSONSchema,
        JSONSchema: &llm.JSONSchemaFormat{Name: "summary", Schema: schema, Strict: &strict},
    },
}
resp, _ := client.LLM.ProxyMessage(ctx, req)
fmt.Println(resp.Content[0]) // JSON string matching your schema
```

Backends that don’t support structured outputs are automatically skipped during
routing, and the request falls back to text if `response_format` is omitted.

### Structured streaming (NDJSON + response_format)

Use the structured streaming contract for `/llm/proxy` to stream JSON payloads
that already conform to your schema:

```go
// Example structured payload – this can be any JSON
// object your application cares about.
type RecommendationPayload struct {
    Items []struct {
        ID    string `json:"id"`
        Label string `json:"label"`
    } `json:"items"`
}

// Build the same ProxyRequest you would use for blocking structured outputs.
req := sdk.ProxyRequest{
    Model:    sdk.NewModelID("grok-4-1-fast"),
    Messages: []llm.ProxyMessage{llm.NewUserMessage("Recommend items for my user")},
    ResponseFormat: &llm.ResponseFormat{
        Type: llm.ResponseFormatTypeJSONSchema,
        JSONSchema: &llm.JSONSchemaFormat{
            Name:   "recommendations",
            Schema: recommendationSchemaJSON, // generated JSON Schema
        },
    },
}

// Stream structured JSON using NDJSON; updates and the final completion are decoded into RecommendationPayload.
stream, err := sdk.ProxyStreamJSON[RecommendationPayload](ctx, client.LLM, req)
if err != nil {
    log.Fatalf("structured stream failed: %v", err)
}
defer stream.Close()

for {
    evt, ok, err := stream.Next()
    if err != nil || !ok {
        break
    }
    switch evt.Type {
    case sdk.StructuredRecordTypeUpdate:
        // Progressive UI: evt.Payload contains a partial but schema-valid payload.
        renderPartialRecommendations(evt.Payload.Items)
    case sdk.StructuredRecordTypeCompletion:
        // Final, authoritative result.
        renderFinalRecommendations(evt.Payload.Items)
    }
}

// Prefer a single blocking result but still want structured streaming semantics?
// Use Collect to wait for the completion record:
finalPayload, err := stream.Collect(ctx)
if err != nil {
    log.Fatalf("collect failed: %v", err)
}
_ = finalPayload.Items
```

The CLI in `examples/apikeys` uses the same calls. Provide `MODELRELAY_EMAIL` and
`MODELRELAY_PASSWORD` in the environment to log in, then run `go run ./examples/apikeys`
to create a project key.

### Chat builders & streaming adapter

Fluent builders mirror the TS/Rust ergonomics for both blocking and streaming
calls while keeping access to the underlying SSE events when needed. Models are
opaque identifiers; the backend selects the cheapest healthy provider for the
requested model automatically.

```go
// Blocking request with stop sequences for fence suppression
resp, err := client.LLM.Chat(sdk.NewModelID("gpt-5.1")).
    System("You are a concise assistant.").
    User("Explain retrieval-augmented generation in one sentence").
    StopSequences("\n\n```").
    Metadata(map[string]string{"trace_id": "abc123"}).
    RequestID("req-chat-123").
    Send(ctx)

// Streaming adapter emits text deltas and the final usage/stop_reason,
// while chunk.Raw exposes the original StreamEvent payload.
stream, _ := client.LLM.Chat(sdk.NewModelID("gpt-5.1")).
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

// Prefer aggregated output from the streaming endpoint? Use Collect.
resp, err := client.LLM.Chat(sdk.NewModelID("gpt-5.1")).
    User("Summarize RAG in 1 sentence").
    RequestID("req-collect-1").
    Collect(ctx)
// resp.Content[0] contains the full text; usage/stop_reason/request_id are filled.
```

### Backend Customer Management

Use a secret key (`mr_sk_*`) to manage customers from your backend:

```go
client, _ := sdk.NewClient(sdk.Config{
    APIKey: os.Getenv("MODELRELAY_API_KEY"), // mr_sk_xxx
})

// Create or update a customer (upsert by external_id)
customer, err := client.Customers.Upsert(context.Background(), sdk.CustomerUpsertRequest{
    TierID:     "your-tier-uuid",
    ExternalID: "github-user-12345",  // your app's user ID
    Email:      "user@example.com",
})

// List all customers
customers, err := client.Customers.List(context.Background())

// Get a specific customer
customer, err := client.Customers.Get(context.Background(), customerUUID)

// Create a checkout session for subscription billing
session, err := client.Customers.CreateCheckoutSession(context.Background(), customer.ID, sdk.CheckoutSessionRequest{
    SuccessURL: "https://myapp.com/billing/success",
    CancelURL:  "https://myapp.com/billing/cancel",
})
// Redirect user to session.URL to complete payment

// Check subscription status
status, err := client.Customers.GetSubscription(context.Background(), customer.ID)
if status.Active {
    // Grant access
}

// Delete a customer
err = client.Customers.Delete(context.Background(), customer.ID)
```

### Frontend Flow

To issue tokens for a browser/mobile app:

```go
client, _ := sdk.NewClient(sdk.Config{
    APIKey: os.Getenv("MODELRELAY_PUBLISHABLE_KEY"), // mr_pk_xxx
})

// Exchange publishable key + customer ID for a short-lived bearer token
token, err := client.Auth.FrontendToken(context.Background(), sdk.FrontendTokenRequest{
    PublishableKey: os.Getenv("MODELRELAY_PUBLISHABLE_KEY"),
    CustomerID:     "customer-123",
})
