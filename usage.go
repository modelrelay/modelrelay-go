package sdk

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/modelrelay/modelrelay/billingproxy/usage"
)

// UsageClient exposes quota summary helpers backed by the SaaS API.
type UsageClient struct {
	client *Client
}

// Summary returns the current usage window for the authenticated project/user.
func (u *UsageClient) Summary(ctx context.Context) (usage.Summary, error) {
	req, err := u.client.newJSONRequest(ctx, http.MethodGet, "/llm/usage", nil)
	if err != nil {
		return usage.Summary{}, err
	}
	resp, _, err := u.client.send(req, nil, nil)
	if err != nil {
		return usage.Summary{}, err
	}
	defer resp.Body.Close()
	var payload struct {
		Summary usage.Summary `json:"summary"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return usage.Summary{}, err
	}
	return payload.Summary, nil
}

// DailyUsageByKey returns usage broken down by day and API key.
func (u *UsageClient) DailyUsageByKey(ctx context.Context) ([]usage.UsagePoint, error) {
	req, err := u.client.newJSONRequest(ctx, http.MethodGet, "/llm/usage/chart", nil)
	if err != nil {
		return nil, err
	}
	resp, _, err := u.client.send(req, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		Points []usage.UsagePoint `json:"points"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Points, nil
}
