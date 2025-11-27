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
    llm "github.com/modelrelay/modelrelay/llmproxy"
    "github.com/modelrelay/modelrelay/sdk/go"
)

func main() {
    client, err := sdk.NewClient(sdk.Config{
        APIKey: os.Getenv("MODELRELAY_API_KEY"),
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
    Model:    sdk.ModelOpenAIGPT4oMini,
    Messages: []llm.ProxyMessage{{Role: "user", Content: "hi"}},
}
resp, _ := client.LLM.ProxyMessage(ctx, req)
if resp.StopReason.IsOther() {
    log.Printf("provider returned custom stop reason %q", resp.StopReason)
}
```

`Usage` now includes optional `cached_tokens` / `reasoning_tokens` fields when
providers emit them, and backfills `total_tokens` if the provider omits it.

The CLI in `examples/apikeys` uses the same calls. Provide `MODELRELAY_EMAIL` and
`MODELRELAY_PASSWORD` in the environment to log in, then run `go run ./examples/apikeys`
to create a project key.

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
