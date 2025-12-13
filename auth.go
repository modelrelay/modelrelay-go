// Package sdk provides the ModelRelay Go SDK for interacting with the ModelRelay API.
package sdk

import (
	"net/http"
)

type authStrategy interface {
	Apply(req *http.Request)
}

type authChain []authStrategy

func (c authChain) Apply(req *http.Request) {
	for _, s := range c {
		if s == nil {
			continue
		}
		s.Apply(req)
	}
}

type bearerAuth struct {
	token string
}

func (b bearerAuth) Apply(req *http.Request) {
	if b.token == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
}

type apiKeyAuth struct {
	key APIKeyAuth
}

func (a apiKeyAuth) Apply(req *http.Request) {
	if a.key == nil || a.key.String() == "" {
		return
	}
	req.Header.Set("X-ModelRelay-Api-Key", a.key.String())
}

// isSecretKey returns true if the API key is a secret key.
func (a apiKeyAuth) isSecretKey() bool {
	if a.key == nil {
		return false
	}
	return a.key.Kind() == APIKeyKindSecret
}
