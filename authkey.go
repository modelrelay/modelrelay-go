package sdk

import (
	"strings"
)

const (
	publishableKeyPrefix = "mr_pk_"
	secretKeyPrefix      = "mr_sk_"
)

// APIKeyAuth is an API key value used for authenticating requests.
//
// Use ParseAPIKeyAuth, ParseSecretKey, or ParsePublishableKey to construct.
type APIKeyAuth interface {
	apiKeyAuth()
	Kind() APIKeyKind
	String() string
}

// PublishableKey is an API key intended for public/frontend use (mr_pk_*).
type PublishableKey string

func (k PublishableKey) apiKeyAuth()      {}
func (k PublishableKey) Kind() APIKeyKind { return APIKeyKindPublishable }
func (k PublishableKey) String() string   { return string(k) }

// SecretKey is an API key that can perform privileged operations (mr_sk_*).
type SecretKey string

func (k SecretKey) apiKeyAuth()      {}
func (k SecretKey) Kind() APIKeyKind { return APIKeyKindSecret }
func (k SecretKey) String() string   { return string(k) }

// ParseAPIKeyAuth parses and validates an API key string (mr_pk_* or mr_sk_*).
func ParseAPIKeyAuth(raw string) (APIKeyAuth, error) {
	trimmed := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(trimmed, publishableKeyPrefix) && len(trimmed) > len(publishableKeyPrefix):
		return PublishableKey(trimmed), nil
	case strings.HasPrefix(trimmed, secretKeyPrefix) && len(trimmed) > len(secretKeyPrefix):
		return SecretKey(trimmed), nil
	default:
		return nil, ConfigError{Reason: "invalid api key format (expected mr_pk_* or mr_sk_*)"}
	}
}

// ParsePublishableKey parses and validates a publishable key (mr_pk_*).
func ParsePublishableKey(raw string) (PublishableKey, error) {
	key, err := ParseAPIKeyAuth(raw)
	if err != nil {
		return "", err
	}
	pk, ok := key.(PublishableKey)
	if !ok {
		return "", ConfigError{Reason: "publishable key required (expected mr_pk_*)"}
	}
	return pk, nil
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
