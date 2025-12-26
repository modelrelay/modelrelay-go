package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

func TestModelsClientList(t *testing.T) {
	model := generated.Model{
		ModelId:         "gpt-4o",
		Provider:        "openai",
		DisplayName:     "GPT-4o",
		ContextWindow:   128000,
		MaxOutputTokens: 4096,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.Models {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.RawQuery == "" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(generated.ModelsResponse{Models: []generated.Model{model}})
			return
		}
		if r.URL.Query().Get("provider") != "openai" || r.URL.Query().Get("capability") != "vision" {
			t.Fatalf("unexpected query %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generated.ModelsResponse{Models: []generated.Model{model}})
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_models")

	list, err := client.Models.List(context.Background(), nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ModelId != "gpt-4o" {
		t.Fatalf("unexpected list response: %+v", list)
	}

	list, err = client.Models.List(context.Background(), &ListModelsParams{Provider: NewProviderID("openai"), Capability: ModelCapabilityVision})
	if err != nil {
		t.Fatalf("list filtered: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected filtered list")
	}
}

func TestModelsClientEnsureInitialized(t *testing.T) {
	client := &ModelsClient{}
	_, err := client.List(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected not initialized error")
	}
}
