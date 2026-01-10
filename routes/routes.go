// Package routes provides shared API route constants used by both
// the API server and dashboard clients to prevent path mismatches.
package routes

// API route paths - these constants are shared between server and clients
// to ensure compile-time safety and prevent endpoint mismatches.
const (
	// AuthMe returns the current authenticated user's profile.
	AuthMe = "/auth/me"

	// AuthCustomerToken mints a customer-scoped bearer token (requires secret key).
	AuthCustomerToken = "/auth/customer-token" // #nosec G101 -- route path, not a credential

	// Pricing returns public model pricing information.
	Pricing = "/pricing"

	// Providers returns the list of configured provider IDs.
	Providers = "/providers"

	// Account is the user's account page (used for Stripe redirect URLs).
	Account = "/account"

	// AdminModels is the admin endpoint for model pricing management.
	AdminModels = "/admin/models"

	// AdminBillingInvariants runs billing invariant checks (admin-only).
	AdminBillingInvariants = "/admin/billing/invariants"

	// MetricsModels returns model usage metrics.
	MetricsModels = "/metrics/models"

	// Customers lists or creates customers in a project.
	Customers = "/customers"

	// CustomersByID targets a customer by ID.
	CustomersByID = "/customers/{customer_id}"

	// CustomersSubscribe creates a checkout session for a customer subscription.
	CustomersSubscribe = "/customers/{customer_id}/subscribe"

	// CustomersSubscription reads or cancels a customer subscription.
	CustomersSubscription = "/customers/{customer_id}/subscription"

	// CustomersMe returns the current authenticated customer (customer bearer token only).
	CustomersMe = "/customers/me"

	// CustomersMeUsage returns the current customer's usage metrics (customer bearer token only).
	CustomersMeUsage = "/customers/me/usage"

	// CustomersMeSubscription returns the current customer's subscription details (customer bearer token only).
	CustomersMeSubscription = "/customers/me/subscription"

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

	// Runs starts a workflow run (workflow) and returns a run_id.
	Runs = "/runs"

	// RunsByID returns snapshot state for a run.
	RunsByID = "/runs/{run_id}"

	// RunsEvents streams the append-only event history for a run.
	RunsEvents = "/runs/{run_id}/events"

	// RunsToolResults submits tool results for an in-progress run (client tool execution mode).
	RunsToolResults = "/runs/{run_id}/tool-results"

	// RunsPendingTools returns the currently pending tool calls for an in-progress run.
	RunsPendingTools = "/runs/{run_id}/pending-tools"

	// WorkflowsCompile compiles a workflow spec (workflow) into a canonical plan and plan_hash.
	WorkflowsCompile = "/workflows/compile"

	// RunEventSchema returns the run event envelope v0 JSON Schema (draft-07).
	RunEventSchema = "/schemas/run_event.schema.json"

	// LLMUsage returns the current usage window for the authenticated user/project.
	LLMUsage = "/llm/usage"

	// LLMUsageChart returns usage broken down by day and API key.
	LLMUsageChart = "/llm/usage/chart"
)
