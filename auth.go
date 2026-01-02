// Package sdk provides the ModelRelay Go SDK for interacting with the ModelRelay API.
package sdk

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

type authStrategy interface {
	Apply(req *http.Request) error
}

type authChain []authStrategy

func (c authChain) Apply(req *http.Request) error {
	for _, s := range c {
		if s == nil {
			continue
		}
		if err := s.Apply(req); err != nil {
			return err
		}
	}
	return nil
}

type bearerAuth struct {
	token string
}

func (b bearerAuth) Apply(req *http.Request) error {
	if b.token == "" {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	return nil
}

type apiKeyAuth struct {
	key APIKeyAuth
}

func (a apiKeyAuth) Apply(req *http.Request) error {
	if a.key == nil || a.key.String() == "" {
		return nil
	}
	req.Header.Set("X-ModelRelay-Api-Key", a.key.String())
	return nil
}

// TokenProvider supplies short-lived bearer tokens for ModelRelay data-plane calls.
// Providers are responsible for caching and refreshing tokens when needed.
type TokenProvider interface {
	Token(ctx context.Context) (string, error)
}

type tokenProviderAuth struct {
	provider TokenProvider
}

func (t tokenProviderAuth) Apply(req *http.Request) error {
	if t.provider == nil {
		return nil
	}
	token, err := t.provider.Token(req.Context())
	if err != nil {
		return TokenProviderError{Cause: err}
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return TokenProviderError{Cause: errors.New("empty token")}
	}
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}
