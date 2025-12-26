package sdk

import (
	"net/http/httptest"
	"testing"
)

func mustSecretKey(t *testing.T, raw string) SecretKey {
	t.Helper()
	key, err := ParseSecretKey(raw)
	if err != nil {
		t.Fatalf("parse secret key: %v", err)
	}
	return key
}

// newTestClient creates a Client configured to use the given httptest.Server.
// This is the standard helper for unit tests that need a mock HTTP backend.
func newTestClient(t *testing.T, srv *httptest.Server, key string) *Client {
	t.Helper()
	client, err := NewClientWithKey(
		mustSecretKey(t, key),
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return client
}
