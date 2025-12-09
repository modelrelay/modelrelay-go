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

### Typed models, stop reasons, and message roles

The Go SDK surfaces typed identifiers for common stop
reasons and message roles. Models are plain strings wrapped in a `ModelID` type:

```go
req := sdk.ProxyRequest{
    Model:    sdk.NewModelID("gpt-5.1"),
    Messages: []llm.ProxyMessage{{Role: llm.RoleUser, Content: "hi"}},
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

### Type-safe structured outputs with automatic schema inference

For a more ergonomic approach, use `sdk.Structured[T]()` to automatically generate
JSON schemas from Go struct types and validate responses:

```go
import (
    "context"
    "fmt"
    "log"

    llm "github.com/modelrelay/modelrelay/providers"
    "github.com/modelrelay/modelrelay/sdk/go"
)

// Define your output type with JSON tags
type Person struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}

func main() {
    ctx := context.Background()
    client, _ := sdk.NewClient(sdk.Config{APIKey: "mr_sk_..."})

    // Structured[T] auto-generates the schema and validates the response
    result, err := sdk.Structured[Person](ctx, client.LLM, sdk.ProxyRequest{
        Model:    sdk.NewModelID("claude-sonnet-4-20250514"),
        Messages: []llm.ProxyMessage{
            {Role: "user", Content: "Extract: John Doe is 30 years old"},
        },
    }, sdk.StructuredOptions{
        MaxRetries: 2, // Retry on validation failures
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Name: %s, Age: %d\n", result.Value.Name, result.Value.Age)
    fmt.Printf("Succeeded on attempt %d\n", result.Attempts)
}
```

#### Schema generation from struct tags

The SDK generates JSON schemas from Go types using reflection:

```go
type Status struct {
    // Basic fields become required properties
    Code string `json:"code"`

    // Pointer fields are optional (not in "required" array)
    Notes *string `json:"notes"`

    // Use description tag for field documentation
    Email string `json:"email" description:"User's email address"`

    // Enum constraint
    Priority string `json:"priority" enum:"low,medium,high"`

    // Format constraint (email, uri, date-time, etc.)
    Website string `json:"website" format:"uri"`
}

// Nested structs are fully supported
type Order struct {
    ID       string   `json:"id"`
    Customer Person   `json:"customer"` // Nested object
    Items    []string `json:"items"`    // Array type
}
```

#### Handling validation errors

When validation fails, you get detailed error information:

```go
result, err := sdk.Structured[Person](ctx, client.LLM, req, sdk.StructuredOptions{
    MaxRetries: 2,
})
if err != nil {
    var exhausted sdk.StructuredExhaustedError
    if errors.As(err, &exhausted) {
        // All retries failed
        fmt.Printf("Failed after %d attempts\n", len(exhausted.AllAttempts))
        for _, attempt := range exhausted.AllAttempts {
            fmt.Printf("Attempt %d: %s\n", attempt.Attempt, attempt.RawJSON)
            for _, issue := range attempt.Error.Issues {
                if issue.Path != nil {
                    fmt.Printf("  - %s: %s\n", *issue.Path, issue.Message)
                } else {
                    fmt.Printf("  - %s\n", issue.Message)
                }
            }
        }
    }
}
```

#### Custom retry handlers

Customize retry behavior with your own handler:

```go
type MyRetryHandler struct{}

func (h MyRetryHandler) OnValidationError(
    attempt int,
    rawJSON string,
    err sdk.StructuredErrorDetail,
    messages []llm.ProxyMessage,
) []llm.ProxyMessage {
    if attempt >= 3 {
        return nil // Stop retrying
    }
    // Custom retry message
    return []llm.ProxyMessage{{
        Role:    "user",
        Content: fmt.Sprintf("Invalid response. Issues: %v. Try again.", err.Issues),
    }}
}

result, err := sdk.Structured[Person](ctx, client.LLM, req, sdk.StructuredOptions{
    MaxRetries:   3,
    RetryHandler: MyRetryHandler{},
})
```

#### Streaming structured outputs

For streaming with schema inference (no retries):

```go
stream, err := sdk.StreamStructured[Person](ctx, client.LLM, sdk.ProxyRequest{
    Model:    sdk.NewModelID("claude-sonnet-4-20250514"),
    Messages: []llm.ProxyMessage{{Role: "user", Content: "Extract: Jane, 25"}},
}, "person")
if err != nil {
    log.Fatal(err)
}
defer stream.Close()

for {
    evt, ok, err := stream.Next()
    if err != nil || !ok {
        break
    }
    if evt.Type == sdk.StructuredRecordTypeCompletion {
        fmt.Printf("Final: %+v\n", evt.Payload)
    }
}
```

The CLI in `examples/apikeys` uses the same calls. Provide `MODELRELAY_EMAIL` and
`MODELRELAY_PASSWORD` in the environment to log in, then run `go run ./examples/apikeys`
to create a project key.

### Message roles

Use typed constants for message roles instead of strings:

```go
// Available role constants
llm.RoleUser      // "user"
llm.RoleAssistant // "assistant"
llm.RoleSystem    // "system"
llm.RoleTool      // "tool"

// Using with builders
resp, _ := client.LLM.Chat(sdk.NewModelID("gpt-4o")).
    Message(llm.RoleSystem, "You are helpful.").
    Message(llm.RoleUser, "Hello!").
    Send(ctx)

// Or use convenience methods (recommended)
resp, _ := client.LLM.Chat(sdk.NewModelID("gpt-4o")).
    System("You are helpful.").
    User("Hello!").
    Send(ctx)
```

### Customer-attributed requests

For customer-attributed requests, the customer's tier determines which model to use.
Use `ChatForCustomer` instead of `Chat`:

```go
// Customer-attributed: tier determines model, no model parameter needed
resp, err := client.LLM.ChatForCustomer("customer-123").
    System("You are a helpful assistant.").
    User("Hello!").
    Send(ctx)

// The customer ID is sent via the X-ModelRelay-Customer-Id header.
// The backend looks up the customer's tier and routes to the appropriate model.

// Streaming works the same way
stream, err := client.LLM.ChatForCustomer("customer-123").
    User("Tell me a joke").
    Stream(ctx)
```

This provides compile-time separation between:
- **Direct/PAYGO requests** (`Chat(model)`) — model is required
- **Customer-attributed requests** (`ChatForCustomer(customerId)`) — tier determines model

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
