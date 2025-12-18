// Package routes provides shared API route constants used by both
// the API server and dashboard clients to prevent path mismatches.
package routes

// API route paths - these constants are shared between server and clients
// to ensure compile-time safety and prevent endpoint mismatches.
const (
	// AuthMe returns the current authenticated user's profile.
	AuthMe = "/auth/me"

	// AuthOIDCExchange exchanges a verified OIDC id_token for a short-lived customer-scoped bearer token.
	AuthOIDCExchange = "/auth/oidc/exchange"

	// AuthCustomerToken mints a customer-scoped bearer token (requires secret key).
	AuthCustomerToken = "/auth/customer-token" // #nosec G101 -- route path, not a credential

	// AuthDeviceStart initiates a device authorization flow (RFC 8628).
	AuthDeviceStart = "/auth/device/start"

	// AuthDeviceToken polls for device authorization completion.
	AuthDeviceToken = "/auth/device/token" // #nosec G101 -- route path, not a credential

	// Pricing returns public model pricing information.
	Pricing = "/pricing"

	// Providers returns the list of configured provider IDs.
	Providers = "/providers"

	// Account is the user's account page (used for Stripe redirect URLs).
	Account = "/account"

	// AdminModels is the admin endpoint for model pricing management.
	AdminModels = "/admin/models"

	// MetricsModels returns model usage metrics.
	MetricsModels = "/metrics/models"

	// CustomersClaim claims a customer by email, setting their external_id.
	CustomersClaim = "/customers/claim"

	// CustomersMe returns the current authenticated customer (customer bearer token only).
	CustomersMe = "/customers/me"

	// CustomersMeUsage returns the current customer's usage summary (customer bearer token only).
	CustomersMeUsage = "/customers/me/usage"

	// Models is the public models page.
	Models = "/models"

	// GenerationsLatest returns the latest clock generation for each model.
	GenerationsLatest = "/generations/latest"

	// GenerationsSSE is the SSE endpoint for real-time generation updates.
	GenerationsSSE = "/generations/sse"

	// Responses is the unified LLM responses endpoint (blocking JSON or streaming NDJSON).
	Responses = "/responses"

	// ResponsesBatch runs multiple /responses calls concurrently under a single request.
	ResponsesBatch = "/responses:batch"

	// Runs starts a workflow run (workflow.v0) and returns a run_id.
	Runs = "/runs"

	// RunsByID returns snapshot state for a run.
	RunsByID = "/runs/{run_id}"

	// RunsEvents streams the append-only event history for a run.
	RunsEvents = "/runs/{run_id}/events"

	// RunsToolResults submits tool results for an in-progress run (client tool execution mode).
	RunsToolResults = "/runs/{run_id}/tool-results"

	// RunsPendingTools returns the currently pending tool calls for an in-progress run.
	RunsPendingTools = "/runs/{run_id}/pending-tools"

	// WorkflowsCompile compiles a workflow.v0 spec into a canonical plan and plan_hash.
	WorkflowsCompile = "/workflows/compile"

	// WorkflowV0Schema returns the workflow.v0 JSON Schema (draft-07).
	WorkflowV0Schema = "/schemas/workflow_v0.schema.json"

	// RunEventV0Schema returns the run event envelope v0 JSON Schema (draft-07).
	RunEventV0Schema = "/schemas/run_event_v0.schema.json"

	// LLMUsage returns the current usage window for the authenticated user/project.
	LLMUsage = "/llm/usage"

	// LLMUsageChart returns usage broken down by day and API key.
	LLMUsageChart = "/llm/usage/chart"
)
