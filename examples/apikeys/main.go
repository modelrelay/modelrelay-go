package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/modelrelay/modelrelay/sdk/go"
	sdkauth "github.com/modelrelay/modelrelay/sdk/go/auth"
)

var (
	label          = flag.String("label", "CLI key", "label for the API key")
	deleteID       = flag.String("delete", "", "API key ID to revoke")
	expiresMinutes = flag.Int("expires-minutes", 0, "expiration time in minutes (optional)")
	defaultBaseURL = "https://api.modelrelay.ai/api/v1"
	baseURLEnvVar  = "MODELRELAY_BASE_URL"
	emailEnvVar    = "MODELRELAY_EMAIL"
	passwordEnvVar = "MODELRELAY_PASSWORD"
)

func main() {
	flag.Parse()

	baseURL := envDefault(baseURLEnvVar, defaultBaseURL)
	email := os.Getenv(emailEnvVar)
	password := os.Getenv(passwordEnvVar)

	if email == "" || password == "" {
		log.Fatalf("%s and %s must be set", emailEnvVar, passwordEnvVar)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	authClient, err := sdkauth.NewClient(sdkauth.Config{BaseURL: baseURL})
	if err != nil {
		log.Fatalf("auth client: %v", err)
	}
	tokens, err := authClient.Login(ctx, sdkauth.Credentials{
		Email:    email,
		Password: password,
	})
	if err != nil {
		log.Fatalf("login failed: %v", err)
	}

	client, err := sdk.NewClient(sdk.Config{
		BaseURL:     baseURL,
		AccessToken: tokens.AccessToken,
	})
	if err != nil {
		log.Fatalf("sdk client: %v", err)
	}

	if *deleteID != "" {
		id, err := uuid.Parse(*deleteID)
		if err != nil {
			log.Fatalf("parse delete id: %v", err)
		}
		if err := client.APIKeys.Delete(ctx, id); err != nil {
			log.Fatalf("delete api key: %v", err)
		}
		fmt.Printf("Revoked API key %s\n", id)
		return
	}

	var expires *time.Time
	if *expiresMinutes > 0 {
		t := time.Now().Add(time.Duration(*expiresMinutes) * time.Minute)
		expires = &t
	}

	key, err := client.APIKeys.Create(ctx, sdk.APIKeyCreateRequest{
		Label:     *label,
		ExpiresAt: expires,
	})
	if err != nil {
		log.Fatalf("create api key: %v", err)
	}
	fmt.Printf("Issued API key %s (%s)\nSecret: %s\n", key.ID, key.RedactedKey, key.SecretKey)
	if key.ExpiresAt != nil {
		fmt.Printf("Expires at: %s\n", key.ExpiresAt.Format(time.RFC3339))
	}
}

func envDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
