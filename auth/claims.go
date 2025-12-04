// Package auth provides authentication helpers for the ModelRelay SDK.
package auth

import cloudauth "github.com/modelrelay/modelrelay/platform/auth"

// Claims re-exports the server-side JWT claims struct so SDK consumers can rely on identical semantics.
type Claims = cloudauth.Claims
