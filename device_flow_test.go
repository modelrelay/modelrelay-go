package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelrelay/modelrelay/sdk/go/generated"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

func TestDeviceFlowStartAndToken(t *testing.T) {
	issuedAt := time.Now().UTC().Truncate(time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case routes.AuthDeviceStart:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(generated.DeviceStartResponse{
				DeviceCode:              "dev-code",
				UserCode:                "USER-CODE",
				VerificationUri:         "https://example.com/device",
				VerificationUriComplete: stringPtr("https://example.com/device?code=USER-CODE"),
				ExpiresIn:               600,
				Interval:                5,
			})
		case routes.AuthDeviceToken:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(generated.CustomerTokenResponse{
				Token:              "customer-token",
				ExpiresAt:          issuedAt.Add(10 * time.Minute),
				ExpiresIn:          600,
				ProjectId:          uuid.New(),
				CustomerId:         uuid.New(),
				CustomerExternalId: "ext_1",
				TierCode:           "pro",
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_device")

	start, err := client.Auth.DeviceStart(context.Background(), DeviceStartRequest{Provider: DeviceFlowProviderGitHub})
	if err != nil {
		t.Fatalf("device start: %v", err)
	}
	if start.DeviceCode != "dev-code" || start.VerificationUri != "https://example.com/device" {
		t.Fatalf("unexpected device start response: %+v", start)
	}

	result, err := client.Auth.DeviceToken(context.Background(), "dev-code")
	if err != nil {
		t.Fatalf("device token: %v", err)
	}
	if result.Status != DeviceTokenStatusApproved || result.Token == nil || result.Token.Token != "customer-token" {
		t.Fatalf("unexpected token result: %+v", result)
	}
}

func TestDeviceTokenPendingAndError(t *testing.T) {
	pendingResp := `{"code":"authorization_pending","message":"pending"}`
	errorResp := `{"code":"access_denied","message":"denied"}`

	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != routes.AuthDeviceToken {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		call++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if call == 1 {
			_, _ = w.Write([]byte(pendingResp))
			return
		}
		_, _ = w.Write([]byte(errorResp))
	}))
	defer srv.Close()

	client := newTestClient(t, srv, "mr_sk_device")

	pending, err := client.Auth.DeviceToken(context.Background(), "dev-code")
	if err != nil {
		t.Fatalf("pending token: %v", err)
	}
	if pending.Status != DeviceTokenStatusPending || pending.Pending == nil {
		t.Fatalf("expected pending status, got %+v", pending)
	}

	denied, err := client.Auth.DeviceToken(context.Background(), "dev-code")
	if err != nil {
		t.Fatalf("error token: %v", err)
	}
	if denied.Status != DeviceTokenStatusError || denied.Error != "access_denied" {
		t.Fatalf("expected error status, got %+v", denied)
	}
}

func TestDeviceTokenValidation(t *testing.T) {
	client := &Client{}
	_, err := client.Auth.DeviceToken(context.Background(), "")
	if err == nil {
		t.Fatalf("expected device_code required error")
	}
}

func TestHandleDeviceTokenError(t *testing.T) {
	pending := handleDeviceTokenError(&APIError{Code: "authorization_pending", Message: "pending"})
	if pending.Status != DeviceTokenStatusPending || pending.Pending == nil {
		t.Fatalf("expected pending status, got %+v", pending)
	}

	errResult := handleDeviceTokenError(&APIError{Code: "access_denied", Message: "denied"})
	if errResult.Status != DeviceTokenStatusError || errResult.Error != "access_denied" {
		t.Fatalf("expected error status, got %+v", errResult)
	}
}

func stringPtr(val string) *string { return &val }
