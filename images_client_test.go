package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
)

func TestImagesClientGenerate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/generate" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var req generated.ImageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Prompt == "" {
			t.Fatalf("expected prompt")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.ImageResponse{
			Id:    "img_1",
			Model: "gpt-4o-image",
			Usage: generated.ImageUsage{Images: 1},
			Data:  []generated.ImageData{{Url: strPtr("https://example.com/image.png"), MimeType: strPtr("image/png")}},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_images")

	model := "gpt-4o-image"
	resp, err := client.Images.Generate(context.Background(), generated.ImageRequest{
		Prompt: "a test image",
		Model:  &model,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if resp.Id != "img_1" || len(resp.Data) != 1 {
		t.Fatalf("unexpected response %+v", resp)
	}
}

func TestImagesClientValidation(t *testing.T) {
	client := &Client{}
	_, err := client.Images.Generate(context.Background(), generated.ImageRequest{})
	if err == nil {
		t.Fatalf("expected prompt validation error")
	}
}

func TestImagesClientGet(t *testing.T) {
	expiresAt := time.Date(2025, 1, 22, 10, 30, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/img_abc123" || r.Method != http.MethodGet {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.ImagePinResponse{
			Id:        "img_abc123",
			Pinned:    false,
			ExpiresAt: &expiresAt,
			Url:       "https://storage.example.com/images/abc123.png",
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_images")

	resp, err := client.Images.Get(context.Background(), "img_abc123")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if resp.Id != "img_abc123" || resp.Pinned {
		t.Fatalf("unexpected response %+v", resp)
	}
	if resp.ExpiresAt == nil || !resp.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expires_at=%v, got %v", expiresAt, resp.ExpiresAt)
	}
}

func TestImagesClientPin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/img_abc123/pin" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.ImagePinResponse{
			Id:        "img_abc123",
			Pinned:    true,
			ExpiresAt: nil, // pinned images don't expire
			Url:       "https://storage.example.com/images/abc123.png",
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_images")

	resp, err := client.Images.Pin(context.Background(), "img_abc123")
	if err != nil {
		t.Fatalf("pin: %v", err)
	}
	if resp.Id != "img_abc123" || !resp.Pinned {
		t.Fatalf("unexpected response %+v", resp)
	}
	if resp.ExpiresAt != nil {
		t.Fatalf("expected no expires_at for pinned image")
	}
}

func TestImagesClientUnpin(t *testing.T) {
	expiresAt := time.Date(2025, 1, 29, 10, 30, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/img_abc123/pin" || r.Method != http.MethodDelete {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.ImagePinResponse{
			Id:        "img_abc123",
			Pinned:    false,
			ExpiresAt: &expiresAt, // will expire in 7 days
			Url:       "https://storage.example.com/images/abc123.png",
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_images")

	resp, err := client.Images.Unpin(context.Background(), "img_abc123")
	if err != nil {
		t.Fatalf("unpin: %v", err)
	}
	if resp.Id != "img_abc123" || resp.Pinned {
		t.Fatalf("unexpected response %+v", resp)
	}
	if resp.ExpiresAt == nil || !resp.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expires_at=%v after unpin, got %v", expiresAt, resp.ExpiresAt)
	}
}

func TestImagesClientPinValidation(t *testing.T) {
	client := &Client{Images: &ImagesClient{}}

	// Get with empty ID
	_, err := client.Images.Get(context.Background(), "")
	if err == nil {
		t.Fatalf("expected image_id validation error for Get")
	}

	// Pin with empty ID
	_, err = client.Images.Pin(context.Background(), "  ")
	if err == nil {
		t.Fatalf("expected image_id validation error for Pin")
	}

	// Unpin with empty ID
	_, err = client.Images.Unpin(context.Background(), "")
	if err == nil {
		t.Fatalf("expected image_id validation error for Unpin")
	}
}

func strPtr(val string) *string { return &val }
