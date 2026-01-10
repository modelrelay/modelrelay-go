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

func newTestClient(t *testing.T, srv *httptest.Server, rawKey string) *Client {
	t.Helper()
	client, err := NewClientWithKey(
		mustSecretKey(t, rawKey),
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("new test client: %v", err)
	}
	return client
}
