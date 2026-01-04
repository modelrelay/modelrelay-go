package sdk

import "testing"

func TestParseAPIKeys(t *testing.T) {
	key, err := ParseAPIKeyAuth("mr_sk_test")
	if err != nil {
		t.Fatalf("parse api key: %v", err)
	}
	if key.Kind() != APIKeyKindSecret {
		t.Fatalf("expected secret key kind")
	}

	if _, err := ParseAPIKeyAuth(" mr_pk_test "); err == nil {
		t.Fatalf("expected invalid api key error for publishable prefix")
	}

	if _, err := ParseAPIKeyAuth("bad"); err == nil {
		t.Fatalf("expected invalid api key error")
	}
}

func TestParseSecretKey(t *testing.T) {
	if _, err := ParseSecretKey("mr_pk_test"); err == nil {
		t.Fatalf("expected secret key error")
	}
	if _, err := ParseSecretKey("mr_sk_test"); err != nil {
		t.Fatalf("expected secret key to parse: %v", err)
	}
}
