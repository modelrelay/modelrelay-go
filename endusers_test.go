package sdk

import (
	"context"
	"testing"
)

func TestEndUserCheckoutValidation(t *testing.T) {
	client, err := NewClient(Config{BaseURL: "https://api.example.com", APIKey: "test"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.EndUsers.Checkout(context.Background(), EndUserCheckoutRequest{})
	if err == nil {
		t.Fatalf("expected validation error for missing end user id")
	}
	_, err = client.EndUsers.Checkout(context.Background(), EndUserCheckoutRequest{
		EndUserID: "abc",
		Success:   "https://ok",
	})
	if err == nil {
		t.Fatalf("expected validation error for missing cancel_url")
	}
}
