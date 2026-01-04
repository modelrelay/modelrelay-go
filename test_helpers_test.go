package sdk

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/modelrelay/modelrelay/platform/workflow"
)

func canonicalJSON(t *testing.T, raw []byte) []byte {
	t.Helper()
	var anyVal any
	if err := json.Unmarshal(raw, &anyVal); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	canon, err := workflow.MarshalCanonicalJSON(anyVal)
	if err != nil {
		t.Fatalf("canonicalize json: %v", err)
	}
	return canon
}

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
