package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelrelay/modelrelay/billingproxy/usage"
)

// Project captures plan metadata for a workspace/project.
type Project struct {
	ID           string                 `json:"id"`
	Plan         string                 `json:"plan"`
	PlanStatus   string                 `json:"plan_status"`
	PlanDisplay  string                 `json:"plan_display,omitempty"`
	PlanType     usage.PlanMeteringType `json:"plan_type"`
	ActionsLimit *int64                 `json:"actions_limit,omitempty"`
	ActionsUsed  *int64                 `json:"actions_used,omitempty"`
	WindowStart  time.Time              `json:"window_start"`
	WindowEnd    time.Time              `json:"window_end"`
}

type projectRecord struct {
	ID           string                 `json:"id"`
	Plan         string                 `json:"plan"`
	PlanStatus   string                 `json:"plan_status"`
	PlanDisplay  string                 `json:"plan_display"`
	PlanType     usage.PlanMeteringType `json:"plan_type"`
	ActionsLimit *int64                 `json:"actions_limit"`
	ActionsUsed  *int64                 `json:"actions_used"`
	WindowStart  time.Time              `json:"window_start"`
	WindowEnd    time.Time              `json:"window_end"`
}

// ProjectsClient exposes project metadata + plan assignment helpers.
type ProjectsClient struct {
	client *Client
}

// List returns projects scoped to the authenticated user.
func (c *ProjectsClient) List(ctx context.Context) ([]Project, error) {
	req, err := c.client.newJSONRequest(ctx, http.MethodGet, "/projects", nil)
	if err != nil {
		return nil, err
	}
	resp, _, err := c.client.send(req, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var payload struct {
		Projects []projectRecord `json:"projects"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	out := make([]Project, 0, len(payload.Projects))
	for _, rec := range payload.Projects {
		out = append(out, normalizeProject(rec))
	}
	return out, nil
}

// AssignPlan updates the plan for the specified project.
func (c *ProjectsClient) AssignPlan(ctx context.Context, projectID, plan string) (Project, error) {
	projectID = strings.TrimSpace(projectID)
	plan = strings.TrimSpace(plan)
	if projectID == "" {
		return Project{}, ConfigError{Reason: "projectID is required"}
	}
	if plan == "" {
		return Project{}, ConfigError{Reason: "plan is required"}
	}
	body := map[string]any{
		"plan": plan,
	}
	req, err := c.client.newJSONRequest(ctx, http.MethodPut, fmt.Sprintf("/projects/%s/plan", projectID), body)
	if err != nil {
		return Project{}, err
	}
	resp, _, err := c.client.send(req, nil, nil)
	if err != nil {
		return Project{}, err
	}
	defer resp.Body.Close()
	var payload struct {
		Project projectRecord `json:"project"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Project{}, err
	}
	return normalizeProject(payload.Project), nil
}

func normalizeProject(rec projectRecord) Project {
	return Project{
		ID:           rec.ID,
		Plan:         rec.Plan,
		PlanStatus:   rec.PlanStatus,
		PlanDisplay:  rec.PlanDisplay,
		PlanType:     rec.PlanType,
		ActionsLimit: rec.ActionsLimit,
		ActionsUsed:  rec.ActionsUsed,
		WindowStart:  rec.WindowStart,
		WindowEnd:    rec.WindowEnd,
	}
}
