package sdk

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAPIKeysClientCreateDelete(t *testing.T) {
	keyID := uuid.New()
	created := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	lastUsed := created.Add(time.Hour)
	mux := http.NewServeMux()
	mux.HandleFunc("/api-keys", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			fmt.Fprintf(w, `{"api_key":{"id":"%s","label":"demo","kind":"secret","created_at":"%s","redacted_key":"mr_sk_abcd****","secret_key":"mr_sk_abcd_deadbeef"}}`, keyID, created.Format(time.RFC3339))
		case http.MethodGet:
			fmt.Fprintf(w, `{"api_keys":[{"id":"%s","label":"demo","kind":"secret","created_at":"%s","last_used_at":"%s","redacted_key":"mr_sk_abcd****"}]}`, keyID, created.Format(time.RFC3339), lastUsed.Format(time.RFC3339))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	})
	mux.HandleFunc("/api-keys/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client, err := NewClient(Config{
		BaseURL:     srv.URL,
		AccessToken: "token",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	key, err := client.APIKeys.Create(context.Background(), APIKeyCreateRequest{Label: "demo"})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	if key.ID != keyID || key.SecretKey == "" {
		t.Fatalf("unexpected api key payload: %#v", key)
	}

	keys, err := client.APIKeys.List(context.Background())
	if err != nil {
		t.Fatalf("list api keys: %v", err)
	}
	if len(keys) != 1 || keys[0].ID != keyID {
		t.Fatalf("unexpected list payload: %#v", keys)
	}
	if keys[0].LastUsedAt == nil || keys[0].LastUsedAt.IsZero() {
		t.Fatalf("expected last used timestamp")
	}

	if err := client.APIKeys.Delete(context.Background(), keyID); err != nil {
		t.Fatalf("delete api key: %v", err)
	}
}
