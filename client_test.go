package sdk

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBearerTokenDuplication(t *testing.T) {
	// detailed logging of what the server receives
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer my-secret-token" {
			t.Errorf("Expected 'Bearer my-secret-token', got '%s'", auth)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Case 1: User provides clean token (this should pass already)
	t.Run("CleanToken", func(t *testing.T) {
		client, err := NewClient(Config{
			BaseURL:     server.URL,
			AccessToken: "my-secret-token",
		})
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		req, _ := client.newJSONRequest(context.Background(), "GET", "/foo", nil)
		_, _, err = client.send(req, nil, nil)
		if err != nil {
			t.Errorf("Request failed: %v", err)
		}
	})

	// Case 2: User provides token with "Bearer " prefix (this is expected to fail currently)
	t.Run("TokenWithPrefix", func(t *testing.T) {
		client, err := NewClient(Config{
			BaseURL:     server.URL,
			AccessToken: "Bearer my-secret-token",
		})
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		req, _ := client.newJSONRequest(context.Background(), "GET", "/foo", nil)
		_, _, err = client.send(req, nil, nil)
		if err != nil {
			// We expect the server (in this test) to catch the double Bearer and fail the test assertion
			// But since the server just logs t.Error, the client.send might succeed with 200 OK unless the server returns 401.
			// In my test server above, I check headers and t.Error.
			// So if this case causes a double bearer, the test will fail.
		}
	})
}

func TestEnvironmentPresets(t *testing.T) {
	client, err := NewClient(Config{Environment: EnvironmentSandbox, APIKey: "test"})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if client.baseURL != sandboxBaseURL {
		t.Fatalf("expected sandbox base url %s got %s", sandboxBaseURL, client.baseURL)
	}

	client, err = NewClient(Config{Environment: EnvironmentStaging, APIKey: "test"})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if client.baseURL != stagingBaseURL {
		t.Fatalf("expected staging base url %s got %s", stagingBaseURL, client.baseURL)
	}

	client, err = NewClient(Config{BaseURL: "https://override.example.com/api/v1", Environment: EnvironmentSandbox, APIKey: "test"})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	if client.baseURL != "https://override.example.com/api/v1" {
		t.Fatalf("base url should prefer explicit override, got %s", client.baseURL)
	}
}

func TestRetryOnServerError(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	req, _ := client.newJSONRequest(context.Background(), http.MethodGet, "/retry", nil)
	resp, meta, err := client.send(req, nil, &RetryConfig{MaxAttempts: 2})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if meta == nil || meta.Attempts != 2 {
		t.Fatalf("expected 2 attempts, meta=%+v", meta)
	}
}

func TestPostDoesNotRetryByDefault(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	req, _ := client.newJSONRequest(context.Background(), http.MethodPost, "/create", nil)
	_, meta, err := client.send(req, nil, nil)
	if err == nil {
		t.Fatalf("expected failure")
	}
	if meta == nil || meta.Attempts != 1 {
		t.Fatalf("expected single attempt for POST, meta=%+v", meta)
	}
}

func TestTransportTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	timeout := 50 * time.Millisecond
	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client(), RequestTimeout: &timeout})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	req, _ := client.newJSONRequest(context.Background(), http.MethodGet, "/slow", nil)
	_, _, err = client.send(req, nil, nil)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	var terr TransportError
	if !errors.As(err, &terr) {
		t.Fatalf("expected TransportError, got %T", err)
	}
}

func TestDisableRetry(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	client, err := NewClient(Config{BaseURL: srv.URL, APIKey: "test", HTTPClient: srv.Client()})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	req, _ := client.newJSONRequest(context.Background(), http.MethodGet, "/fail", nil)
	_, meta, err := client.send(req, nil, &RetryConfig{MaxAttempts: 1})
	if err == nil {
		t.Fatalf("expected failure")
	}
	if meta == nil || meta.Attempts != 1 {
		t.Fatalf("expected 1 attempt, meta=%+v", meta)
	}
}
