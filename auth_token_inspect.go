package sdk

import "strings"

func isAPIKeyToken(token string) bool {
	t := strings.TrimSpace(token)
	if t == "" {
		return false
	}
	lower := strings.ToLower(t)
	return strings.HasPrefix(lower, "mr_sk_")
}

func isJWTLikeToken(token string) bool {
	t := strings.TrimSpace(token)
	if t == "" {
		return false
	}
	// JWTs have 3 base64url segments separated by '.'.
	return strings.Count(t, ".") >= 2
}

func (c *Client) hasJWTAccessToken() bool {
	if c == nil {
		return false
	}
	for _, strat := range c.auth {
		b, ok := strat.(bearerAuth)
		if !ok {
			continue
		}
		if isAPIKeyToken(b.token) {
			continue
		}
		if isJWTLikeToken(b.token) {
			return true
		}
	}
	return false
}
