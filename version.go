package sdk

// Version is the published SDK version.
// 2.0.0: Breaking - rename EndUser identity to Customer; remove publishable keys (mr_pk_*).
// 1.39.0: Add PAYGO wallet balance/reserved + overage indicator to customer usage summary.
// 1.38.0: Flatten billing response types (return inner types directly, not wrappers).
// 1.37.0: Add BillingClient for customer self-service operations (me, subscription, usage, balance, topup, checkout).
// 1.36.0: Remove simple CRUD clients (Customers, Tiers, Models, Usage) - use raw API for these endpoints.
// 1.35.1: Add project markup_percentage and compute PAYGO topup fee split server-side.
// 1.35.0: Add PAYGO customer balance/topup endpoints and tier billing_mode fields.
// 1.32.0: Breaking - replace web tool mode with intent (search_web/fetch_url/auto).
// 1.30.0: Add streaming timing helpers (StreamEvent.Elapsed, StreamHandle.TTFT/Elapsed/StartedAt) + Items() variadic builder.
// 1.29.0: Add Pointer field to MapFanoutItemsV1 for extracting structured output from LLM response envelopes.
// 1.28.0: Add CreateV1 method to RunsClient for workflow.v1 specs (#974).
// 1.26.0: Add workflow.v1 builders/types + compile helpers.
// 1.23.0: Add session-linked runs (session_id) for server-managed sessions.
// 1.22.0: Add placeholder binding methods to fluent workflow builder (BindToPlaceholder, BindTextToPlaceholder).
// 1.21.0: Validate binding targets at workflow build time - fixes #956.
// 1.20.0: Add JoinOutput helper for ergonomic join.all output pointer construction - fixes #955.
// 1.19.0: Add ToPlaceholder binding support for ergonomic prompt injection - fixes #953.
// 1.18.0: Bindings imply edges - LLMResponsesNodeWithBindings auto-adds edges for binding sources.
// 1.9.0: Add ergonomic workflow builder pattern (NewWorkflow/AddLLMNode) with auto-edge inference (#908).
// 1.8.5: Add session message append request type.
// 1.8.4: Simplify CustomerMetadata types in OpenAPI spec (remove recursive type alias).
// 1.8.3: Surface tool_result payloads on streaming tool_use_stop events.
// 1.7.2: Breaking - Rename tier stripe_price_id to billing_price_ref, add billing_provider to tiers.
// 1.7.1: Add image usage counters to usage summaries.
// 1.7.0: Breaking - Rename subscription billing fields to billing_* and add billing_provider.
// 1.6.0: Breaking - Rename customer to customer across SDK/endpoints; update customer token providers.
// 1.5.0: Breaking - Rename customer to customer across SDK/endpoints; add customer token providers.
// 1.3.1: Align NDJSON streaming helpers with v2 structured/text stream contract updates.
// 1.3.0: Breaking - responses streaming v2 (delta/content top-level, structured patches, v2 Accept profile).
// 1.2.0: Breaking - NDJSON streaming updates now emit text deltas (not accumulated content).
// 1.1.1: Structured streaming now surfaces usage on events.
// 1.0.1: Regenerate SDK types after customer metadata constraints updates.
// 0.39.1: Fix request timeouts to not cancel streaming bodies.
// 0.39.0: Add stream collection metrics helper (CollectWithMetrics) for /responses streaming.
// 0.38.0: Breaking - Responses-first API with pure /responses builders; drop chat completions;
// rename request ID header; fail fast on invalid structured stream records;
// move structured retry loop into client layer.
// 0.37.0: Breaking - Strengthen Go SDK types for tier codes, customer external IDs,
// tier IDs in customer requests, and API key ids/kind (#499).
// 0.37.1: Docs - Add blocking chat example to README.
// 0.36.0: Add MockClient for testing, response content helpers, TTFT telemetry hook,
// and tighten NDJSON/error parsing parity with Rust SDK (#499).
// 0.33.0: Add InputPricePerMillionCents and OutputPricePerMillionCents fields to Tier struct
// for per-tier token pricing (#476).
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
// 0.26.0: Breaking - Add TextForCustomer(customerID) for customer-attributed requests where tier
// determines model. This separates customer flow (no model param) from direct flow (model required).
// 0.34.0: Breaking - Remove unused Metadata field from responses requests and builder methods.
// Metadata was accepted but never used by providers or stored.
// 0.24.0: Add package-level error helpers: IsEmailRequired, IsNoFreeTier, IsNoTiers, IsProvisioningError.
// 0.23.0: Breaking - Token mint request requires customer_id, add EMAIL_REQUIRED error code,
// Rich Hickey-style design with separate types for auto-provisioning.
// 0.35.0: CustomersClient.Claim now works with publishable keys (mr_pk_*) for user self-service.
// Enables CLI tools and frontends to link Stripe subscriptions to user identities.
// 0.41.0: Streaming robustness + explicit stream timeout options (TTFT/Idle/Total) and typed stream errors.
// 0.42.0: Breaking - Use typed API key auth values (publishable vs secret) (#505).
// 0.43.0: Add workflow run helpers (/runs) with NDJSON event streaming (workflow.v1).
// 0.45.0: Add workflow.v1 builder DSL helpers (compile to workflow.v1 DAG) (#567).
// 0.45.1: Canonicalize workflow specs and add builder helpers.
// 0.45.3: Add cost_summary to runs get response (/runs/{run_id}).
// 0.46.0: Breaking - Remove monorepo module imports; add server-authoritative workflow compilation.
// 0.47.0: Breaking - Customer bearer tokens; /responses and /runs reject publishable keys;
// add /auth/customer-token with identity mapping + auto-provision support; /customers/claim requires secret key.
// 0.48.0: Add token providers for automatic bearer auth (customer token + OIDC exchange).
// 0.49.0: Add StreamEventKind type for typed delta event kinds in workflow runs.
// 0.50.0: Breaking - Add typed APIErrorCode and shared apierrors.Code for compile-time error code checking; add OIDC exchange error codes.
// 0.51.0: Add server-side tool execution events for workflow runs.
// 0.52.0: Refactor - Extract shared streamTimeoutMonitor; add pure parseStructuredRecord() and buildCompleteFieldsMap() functions;
// use sync.Once for first-content signaling. Eliminates ~70 lines of duplicate timeout logic.
// 0.53.0: Add client-side tool handoff + resume for workflow runs (/runs/{run_id}/tool-results).
// 0.55.0: Breaking - Tool-results submission now requires step/request_id + tool name; server persists tool-loop checkpoints.
// 0.56.0: Add per-node tool_limits for workflow tool loops (max_llm_calls/max_tool_calls_per_step/wait_ttl_ms).
// 0.57.0: Add generated types from OpenAPI spec (sdk/go/generated package).
// 0.60.0: Use unsigned integers (uint32/uint64) for semantically non-negative fields (token counts, costs, seq, limits).
// 0.61.0: Breaking - Multi-model tiers with per-model pricing (#676).
// 0.62.0: Add customer self-discovery endpoint wrapper (GET /customers/me) (#680).
// 0.63.0: Use strong ModelId and TierCode types from OpenAPI spec; regenerate SDK types.
// 0.64.0: Add plugin execution helpers (PluginsClient) via workflows (#664).
// 0.65.0: Add plugin GitHub loader + core plugin types (#665).
// 0.66.0: Breaking - strengthen plugin identifier types (names, URLs, repo paths) (#665).
// 0.67.0: Add /models catalog methods and model metadata on tiers (#685).
// 0.68.0: Add PluginConverter for local plugin→workflow conversion (#666).
// 0.69.0: PluginsClient now loads/converts plugins locally via PluginLoader + PluginConverter (#668).
// 0.70.0: Add Detail field to ProviderError and NodeError for raw provider error messages.
// 0.71.0: Add customer usage endpoint (MeUsage) for spend/usage monitoring.
// 0.72.0: Plugins use client-side fs.* tools (no repo.* tools) (#695).
// 0.73.0: Add /customers/me/subscription (MeSubscription) for customer-visible subscription pricing.
// 0.74.0: Add /customers/me/usage (MeUsage) returning non-private usage metrics (requests/tokens + daily history).
// 0.75.0: Add LocalFSToolPack (fs.read_file/fs.list_files/fs.search) for tools.v0 client tools (#701).
// 0.76.0: Add customer credit balance + low-credit signal to CustomerMeUsage.
// 0.77.0: Add LocalBashToolPack (`bash`) for tools.v0 client tools (#702).
// 0.78.0: Add LocalWriteFileToolPack (`write_file`) for tools.v0 client tools (#703).
// 0.79.0: Plugin converter targets tools.v0 (fs.* tools, client execution) (#704).
// 0.79.1: Docs: tools.v0 tool pack wiring for plugins (#708).
// 0.79.2: Add MODEL_CAPABILITY_UNSUPPORTED API error code constant (#710).
// 1.0.0: Breaking - remove deprecated Config-based SDK client constructor.
// 0.80.0: Breaking - tighten tool typing (ToolName, ToolCallID, ToolExecutionResult) (#711).
// 1.8.2: PluginRunner streams run events (NDJSON) instead of polling (#672).
// 1.8.1: Add tools.v0 conformance fixtures/tests across SDKs (#709).
// 1.8.0: Add image generation support (ImagesClient) (#854).
// 1.17.0: Add transport error kinds, NDJSON test helpers, and structured stream timeout coverage.
// 1.16.1: Allow empty-body responses when decoding into nil; expand SDK test coverage.
// 1.16.0: Add typed JSON path builders (LLMOutput, LLMInput) for compile-time safe pointer construction.
// 1.15.1: Fix LLMUserMessageText binding pointer (remove /request prefix) - fixes #942.
// 1.15.0: Add Item() and ItemWithStream() builder methods to MapReduceBuilder.
// 1.14.0: Add NewMapItem constructor for ergonomic MapReduce usage.
// 1.13.1: Refactor workflow pattern builders to use value receivers (immutable) for consistency with ResponseBuilder/WorkflowBuilderV0.
// 1.13.0: Add MapReduce workflow pattern helper - fixes #932.
// 1.12.0: Add high-level workflow pattern helpers (Chain, Parallel) - fixes #913.
// 1.11.0: Add workflow package with clean type names (workflow.SpecV0, workflow.Kind) - fixes #912.
// 1.24.0: Add image pinning support (Get, Pin, Unpin) for hosted image storage (#877).
// 1.25.0: Add sessions client for multi-turn conversation management (Create, Get, List, Delete, AddMessage).
// 1.31.0: Breaking - Remove per-tier token pricing; pricing now derived from model_pricing (provider cost + 4.5% fee).
// TierModel fields renamed: InputPricePerMillionCents → ModelInputCostCents, OutputPricePerMillionCents → ModelOutputCostCents.
// 1.33.0: Make TierCode optional (*TierCode) in CustomerToken for BYOB projects without subscriptions.
// 1.34.0: Make CustomerID optional (*uuid.UUID) in CustomerToken for BYOB projects without customers.
// 2.1.0: Add GetOrCreateCustomerToken helper that upserts customer before minting token.
// 2.0.0: Breaking - remove subscription_id from customer balance response (wallet decoupled from subscription).
// 1.41.0: Add admin billing invariants route constant.
// 1.40.0: Add PAYGO wallet balance/reserved + overage indicator to customer usage summary.
// 1.39.0: Restore TiersClient (list, get, checkout) for tier querying operations.
const Version = "2.1.0"
