package sdk

import "net/http"

type authProvider interface {
	Apply(req *http.Request)
}

type authChain []authProvider

func (c authChain) Apply(req *http.Request) {
	for _, provider := range c {
		if provider == nil {
			continue
		}
		provider.Apply(req)
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
