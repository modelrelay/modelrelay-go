// Package sdk provides the ModelRelay Go SDK for interacting with the ModelRelay API.
package sdk

import (
	"time"

	"github.com/google/uuid"
)

// APIKey describes the API key payload returned by the SaaS API.
type APIKey struct {
	ID          uuid.UUID  `json:"id"`
	ProjectID   uuid.UUID  `json:"project_id"`
	Label       string     `json:"label"`
	Kind        APIKeyKind `json:"kind"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	RedactedKey string     `json:"redacted_key"`
	SecretKey   string     `json:"secret_key,omitempty"`
}
