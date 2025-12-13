# ModelRelay Go SDK

```bash
go get github.com/modelrelay/modelrelay/sdk/go
```

## Responses (Blocking)

```go
ctx := context.Background()
client, _ := sdk.NewClientWithKey(os.Getenv("MODELRELAY_API_KEY"))

req, callOpts, _ := client.Responses.New().
    Model(sdk.NewModelID("claude-sonnet-4-20250514")).
    System("You are helpful.").
    User("Hello!").
    Build()
resp, _ := client.Responses.Create(ctx, req, callOpts...)

fmt.Println(resp.Text())
```

## Streaming Responses

```go
ctx := context.Background()
client, _ := sdk.NewClientWithKey(os.Getenv("MODELRELAY_API_KEY"))

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

## Configuration

```go
client, _ := sdk.NewClientWithKey(
    "mr_sk_...",
    sdk.WithConnectTimeout(5*time.Second),
    sdk.WithRetryConfig(sdk.RetryConfig{MaxAttempts: 3}),
)
```
