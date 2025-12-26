package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

func strPtr(val string) *string { return &val }
