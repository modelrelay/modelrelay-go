# ModelRelay Go SDK

```bash
go get github.com/modelrelay/modelrelay/sdk/go
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

mustJSON := func(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

spec := sdk.WorkflowSpecV0{
	Kind: sdk.WorkflowKindV0,
	Nodes: []workflow.NodeV0{
		{
			ID:   "agent_a",
			Type: sdk.WorkflowNodeTypeLLMResponses,
			Input: mustJSON(map[string]any{
				"request": map[string]any{
					"model": "claude-sonnet-4-20250514",
					"input": []any{
						map[string]any{"type": "message", "role": "system", "content": []any{map[string]any{"type": "text", "text": "You are Agent A."}}},
						map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "text", "text": "Write 3 ideas for a landing page."}}},
					},
				},
			}),
		},
		{
			ID:   "agent_b",
			Type: sdk.WorkflowNodeTypeLLMResponses,
			Input: mustJSON(map[string]any{
				"request": map[string]any{
					"model": "claude-sonnet-4-20250514",
					"input": []any{
						map[string]any{"type": "message", "role": "system", "content": []any{map[string]any{"type": "text", "text": "You are Agent B."}}},
						map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "text", "text": "Write 3 objections a user might have."}}},
					},
				},
			}),
		},
		{
			ID:   "agent_c",
			Type: sdk.WorkflowNodeTypeLLMResponses,
			Input: mustJSON(map[string]any{
				"request": map[string]any{
					"model": "claude-sonnet-4-20250514",
					"input": []any{
						map[string]any{"type": "message", "role": "system", "content": []any{map[string]any{"type": "text", "text": "You are Agent C."}}},
						map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "text", "text": "Write 3 alternative headlines."}}},
					},
				},
			}),
		},
		{ID: "join", Type: sdk.WorkflowNodeTypeJoinAll},
		{
			ID:   "aggregate",
			Type: sdk.WorkflowNodeTypeTransformJSON,
			Input: mustJSON(map[string]any{
				"object": map[string]any{
					"agent_a": map[string]any{"from": "join", "pointer": "/agent_a"},
					"agent_b": map[string]any{"from": "join", "pointer": "/agent_b"},
					"agent_c": map[string]any{"from": "join", "pointer": "/agent_c"},
				},
			}),
		},
	},
	Edges: []workflow.EdgeV0{
		{From: "agent_a", To: "join"},
		{From: "agent_b", To: "join"},
		{From: "agent_c", To: "join"},
		{From: "join", To: "aggregate"},
	},
	Outputs: []workflow.OutputRefV0{
		{Name: "result", From: "aggregate"},
	},
}

created, _ := client.Runs.Create(ctx, spec)

stream, _ := client.Runs.StreamEvents(ctx, created.RunID)
defer stream.Close()
	for {
		ev, ok, _ := stream.Next()
		if !ok {
			break
		}
		switch e := ev.(type) {
		case sdk.RunEventRunCompletedV0:
			_ = e // event includes outputs_artifact_key; fetch the full outputs via /runs/{run_id}
			status, _ := client.Runs.Get(ctx, created.RunID)
			b, _ := json.Marshal(status.Outputs)
			fmt.Printf("outputs: %s\n", string(b))
		}
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
secret, _ := sdk.ParseSecretKey("mr_sk_...")
client, _ := sdk.NewClientWithKey(secret,
    sdk.WithConnectTimeout(5*time.Second),
    sdk.WithRetryConfig(sdk.RetryConfig{MaxAttempts: 3}),
)
```
