package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

// DeviceFlowProvider specifies which OAuth provider to use for native device flow.
type DeviceFlowProvider string

const (
	// DeviceFlowProviderGitHub uses GitHub's native device flow (RFC 8628).
	// User authenticates directly at github.com/login/device.
	DeviceFlowProviderGitHub DeviceFlowProvider = "github"
)

// DeviceStartRequest options for starting a device authorization flow.
type DeviceStartRequest struct {
	// Provider for native device flow. Leave empty for wrapped OAuth flow.
	Provider DeviceFlowProvider
}

// DeviceTokenStatus indicates the result of polling the device token endpoint.
type DeviceTokenStatus string

const (
	// DeviceTokenStatusApproved means the user authorized and a token is available.
	DeviceTokenStatusApproved DeviceTokenStatus = "approved"
	// DeviceTokenStatusPending means the user hasn't completed authorization yet.
	DeviceTokenStatusPending DeviceTokenStatus = "pending"
	// DeviceTokenStatusError means authorization failed (expired, denied, etc.).
	DeviceTokenStatusError DeviceTokenStatus = "error"
)

// DeviceTokenResult is the result of polling the device token endpoint.
// This is a discriminated union - check Status to determine which field is populated.
type DeviceTokenResult struct {
	Status DeviceTokenStatus
	// Token is set when Status == DeviceTokenStatusApproved
	Token *generated.CustomerTokenResponse
	// Pending is set when Status == DeviceTokenStatusPending
	Pending *generated.DeviceTokenError
	// Error/ErrorDescription are set when Status == DeviceTokenStatusError
	Error            string
	ErrorDescription string
}

// DeviceStart initiates a device authorization flow (RFC 8628).
//
// Example (native GitHub flow):
//
//	resp, err := client.Auth.DeviceStart(ctx, DeviceStartRequest{Provider: DeviceFlowProviderGitHub})
//	// resp.VerificationUri will be "https://github.com/login/device"
//	fmt.Printf("Go to %s and enter code: %s\n", resp.VerificationUri, resp.UserCode)
func (a *AuthClient) DeviceStart(ctx context.Context, req DeviceStartRequest) (generated.DeviceStartResponse, error) {
	if a == nil || a.client == nil {
		return generated.DeviceStartResponse{}, fmt.Errorf("sdk: auth client not initialized")
	}

	path := routes.AuthDeviceStart
	if req.Provider != "" {
		path += "?provider=" + string(req.Provider)
	}

	httpReq, err := a.client.newJSONRequest(ctx, http.MethodPost, path, nil)
	if err != nil {
		return generated.DeviceStartResponse{}, err
	}

	resp, _, err := a.client.send(httpReq, nil, nil)
	if err != nil {
		return generated.DeviceStartResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	var payload generated.DeviceStartResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return generated.DeviceStartResponse{}, err
	}

	return payload, nil
}

// DeviceToken polls the device token endpoint for authorization completion.
//
// Returns a DeviceTokenResult with:
//   - Status=DeviceTokenStatusApproved and Token set on success
//   - Status=DeviceTokenStatusPending and Pending set when user hasn't authorized yet
//   - Status=DeviceTokenStatusError with Error/ErrorDescription on failure
//
// Example:
//
//	for {
//	    time.Sleep(time.Duration(interval) * time.Second)
//	    result, err := client.Auth.DeviceToken(ctx, deviceCode)
//	    if err != nil {
//	        return err
//	    }
//	    switch result.Status {
//	    case sdk.DeviceTokenStatusApproved:
//	        return result.Token, nil
//	    case sdk.DeviceTokenStatusPending:
//	        if result.Pending.Interval != nil {
//	            interval = *result.Pending.Interval
//	        }
//	        continue
//	    case sdk.DeviceTokenStatusError:
//	        return nil, fmt.Errorf("authorization failed: %s", result.Error)
//	    }
//	}
func (a *AuthClient) DeviceToken(ctx context.Context, deviceCode string) (DeviceTokenResult, error) {
	if a == nil || a.client == nil {
		return DeviceTokenResult{}, fmt.Errorf("sdk: auth client not initialized")
	}
	if deviceCode == "" {
		return DeviceTokenResult{}, fmt.Errorf("sdk: device_code is required")
	}

	body := generated.PollDeviceTokenJSONRequestBody{DeviceCode: deviceCode}

	httpReq, err := a.client.newJSONRequest(ctx, http.MethodPost, routes.AuthDeviceToken, body)
	if err != nil {
		return DeviceTokenResult{}, err
	}

	resp, _, err := a.client.send(httpReq, nil, nil)
	if err != nil {
		// Check if this is an API error with a 400 status (expected for pending/error states)
		var apiErr *APIError
		if asAPIError(err, &apiErr) && apiErr.Status == http.StatusBadRequest {
			return handleDeviceTokenError(apiErr), nil
		}
		return DeviceTokenResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	var payload generated.CustomerTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return DeviceTokenResult{}, err
	}

	return DeviceTokenResult{
		Status: DeviceTokenStatusApproved,
		Token:  &payload,
	}, nil
}

func handleDeviceTokenError(apiErr *APIError) DeviceTokenResult {
	errorCode := string(apiErr.Code)
	errorDesc := apiErr.Message

	// Handle OAuth device flow error codes
	if errorCode == "authorization_pending" || errorCode == "slow_down" {
		return DeviceTokenResult{
			Status: DeviceTokenStatusPending,
			Pending: &generated.DeviceTokenError{
				Error:            errorCode,
				ErrorDescription: &errorDesc,
			},
		}
	}

	return DeviceTokenResult{
		Status:           DeviceTokenStatusError,
		Error:            errorCode,
		ErrorDescription: errorDesc,
	}
}

// asAPIError checks if the error is an APIError and extracts it.
func asAPIError(err error, target **APIError) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*APIError); ok {
		*target = e
		return true
	}
	if e, ok := err.(APIError); ok {
		*target = &e
		return true
	}
	return false
}
