# ModelRelay Go SDK

```bash
go get github.com/modelrelay/modelrelay/sdk/go
```

## Token Providers (Automatic Bearer Auth)

Use token providers when you want the SDK to automatically obtain/refresh **bearer tokens** for data-plane calls like `/responses` and `/runs`.

### Secret key → customer bearer token (mint)

```go
ctx := context.Background()
secret, _ := sdk.ParseSecretKey(os.Getenv("MODELRELAY_API_KEY"))

customerID := uuid.MustParse(os.Getenv("MODELRELAY_CUSTOMER_ID"))

provider, _ := sdk.NewCustomerTokenProvider(sdk.CustomerTokenProviderConfig{
    SecretKey: secret,
    Request:   sdk.NewCustomerTokenRequestForCustomerID(customerID),
})

client, _ := sdk.NewClientWithTokenProvider(provider)

text, _ := client.Responses.Text(ctx, sdk.NewModelID("claude-sonnet-4-20250514"), "You are helpful.", "Hello!")
fmt.Println(text)
```

### OIDC id_token → customer bearer token (exchange)

```go
ctx := context.Background()
key, _ := sdk.ParseAPIKeyAuth(os.Getenv("MODELRELAY_API_KEY"))

provider, _ := sdk.NewOIDCExchangeTokenProvider(sdk.OIDCExchangeTokenProviderConfig{
    APIKey: key,
    IDTokenSource: func(ctx context.Context) (string, error) {
        return os.Getenv("OIDC_ID_TOKEN"), nil
    },
})

client, _ := sdk.NewClientWithTokenProvider(provider)
```

## Responses (Blocking)

```go
ctx := context.Background()
client, _ := sdk.NewClientFromAPIKey(os.Getenv("MODELRELAY_API_KEY"))

req, callOpts, _ := client.Responses.New().
    Model(sdk.NewModelID("claude-sonnet-4-20250514")).
    System("You are helpful.").
    User("Hello!").
    Build()
resp, _ := client.Responses.Create(ctx, req, callOpts...)

fmt.Println(resp.Text())
```

## Chat-Like Text Helpers

For the most common path (**system + user → assistant text**):

```go
ctx := context.Background()
client, _ := sdk.NewClientFromAPIKey(os.Getenv("MODELRELAY_API_KEY"))

text, _ := client.Responses.Text(
    ctx,
    sdk.NewModelID("claude-sonnet-4-20250514"),
    "You are helpful.",
    "Hello!",
)
fmt.Println(text)
```

For customer-attributed requests where the backend selects the model:

```go
text, _ := client.Responses.TextForCustomer(ctx, "customer-123", "You are helpful.", "Hello!")
```

To stream only text deltas:

```go
stream, _ := client.Responses.StreamTextDeltas(
    ctx,
    sdk.NewModelID("claude-sonnet-4-20250514"),
    "You are helpful.",
    "Hello!",
)
defer stream.Close()

for {
    delta, ok, _ := stream.Next()
    if !ok {
        break
    }
    fmt.Print(delta)
}
```

## Streaming Responses

```go
ctx := context.Background()
client, _ := sdk.NewClientFromAPIKey(os.Getenv("MODELRELAY_API_KEY"))

req, callOpts, _ := client.Responses.New().
    Model(sdk.NewModelID("claude-sonnet-4-20250514")).
    System("You are helpful.").
    User("Hello!").
    Build()
stream, _ := client.Responses.Stream(ctx, req, callOpts...)
defer stream.Close()

for {
    event, ok, _ := stream.Next()
    if !ok {
        break
    }
    fmt.Print(event.TextDelta)
}
```

## Workflows

High-level helpers for common workflow patterns:

### Chain (Sequential)

Sequential LLM calls where each step's output feeds the next step's input:

```go
summarizeReq, _, _ := (sdk.ResponseBuilder{}).
    Model(sdk.NewModelID("claude-sonnet-4-20250514")).
    System("Summarize the input concisely.").
    User("The quick brown fox...").
    Build()

translateReq, _, _ := (sdk.ResponseBuilder{}).
    Model(sdk.NewModelID("claude-sonnet-4-20250514")).
    System("Translate the input to French.").
    User(""). // Bound from previous step
    Build()

spec, _ := sdk.Chain("summarize-translate",
    sdk.LLMStep("summarize", summarizeReq),
    sdk.LLMStep("translate", translateReq).WithStream(),
).
    OutputLast("result").
    Build()
```

### Parallel (Fan-out with Aggregation)

Concurrent LLM calls with optional aggregation:

```go
gpt4Req, _, _ := (sdk.ResponseBuilder{}).Model(sdk.NewModelID("gpt-4.1")).User("Analyze this...").Build()
claudeReq, _, _ := (sdk.ResponseBuilder{}).Model(sdk.NewModelID("claude-sonnet-4-20250514")).User("Analyze this...").Build()
synthesizeReq, _, _ := (sdk.ResponseBuilder{}).
    Model(sdk.NewModelID("claude-sonnet-4-20250514")).
    System("Synthesize the analyses into a unified view.").
    User(""). // Bound from join output
    Build()

spec, _ := sdk.Parallel("multi-model-compare",
    sdk.LLMStep("gpt4", gpt4Req),
    sdk.LLMStep("claude", claudeReq),
).
    Aggregate("synthesize", synthesizeReq).
    Output("result", "synthesize").
    Build()
```

### MapReduce (Parallel Map with Reduce)

Process items in parallel, then combine results:

```go
combineReq, _, _ := (sdk.ResponseBuilder{}).
    Model(sdk.NewModelID("claude-sonnet-4-20250514")).
    System("Combine summaries into a cohesive overview.").
    User(""). // Bound from join output
    Build()

spec, _ := sdk.MapReduce("summarize-docs").
    Item("doc1", doc1Req).
    Item("doc2", doc2Req).
    Item("doc3", doc3Req).
    Reduce("combine", combineReq).
    Output("result", "combine").
    Build()
```

## Structured Outputs

```go
type Person struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}

ctx := context.Background()
req, callOpts, _ := client.Responses.New().
    Model(sdk.NewModelID("claude-sonnet-4-20250514")).
    User("Extract: John Doe is 30").
    Build()
result, _ := sdk.Structured[Person](ctx, client.Responses, req, sdk.StructuredOptions{MaxRetries: 2}, callOpts...)

fmt.Printf("Name: %s, Age: %d\n", result.Value.Name, result.Value.Age)
```

## Streaming Structured Outputs

Build progressive UIs that render fields as they complete:

```go
type Article struct {
    Title   string `json:"title"`
    Summary string `json:"summary"`
    Body    string `json:"body"`
}

ctx := context.Background()
req, callOpts, _ := client.Responses.New().
    Model(sdk.NewModelID("claude-sonnet-4-20250514")).
    User("Write an article about Go").
    Build()
stream, _ := sdk.StreamStructured[Article](ctx, client.Responses, req, "", callOpts...)
defer stream.Close()

for {
    event, ok, _ := stream.Next()
    if !ok {
        break
    }
    if event.Payload == nil {
        continue
    }

    // Render fields as soon as they're complete
    if event.CompleteFields["title"] {
        renderTitle(event.Payload.Title)  // Safe to display
    }
    if event.CompleteFields["summary"] {
        renderSummary(event.Payload.Summary)
    }

    // Show streaming preview of incomplete fields
    if !event.CompleteFields["body"] && event.Payload.Body != "" {
        renderBodyPreview(event.Payload.Body + "▋")
    }
}
```

## Customer-Attributed Requests

For metered billing, use `CustomerID` — the customer's subscription tier determines the model:

```go
ctx := context.Background()
req, callOpts, _ := client.Responses.New().
    CustomerID("customer-123").
    User("Hello!").
    Build()
resp, _ := client.Responses.Create(ctx, req, callOpts...)
```

## Plugins (Workflows)

Plugins are GitHub-hosted markdown agents that the Go SDK loads from GitHub, converts to `workflow.v0` via `/responses`, then executes via `/runs` with automatic client-side tool handoff.

Plugin manifests can be `PLUGIN.md` or `SKILL.md`, and plugin URLs can be GitHub `tree/blob/raw` URLs or `github.com/owner/repo@ref/path` canonical URLs.

Client tool names + argument schemas are standardized by the tools.v0 contract: `docs/reference/tools-v0.md` (design notes: `docs/architecture/client-tools.md`).

The Go SDK uses strong types for tool plumbing:
- `sdk.ToolName` for function tool names
- `sdk.ToolCallID` for correlating tool calls/results

```go
ctx := context.Background()
client, _ := sdk.NewClientFromAPIKey(os.Getenv("MODELRELAY_API_KEY"))

workspaceRoot := "."

registry := sdk.NewToolRegistry()

// Safe-by-default workspace tools (tools.v0): root sandbox + traversal prevention + caps.
sdk.NewLocalFSToolPack(workspaceRoot).RegisterInto(registry)

// Opt-in: file writes are deny-by-default (enable explicitly).
sdk.NewLocalWriteFileToolPack(
    workspaceRoot,
    sdk.WithLocalWriteFileAllow(),
).RegisterInto(registry)

// Optional: `bash` is intentionally powerful and deny-by-default.
// Only register it if you need it, and prefer allow-rules over allow-all.
// sdk.NewLocalBashToolPack(
//     workspaceRoot,
//     sdk.WithLocalBashAllowRules(sdk.BashCommandPrefix("git ")),
//     sdk.WithLocalBashAllowEnvVars("PATH"),
// ).RegisterInto(registry)

result, err := client.Plugins().QuickRun(
    ctx,
    "github.com/org/repo/my-plugin",
    "analyze",
    "Review the authentication module",
    sdk.WithToolRegistry(registry),
    sdk.WithPluginModel("claude-opus-4-5-20251101"),
    sdk.WithConverterModel("claude-3-5-haiku-latest"),
)
_ = result
_ = err
```

See `docs/guides/PLUGIN_QUICKSTART.md` for a step-by-step guide, and `docs/architecture/plugins.md` for architecture details. The runnable example lives at `sdk/go/examples/plugins/main.go`.

## Configuration

```go
client, _ := sdk.NewClientFromSecretKey("mr_sk_...",
    sdk.WithConnectTimeout(5*time.Second),
    sdk.WithRetryConfig(sdk.RetryConfig{MaxAttempts: 3}),
)
```
