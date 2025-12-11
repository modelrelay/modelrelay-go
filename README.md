# ModelRelay Go SDK

```bash
go get github.com/modelrelay/modelrelay/sdk/go
```

## Streaming Chat

```go
client, _ := sdk.NewClient(sdk.Config{APIKey: os.Getenv("MODELRELAY_API_KEY")})

stream, _ := client.LLM.Chat(sdk.NewModelID("claude-sonnet-4-20250514")).
    System("You are helpful.").
    User("Hello!").
    Stream(ctx)
defer stream.Close()

for {
    chunk, ok, _ := stream.Next()
    if !ok {
        break
    }
    fmt.Print(chunk.TextDelta)
}
```

## Structured Outputs

```go
type Person struct {
    Name string `json:"name"`
    Age  int    `json:"age"`
}

result, _ := sdk.Structured[Person](ctx, client.LLM, sdk.ProxyRequest{
    Model:    sdk.NewModelID("claude-sonnet-4-20250514"),
    Messages: []llm.ProxyMessage{{Role: "user", Content: "Extract: John Doe is 30"}},
}, sdk.StructuredOptions{MaxRetries: 2})

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

stream, _ := sdk.StreamStructured[Article](ctx, client.LLM, sdk.ProxyRequest{
    Model:    sdk.NewModelID("claude-sonnet-4-20250514"),
    Messages: []llm.ProxyMessage{{Role: "user", Content: "Write an article about Go"}},
})
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

For metered billing, use `ChatForCustomer` — the customer's tier determines the model:

```go
resp, _ := client.LLM.ChatForCustomer("customer-123").
    User("Hello!").
    Send(ctx)
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

## Configuration

```go
client, _ := sdk.NewClient(sdk.Config{
    APIKey:         "mr_sk_...",
    Environment:    sdk.EnvironmentProduction,
    ConnectTimeout: sdk.DurationPtr(5 * time.Second),
    Retry:          &sdk.RetryConfig{MaxAttempts: 3},
})
```
