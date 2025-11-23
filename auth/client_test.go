package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientLogin(t *testing.T) {
	var captured struct {
		Path string
		Body map[string]string
		Ua   string
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Path = r.URL.Path
		captured.Ua = r.Header.Get("User-Agent")
		if err := json.NewDecoder(r.Body).Decode(&captured.Body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		resp := TokenResponse{
			AccessToken:  "access",
			RefreshToken: "refresh",
			ExpiresAt:    time.Now().Add(time.Hour).UTC(),
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	tokens, err := client.Login(context.Background(), Credentials{
		Email:    "me@example.com",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if tokens.AccessToken != "access" || tokens.RefreshToken != "refresh" {
		t.Fatalf("unexpected tokens: %+v", tokens)
	}
	if captured.Path != "/auth/login" {
		t.Fatalf("expected /auth/login, got %s", captured.Path)
	}
	if captured.Body["email"] != "me@example.com" || captured.Body["password"] != "secret" {
		t.Fatalf("unexpected payload: %+v", captured.Body)
	}
	if !strings.Contains(captured.Ua, "ModelRelaySDK") {
		t.Fatalf("expected default user agent, got %s", captured.Ua)
	}
}

func TestRefreshErrorPropagation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.Refresh(context.Background(), RefreshRequest{RefreshToken: "bad"})
	if err == nil {
		t.Fatalf("expected error")
	}
	var apiErr Error
	if !(errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized) {
		t.Fatalf("expected Error, got %v", err)
	}
}
