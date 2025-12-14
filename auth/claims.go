// Package auth provides authentication helpers for the ModelRelay SDK.
package auth

import "github.com/golang-jwt/jwt/v5"

// Claims encodes JWT claims embedded into access tokens.
//
// This is a DTO matching the server's access token contract. The SDK keeps this
// struct local to avoid importing internal monorepo modules.
type Claims struct {
	AccountID string `json:"uid"`
	SessionID string `json:"sid"`
	TokenType string `json:"typ,omitempty"`
	KeyID     string `json:"key_id,omitempty"`

	// Typed fields instead of encoded strings in scope array.
	// These are used for frontend tokens issued via publishable keys.
	ProjectID        string `json:"pid,omitempty"`  // Project UUID
	CustomerID       string `json:"cid,omitempty"`  // Internal customer UUID
	CustomerExternal string `json:"cext,omitempty"` // External customer identifier

	jwt.RegisteredClaims
}
