package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelrelay/modelrelay/billingproxy/billing"
	"github.com/modelrelay/modelrelay/billingproxy/usage"
)

// RequestPlansClient manages the request-weighted plan catalog.
type RequestPlansClient struct {
	client *Client
}

// List returns all configured request-weighted plans.
func (c *RequestPlansClient) List(ctx context.Context) ([]usage.RequestPlan, error) {
	req, err := c.client.newJSONRequest(ctx, http.MethodGet, "/request-plans", nil)
	if err != nil {
		return nil, err
	}
	resp, _, err := c.client.send(req, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		Plans []usage.RequestPlan `json:"plans"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Plans, nil
}

// Replace atomically overwrites the request plan catalog with the provided plans.
func (c *RequestPlansClient) Replace(ctx context.Context, plans []usage.RequestPlan) ([]usage.RequestPlan, error) {
	if len(plans) == 0 {
		return nil, ConfigError{Reason: "at least one plan is required"}
	}
	for i, rp := range plans {
		id := strings.TrimSpace(rp.PlanID)
		if id == "" {
			id = strings.TrimSpace(string(rp.Plan))
		}
		if id == "" {
			return nil, ConfigError{Reason: fmt.Sprintf("plan_id missing at index %d", i)}
		}
		if rp.ActionsLimit <= 0 {
			return nil, ConfigError{Reason: fmt.Sprintf("actions_limit must be > 0 for plan %s", id)}
		}
		if rp.Plan == "" {
			plans[i].Plan = billing.PlanType(id)
		}
	}

	body := map[string]any{"plans": plans}
	req, err := c.client.newJSONRequest(ctx, http.MethodPut, "/request-plans", body)
	if err != nil {
		return nil, err
	}
	resp, _, err := c.client.send(req, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		Plans []usage.RequestPlan `json:"plans"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Plans, nil
}
