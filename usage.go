package sdk

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

// UsageClient exposes quota summary helpers backed by the SaaS API.
type UsageClient struct {
	client *Client
}

// Summary returns the current usage window for the authenticated project/user.
func (u *UsageClient) Summary(ctx context.Context) (UsageSummary, error) {
	req, err := u.client.newJSONRequest(ctx, http.MethodGet, routes.LLMUsage, nil)
	if err != nil {
		return UsageSummary{}, err
	}
	resp, _, err := u.client.send(req, nil, nil)
	if err != nil {
		return UsageSummary{}, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	var payload struct {
		Summary UsageSummary `json:"summary"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return UsageSummary{}, err
	}
	return payload.Summary, nil
}

// DailyUsageByKey returns usage broken down by day and API key.
func (u *UsageClient) DailyUsageByKey(ctx context.Context) ([]UsagePoint, error) {
	req, err := u.client.newJSONRequest(ctx, http.MethodGet, routes.LLMUsageChart, nil)
	if err != nil {
		return nil, err
	}
	resp, _, err := u.client.send(req, nil, nil)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	var payload struct {
		Points []UsagePoint `json:"points"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Points, nil
}
