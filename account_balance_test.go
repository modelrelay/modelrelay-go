package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAccountBalance(t *testing.T) {
	balance := AccountBalanceResponse{
		BalanceCents:             5000,
		BalanceFormatted:         "$50.00",
		Currency:                 "usd",
		LowBalanceThresholdCents: 1000,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/account/balance", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Verify API key auth is accepted
		if r.Header.Get("X-ModelRelay-Api-Key") == "" && r.Header.Get("Authorization") == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(balance)
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client, err := NewClientWithKey(SecretKey("mr_sk_test"), WithBaseURL(ts.URL))
	if err != nil {
		t.Fatalf("NewClientWithKey: %v", err)
	}

	result, err := client.AccountBalance(context.Background())
	if err != nil {
		t.Fatalf("AccountBalance: %v", err)
	}

	if result.BalanceCents != balance.BalanceCents {
		t.Errorf("expected BalanceCents %d, got %d", balance.BalanceCents, result.BalanceCents)
	}
	if result.BalanceFormatted != balance.BalanceFormatted {
		t.Errorf("expected BalanceFormatted %q, got %q", balance.BalanceFormatted, result.BalanceFormatted)
	}
	if result.Currency != balance.Currency {
		t.Errorf("expected Currency %q, got %q", balance.Currency, result.Currency)
	}
	if result.LowBalanceThresholdCents != balance.LowBalanceThresholdCents {
		t.Errorf("expected LowBalanceThresholdCents %d, got %d", balance.LowBalanceThresholdCents, result.LowBalanceThresholdCents)
	}
}

func TestAccountBalanceResponse_Decode(t *testing.T) {
	jsonData := `{
		"balance_cents": 12345,
		"balance_formatted": "$123.45",
		"currency": "usd",
		"low_balance_threshold_cents": 500
	}`

	var response AccountBalanceResponse
	if err := json.Unmarshal([]byte(jsonData), &response); err != nil {
		t.Fatalf("failed to decode AccountBalanceResponse: %v", err)
	}

	if response.BalanceCents != 12345 {
		t.Errorf("expected BalanceCents 12345, got %d", response.BalanceCents)
	}
	if response.BalanceFormatted != "$123.45" {
		t.Errorf("expected BalanceFormatted '$123.45', got %q", response.BalanceFormatted)
	}
	if response.Currency != "usd" {
		t.Errorf("expected Currency 'usd', got %q", response.Currency)
	}
	if response.LowBalanceThresholdCents != 500 {
		t.Errorf("expected LowBalanceThresholdCents 500, got %d", response.LowBalanceThresholdCents)
	}
}
