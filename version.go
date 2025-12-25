package sdk

// Version is the published SDK version.
// 1.7.2: Breaking - Rename tier stripe_price_id to billing_price_ref, add billing_provider to tiers.
// 1.7.1: Add image usage counters to usage summaries.
// 1.7.0: Breaking - Rename subscription billing fields to billing_* and add billing_provider.
// 1.6.0: Breaking - Rename end-user to customer across SDK/endpoints; update customer token providers.
// 1.5.0: Breaking - Rename customer to end-user across SDK/endpoints; add end-user token providers.
// 1.3.1: Align NDJSON streaming helpers with v2 structured/text stream contract updates.
// 1.3.0: Breaking - responses streaming v2 (delta/content top-level, structured patches, v2 Accept profile).
// 1.2.0: Breaking - NDJSON streaming updates now emit text deltas (not accumulated content).
// 1.1.1: Structured streaming now surfaces usage on events.
// 1.0.1: Regenerate SDK types after end-user metadata constraints updates.
// 0.39.1: Fix request timeouts to not cancel streaming bodies.
// 0.39.0: Add stream collection metrics helper (CollectWithMetrics) for /responses streaming.
// 0.38.0: Breaking - Responses-first API with pure /responses builders; drop chat completions;
// rename request ID header; fail fast on invalid structured stream records;
// move structured retry loop into client layer.
// 0.37.0: Breaking - Strengthen Go SDK types for tier codes, end-user external IDs,
// tier IDs in end-user requests, and API key ids/kind (#499).
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
// EndUserID, EndUserExternal). Claims now has explicit fields instead of Scope []string.
// 0.26.0: Breaking - Add TextForEndUser(endUserID) for end-user-attributed requests where tier
// determines model. This separates end-user flow (no model param) from direct flow (model required).
// 0.34.0: Breaking - Remove unused Metadata field from responses requests and builder methods.
// Metadata was accepted but never used by providers or stored.
// 0.24.0: Add package-level error helpers: IsEmailRequired, IsNoFreeTier, IsNoTiers, IsProvisioningError.
// 0.23.0: Breaking - Token mint request requires end_user_id, add EMAIL_REQUIRED error code,
// Rich Hickey-style design with separate types for auto-provisioning.
// 0.35.0: EndUsersClient.Claim now works with publishable keys (mr_pk_*) for user self-service.
// Enables CLI tools and frontends to link Stripe subscriptions to user identities.
// 0.41.0: Streaming robustness + explicit stream timeout options (TTFT/Idle/Total) and typed stream errors.
// 0.42.0: Breaking - Use typed API key auth values (publishable vs secret) (#505).
// 0.43.0: Add workflow run helpers (/runs) with NDJSON event streaming (workflow.v0).
// 0.45.0: Add workflow.v0 builder DSL helpers (compile to workflow.v0 DAG) (#567).
// 0.45.1: Canonicalize workflow specs and add builder helpers.
// 0.45.3: Add cost_summary to runs get response (/runs/{run_id}).
// 0.46.0: Breaking - Remove monorepo module imports; add server-authoritative workflow compilation.
// 0.47.0: Breaking - End-user bearer tokens; /responses and /runs reject publishable keys;
// add /auth/end-user-token with identity mapping + auto-provision support; /end-users/claim requires secret key.
// 0.48.0: Add token providers for automatic bearer auth (end-user token + OIDC exchange).
// 0.49.0: Add StreamEventKind type for typed delta event kinds in workflow runs.
// 0.50.0: Breaking - Add typed APIErrorCode and shared apierrors.Code for compile-time error code checking; add OIDC exchange error codes.
// 0.51.0: Add server-side tool execution events for workflow runs.
// 0.52.0: Refactor - Extract shared streamTimeoutMonitor; add pure parseStructuredRecord() and buildCompleteFieldsMap() functions;
// use sync.Once for first-content signaling. Eliminates ~70 lines of duplicate timeout logic.
// 0.53.0: Add client-side tool handoff + resume for workflow runs (/runs/{run_id}/tool-results).
// 0.55.0: Breaking - Tool-results submission now requires step/request_id + tool name; server persists tool-loop checkpoints.
// 0.56.0: Add per-node tool_limits for workflow tool loops (max_llm_calls/max_tool_calls_per_step/wait_ttl_ms).
// 0.57.0: Add generated types from OpenAPI spec (sdk/go/generated package).
// 0.58.0: Add device flow methods (DeviceStart, DeviceToken) for RFC 8628 device authorization.
// 0.59.0: Refactor device flow to use generated types (generated.DeviceStartResponse, etc).
// 0.60.0: Use unsigned integers (uint32/uint64) for semantically non-negative fields (token counts, costs, seq, limits).
// 0.61.0: Breaking - Multi-model tiers with per-model pricing (#676).
// 0.62.0: Add end-user self-discovery endpoint wrapper (GET /end-users/me) (#680).
// 0.63.0: Use strong ModelId and TierCode types from OpenAPI spec; regenerate SDK types.
// 0.64.0: Add plugin execution helpers (PluginsClient) via workflows (#664).
// 0.65.0: Add plugin GitHub loader + core plugin types (#665).
// 0.66.0: Breaking - strengthen plugin identifier types (names, URLs, repo paths) (#665).
// 0.67.0: Add /models catalog methods and model metadata on tiers (#685).
// 0.68.0: Add PluginConverter for local plugin→workflow conversion (#666).
// 0.69.0: PluginsClient now loads/converts plugins locally via PluginLoader + PluginConverter (#668).
// 0.70.0: Add Detail field to ProviderError and NodeErrorV0 for raw provider error messages.
// 0.71.0: Add end-user usage endpoint (MeUsage) for spend/usage monitoring.
// 0.72.0: Plugins use client-side fs.* tools (no repo.* tools) (#695).
// 0.73.0: Add /end-users/me/subscription (MeSubscription) for end-user-visible subscription pricing.
// 0.74.0: Add /end-users/me/usage (MeUsage) returning non-private usage metrics (requests/tokens + daily history).
// 0.75.0: Add LocalFSToolPack (fs.read_file/fs.list_files/fs.search) for tools.v0 client tools (#701).
// 0.76.0: Add end-user credit balance + low-credit signal to EndUserMeUsage.
// 0.77.0: Add LocalBashToolPack (`bash`) for tools.v0 client tools (#702).
// 0.78.0: Add LocalWriteFileToolPack (`write_file`) for tools.v0 client tools (#703).
// 0.79.0: Plugin converter targets tools.v0 (fs.* tools, client execution) (#704).
// 0.79.1: Docs: tools.v0 tool pack wiring for plugins (#708).
// 0.79.2: Add MODEL_CAPABILITY_UNSUPPORTED API error code constant (#710).
// 1.0.0: Breaking - remove deprecated Config-based SDK client constructor.
// 0.80.0: Breaking - tighten tool typing (ToolName, ToolCallID, ToolExecutionResult) (#711).
const Version = "1.7.2"
