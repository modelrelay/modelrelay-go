# ModelRelay Go SDK

```bash
go get github.com/modelrelay/modelrelay/sdk/go
```

## Convenience API

The simplest way to get started. Three methods cover the most common use cases:

### Ask — Get a Quick Answer

```go
client, _ := sdk.NewClientFromAPIKey(os.Getenv("MODELRELAY_API_KEY"))

answer, _ := client.Ask(ctx, "claude-sonnet-4-5", "What is 2 + 2?", nil)
fmt.Println(answer) // "4"
```

### Chat — Full Response with Metadata

```go
response, _ := client.Chat(ctx, "claude-sonnet-4-5", "Explain quantum computing", &sdk.ChatOptions{
    System: "You are a physics professor",
})

fmt.Println(response.AssistantText())
fmt.Println("Tokens:", response.Usage.TotalTokens)
```

### Agent — Agentic Tool Loops

Run an agent that automatically executes tools until completion:

```go
type ReadFileArgs struct {
    Path string `json:"path" description:"File path to read"`
}

tools := sdk.NewToolBuilder()
sdk.AddFunc(tools, "read_file", "Read a file", func(args ReadFileArgs) (any, error) {
    content, err := os.ReadFile(args.Path)
    return string(content), err
})

result, _ := client.Agent(ctx, "claude-sonnet-4-5", sdk.AgentOptions{
    Tools:  tools,
    Prompt: "Read config.json and summarize it",
    System: "You are a helpful file assistant",
})

fmt.Println(result.Output)
fmt.Println("Tool calls:", result.Usage.ToolCalls)
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

text, _ := client.Responses.Text(ctx, sdk.NewModelID("claude-sonnet-4-5"), "You are helpful.", "Hello!")
fmt.Println(text)
```

## Responses (Blocking)

```go
ctx := context.Background()
client, _ := sdk.NewClientFromAPIKey(os.Getenv("MODELRELAY_API_KEY"))

req, callOpts, _ := client.Responses.New().
    Model(sdk.NewModelID("claude-sonnet-4-5")).
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
    sdk.NewModelID("claude-sonnet-4-5"),
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
    sdk.NewModelID("claude-sonnet-4-5"),
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
    Model(sdk.NewModelID("claude-sonnet-4-5")).
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

Build multi-step AI pipelines with the workflow helpers.

### Sequential Chain

```go
spec, _ := sdk.Chain([]sdk.WorkflowIntentNode{
    sdk.LLM("summarize", func(n sdk.LLMNodeBuilder) sdk.LLMNodeBuilder {
        return n.System("Summarize.").User("{{task}}")
    }),
    sdk.LLM("translate", func(n sdk.LLMNodeBuilder) sdk.LLMNodeBuilder {
        return n.System("Translate to French.").User("{{summarize}}")
    }),
}, sdk.ChainOptions{Name: "summarize-translate", Model: "claude-sonnet-4-5"}).
    Output("result", "translate").
    Build()

created, _ := client.Runs.Create(ctx, spec)
```

### Parallel with Aggregation

```go
spec, _ := sdk.Parallel([]sdk.WorkflowIntentNode{
    sdk.LLM("agent_a", func(n sdk.LLMNodeBuilder) sdk.LLMNodeBuilder {
        return n.User("Write 3 ideas for {{task}}.")
    }),
    sdk.LLM("agent_b", func(n sdk.LLMNodeBuilder) sdk.LLMNodeBuilder {
        return n.User("Write 3 objections for {{task}}.")
    }),
}, sdk.ParallelOptions{Name: "multi-agent", Model: "claude-sonnet-4-5"}).
    LLM("aggregate", func(n sdk.LLMNodeBuilder) sdk.LLMNodeBuilder {
        return n.System("Synthesize.").User("{{join}}")
    }).
    Edge("join", "aggregate").
    Output("result", "aggregate").
    Build()
```

### Precompiled Workflows

For workflows that run repeatedly, compile once and reuse:

```go
// Compile once
compiled, _ := client.Workflows().Compile(ctx, spec)

// Run multiple times with different inputs
for _, task := range tasks {
    run, _ := client.Runs.CreateFromPlan(ctx, compiled.PlanHash,
        sdk.WithRunInputs(map[string]any{"task": task}),
    )
}
```

## Structured Outputs

```go
type Person struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}

ctx := context.Background()
req, callOpts, _ := client.Responses.New().
    Model(sdk.NewModelID("claude-sonnet-4-5")).
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
    Model(sdk.NewModelID("claude-sonnet-4-5")).
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

Plugins are GitHub-hosted markdown agents that the Go SDK loads from GitHub, converts to `workflow` via `/responses`, then executes via `/runs` with automatic client-side tool handoff.

Plugin manifests can be `PLUGIN.md` or `SKILL.md`, and plugin URLs can be GitHub `tree/blob/raw` URLs or `github.com/owner/repo@ref/path` canonical URLs.

Client tool names + argument schemas are standardized by the tools.v0 contract: `docs/reference/tools.md` (design notes: `docs/architecture/client-tools.md`).

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
