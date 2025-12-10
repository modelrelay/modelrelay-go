package sdk

// Version is the published SDK version.
// 0.32.0: Add tool use event parsing (tool_use_start, tool_use_delta, tool_use_stop) to NDJSON stream.
// ToolCallDelta and ToolCalls fields now populated for tool use events.
// 0.31.0: Breaking - Unified NDJSON streaming format. Remove SSE support. All streaming now uses
// single NDJSON format with type discriminator (start/update/completion/error).
// 0.30.0: Breaking - Remove response_format.type=json_object support. Only json_schema is supported
// for structured outputs. json_object was deprecated and had inconsistent provider behavior.
// 0.29.0: Add CompleteFields to StructuredJSONEvent for progressive UI rendering.
// Enables clients to know which fields are complete during streaming.
// 0.28.1: Fix StructuredDecodeError surfacing for first-attempt decode failures,
// fix validation path prefix issue (#. prefix), add integration tests.
// 0.28.0: Add ergonomic structured output API with reflection-based schema generation.
// Structured[T]() and StreamStructured[T]() for type-safe structured outputs with validation retries.
// 0.27.0: Breaking - Replace stringly-typed scope in JWT claims with typed fields (ProjectID,
// CustomerID, CustomerExternal). Claims now has explicit fields instead of Scope []string.
// 0.26.0: Breaking - Add ChatForCustomer(customerID) for customer-attributed requests where tier
// determines model. This separates customer flow (no model param) from direct flow (model required).
// 0.24.0: Add package-level error helpers: IsEmailRequired, IsNoFreeTier, IsNoTiers, IsProvisioningError.
// 0.23.0: Breaking - FrontendTokenRequest requires customer_id, add EMAIL_REQUIRED error code,
// Rich Hickey-style design with separate types for auto-provisioning.
const Version = "0.32.0"
