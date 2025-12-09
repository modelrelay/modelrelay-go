package sdk

// Version is the published SDK version.
// 0.24.0: Add package-level error helpers: IsEmailRequired, IsNoFreeTier, IsNoTiers, IsProvisioningError.
// 0.23.0: Breaking - FrontendTokenRequest requires customer_id, add EMAIL_REQUIRED error code,
// Rich Hickey-style design with separate types for auto-provisioning.
const Version = "0.25.0"
