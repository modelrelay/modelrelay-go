package sdk

import (
	"strings"
)

const (
	secretKeyPrefix = "mr_sk_"
)

// APIKeyAuth is an API key value used for authenticating requests.
//
// Use ParseAPIKeyAuth or ParseSecretKey to construct.
type APIKeyAuth interface {
	apiKeyAuth()
	Kind() APIKeyKind
	String() string
}

// SecretKey is an API key that can perform privileged operations (mr_sk_*).
type SecretKey string

func (k SecretKey) apiKeyAuth()      {}
func (k SecretKey) Kind() APIKeyKind { return APIKeyKindSecret }
func (k SecretKey) String() string   { return string(k) }

// ParseAPIKeyAuth parses and validates an API key string (mr_sk_*).
func ParseAPIKeyAuth(raw string) (APIKeyAuth, error) {
	trimmed := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(trimmed, secretKeyPrefix) && len(trimmed) > len(secretKeyPrefix):
		return SecretKey(trimmed), nil
	default:
		return nil, ConfigError{Reason: "invalid api key format (expected mr_sk_*)"}
	}
}

// ParseSecretKey parses and validates a secret key (mr_sk_*).
func ParseSecretKey(raw string) (SecretKey, error) {
	key, err := ParseAPIKeyAuth(raw)
	if err != nil {
		return "", err
	}
	sk, ok := key.(SecretKey)
	if !ok {
		return "", ConfigError{Reason: "secret key required (expected mr_sk_*)"}
	}
	return sk, nil
}
