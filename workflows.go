package sdk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

// WorkflowsClient calls workflow compilation/validation endpoints.
type WorkflowsClient struct {
	client *Client
}

type WorkflowsCompileResponseV0 struct {
	PlanJSON json.RawMessage `json:"plan_json"`
	PlanHash PlanHash        `json:"plan_hash"`
}

type workflowsCompileOptions struct {
	timeout *time.Duration
	retry   *RetryConfig
}

type WorkflowsCompileOption func(*workflowsCompileOptions)

func buildWorkflowsCompileOptions(opts []WorkflowsCompileOption) workflowsCompileOptions {
	var out workflowsCompileOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&out)
		}
	}
	return out
}

func WithWorkflowsCompileTimeout(d time.Duration) WorkflowsCompileOption {
	return func(o *workflowsCompileOptions) { o.timeout = &d }
}

func WithWorkflowsCompileRetry(cfg RetryConfig) WorkflowsCompileOption {
	return func(o *workflowsCompileOptions) { o.retry = &cfg }
}

// CompileV0 compiles a workflow.v0 spec into a canonical plan JSON and plan_hash.
//
// On validation failures, it returns WorkflowValidationError.
func (c *WorkflowsClient) CompileV0(ctx context.Context, spec WorkflowSpecV0, opts ...WorkflowsCompileOption) (*WorkflowsCompileResponseV0, error) {
	options := buildWorkflowsCompileOptions(opts)

	req, err := c.client.newJSONRequest(ctx, http.MethodPost, routes.WorkflowsCompile, spec)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, _, err := c.client.send(req, options.timeout, options.retry)
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(resp.Body)
	//nolint:errcheck // best-effort cleanup on return
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}

	if resp.StatusCode >= 400 {
		if resp.StatusCode == http.StatusBadRequest {
			var verr WorkflowValidationError
			if err := json.Unmarshal(body, &verr); err == nil && len(verr.Issues) > 0 {
				return nil, verr
			}
		}
		return nil, decodeAPIErrorFromBytes(resp.StatusCode, body, nil)
	}

	var out WorkflowsCompileResponseV0
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
