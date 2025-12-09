package sdk

// Version is the published SDK version.
// 0.27.0: Breaking - Replace stringly-typed scope in JWT claims with typed fields (ProjectID,
// CustomerID, CustomerExternal). Claims now has explicit fields instead of Scope []string.
// 0.26.0: Breaking - Add ChatForCustomer(customerID) for customer-attributed requests where tier
// determines model. This separates customer flow (no model param) from direct flow (model required).
// 0.24.0: Add package-level error helpers: IsEmailRequired, IsNoFreeTier, IsNoTiers, IsProvisioningError.
// 0.23.0: Breaking - FrontendTokenRequest requires customer_id, add EMAIL_REQUIRED error code,
// Rich Hickey-style design with separate types for auto-provisioning.
const Version = "0.27.0"
