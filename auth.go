// Package sdk provides the ModelRelay Go SDK for interacting with the ModelRelay API.
package sdk

import (
	"net/http"
	"strings"
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
	key string
}

func (a apiKeyAuth) Apply(req *http.Request) {
	if a.key == "" {
		return
	}
	req.Header.Set("X-ModelRelay-Api-Key", a.key)
}

// isSecretKey returns true if the API key is a secret key (mr_sk_*).
func (a apiKeyAuth) isSecretKey() bool {
	return strings.HasPrefix(a.key, "mr_sk_")
}
