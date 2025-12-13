package sdk

import "testing"

func mustSecretKey(t *testing.T, raw string) SecretKey {
	t.Helper()
	key, err := ParseSecretKey(raw)
	if err != nil {
		t.Fatalf("parse secret key: %v", err)
	}
	return key
}
