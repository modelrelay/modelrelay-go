package auth

import cloudauth "github.com/recall-gpt/modelrelay/cloud/auth"

// Claims re-exports the server-side JWT claims struct so SDK consumers can rely on identical semantics.
type Claims = cloudauth.Claims
