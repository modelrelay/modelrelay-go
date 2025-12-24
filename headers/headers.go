// Package headers defines HTTP header constants used across the ModelRelay platform.
// This is the single source of truth for header names used in API requests/responses.
package headers

const (
	// RequestID is the header for request correlation / idempotency.
	// Clients can supply this header for idempotency on retries.
	RequestID = "X-ModelRelay-Request-Id"

	// APIKey is the header for API key authentication.
	APIKey = "X-ModelRelay-Api-Key" //nolint:gosec // This is a header name, not a credential

	// CustomerID is the header for customer-attributed requests.
	// When set, the customer's subscription tier (if any) determines model defaults.
	CustomerID = "X-ModelRelay-Customer-Id"
)
