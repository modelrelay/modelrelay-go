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

projectID := uuid.MustParse(os.Getenv("MODELRELAY_PROJECT_ID"))
customerID := uuid.MustParse(os.Getenv("MODELRELAY_CUSTOMER_ID"))

provider, _ := sdk.NewCustomerTokenProvider(sdk.CustomerTokenProviderConfig{
    SecretKey: secret,
    Request:   sdk.NewCustomerTokenRequestForCustomerID(projectID, customerID),
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
key, _ := sdk.ParseAPIKeyAuth(os.Getenv("MODELRELAY_API_KEY"))
client, _ := sdk.NewClientWithKey(key)

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
key, _ := sdk.ParseAPIKeyAuth(os.Getenv("MODELRELAY_API_KEY"))
client, _ := sdk.NewClientWithKey(key)

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

To stream only text updates (accumulated content in unified NDJSON):

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
key, _ := sdk.ParseAPIKeyAuth(os.Getenv("MODELRELAY_API_KEY"))
client, _ := sdk.NewClientWithKey(key)

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

## Workflow Runs (workflow.v0)

```go
ctx := context.Background()
key, _ := sdk.ParseAPIKeyAuth(os.Getenv("MODELRELAY_API_KEY"))
client, _ := sdk.NewClientWithKey(key)

exec := workflow.ExecutionV0{
	MaxParallelism: sdk.Int64Ptr(3),
	NodeTimeoutMS:  sdk.Int64Ptr(60_000),
	RunTimeoutMS:   sdk.Int64Ptr(180_000),
}

reqA, _, _ := (sdk.ResponseBuilder{}).
	Model(sdk.NewModelID("claude-sonnet-4-20250514")).
	MaxOutputTokens(64).
	System("You are Agent A.").
	User("Analyze the question.").
	Build()
reqB, _, _ := (sdk.ResponseBuilder{}).
	Model(sdk.NewModelID("claude-sonnet-4-20250514")).
	MaxOutputTokens(64).
	System("You are Agent B.").
	User("Find edge cases.").
	Build()
reqC, _, _ := (sdk.ResponseBuilder{}).
	Model(sdk.NewModelID("claude-sonnet-4-20250514")).
	MaxOutputTokens(64).
	System("You are Agent C.").
	User("Propose a solution.").
	Build()
reqAgg, _, _ := (sdk.ResponseBuilder{}).
	Model(sdk.NewModelID("claude-sonnet-4-20250514")).
	MaxOutputTokens(256).
	System("Synthesize the best answer.").
	User(""). // overwritten by bindings
	Build()

b := sdk.WorkflowV0().
	Name("parallel_agents_aggregate").
	Execution(exec)
b, _ = b.LLMResponsesNode("agent_a", reqA, sdk.BoolPtr(false))
b, _ = b.LLMResponsesNode("agent_b", reqB, nil)
b, _ = b.LLMResponsesNode("agent_c", reqC, nil)
b = b.JoinAllNode("join")
b, _ = b.LLMResponsesNodeWithBindings("aggregate", reqAgg, nil, []workflow.LLMResponsesBindingV0{
	{
		From:     "join",
		To:       "/input/1/content/0/text",
		Encoding: workflow.LLMResponsesBindingEncodingJSONString,
	},
})
b = b.
	Edge("agent_a", "join").
	Edge("agent_b", "join").
	Edge("agent_c", "join").
	Edge("join", "aggregate").
	Output("final", "aggregate", "")

spec, _ := b.Build()

created, _ := client.Runs.Create(ctx, spec)

stream, _ := client.Runs.StreamEvents(ctx, created.RunID)
defer stream.Close()
for {
	ev, ok, _ := stream.Next()
	if !ok {
		break
	}
	switch ev.(type) {
	case sdk.RunEventRunCompletedV0:
		status, _ := client.Runs.Get(ctx, created.RunID)
		b, _ := json.MarshalIndent(status.Outputs, "", "  ")
		fmt.Printf("outputs: %s\n", string(b))
	}
}
```

See the full example in `sdk/go/examples/workflows/main.go`.

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

    // Render fields as soon as they're complete
    if event.CompleteFields["title"] {
        renderTitle(*event.Payload.Title)  // Safe to display
    }
    if event.CompleteFields["summary"] {
        renderSummary(*event.Payload.Summary)
    }

    // Show streaming preview of incomplete fields
    if !event.CompleteFields["body"] && event.Payload.Body != nil {
        renderBodyPreview(*event.Payload.Body + "▋")
    }
}
```

## Customer-Attributed Requests

For metered billing, use `CustomerID` — the customer's tier determines the model:

```go
ctx := context.Background()
req, callOpts, _ := client.Responses.New().
    CustomerID("customer-123").
    User("Hello!").
    Build()
resp, _ := client.Responses.Create(ctx, req, callOpts...)
```

## Customer Management (Backend)

```go
// Create/update customer
customer, _ := client.Customers.Upsert(ctx, sdk.CustomerUpsertRequest{
    TierID:     uuid.MustParse("00000000-0000-0000-0000-000000000000"), // Replace with your tier UUID
    ExternalID: sdk.NewCustomerExternalID("your-user-id"),
    Email:      "user@example.com",
})

// Create checkout session for subscription billing
session, _ := client.Customers.CreateCheckoutSession(ctx, customer.ID, sdk.CheckoutSessionRequest{
    SuccessURL: "https://myapp.com/success",
    CancelURL:  "https://myapp.com/cancel",
})

// Check subscription status
status, _ := client.Customers.GetSubscription(ctx, customer.ID)
```

## Plugins (Workflows)

Plugins are GitHub-hosted markdown agents that the Go SDK loads from GitHub, converts to `workflow.v0` via `/responses`, then executes via `/runs` with automatic client-side tool handoff.

```go
ctx := context.Background()
key, _ := sdk.ParseAPIKeyAuth(os.Getenv("MODELRELAY_API_KEY"))
client, _ := sdk.NewClientWithKey(key)

registry := sdk.NewToolRegistry().
    Register("bash", func(args map[string]any, call llm.ToolCall) (any, error) {
        return "ok", nil
    })

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

See `docs/guides/PLUGIN_QUICKSTART.md` for a step-by-step guide.

## Configuration

```go
secret, _ := sdk.ParseSecretKey("mr_sk_...")
client, _ := sdk.NewClientWithKey(secret,
    sdk.WithConnectTimeout(5*time.Second),
    sdk.WithRetryConfig(sdk.RetryConfig{MaxAttempts: 3}),
)
```
