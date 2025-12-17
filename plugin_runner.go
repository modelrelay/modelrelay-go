package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

type PluginRunner struct {
	client *Client
}

func NewPluginRunner(client *Client) *PluginRunner {
	return &PluginRunner{client: client}
}

type PluginRunConfig struct {
	// Model is the model used for executing the generated workflow nodes.
	Model ModelID
	// ConverterModel overrides the converter model used to create the workflow spec.
	ConverterModel ModelID
	// UserTask is the user-provided task/prompt for the plugin command.
	UserTask string
	// ToolHandler executes client-side tool calls when the run enters waiting status.
	ToolHandler *ToolRegistry

	// PollInterval controls how frequently the SDK polls /runs for status updates.
	PollInterval time.Duration
}

type PluginRunResult struct {
	RunID       RunID                          `json:"run_id"`
	Status      RunStatusV0                    `json:"status"`
	Outputs     map[OutputName]json.RawMessage `json:"outputs,omitempty"`
	CostSummary RunCostSummaryV0               `json:"cost_summary"`
	Events      []RunEventV0                   `json:"events,omitempty"`

	// Conversion metadata from POST /plugins/runs (not included in /runs cost_summary).
	ConversionModel      ModelID `json:"conversion_model,omitempty"`
	ConversionResponseID string  `json:"conversion_response_id,omitempty"`
	ConversionUsage      Usage   `json:"conversion_usage,omitempty"`
}

func (r *PluginRunner) Run(ctx context.Context, spec *WorkflowSpecV0, cfg PluginRunConfig) (*PluginRunResult, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("plugin runner: client required")
	}
	if spec == nil {
		return nil, errors.New("plugin runner: workflow spec required")
	}

	created, err := r.client.Runs.Create(ctx, *spec)
	if err != nil {
		return nil, err
	}

	return r.Wait(ctx, created.RunID, cfg)
}

type PluginRunError struct {
	RunID  RunID
	Status RunStatusV0
	Events []RunEventV0
}

func (e *PluginRunError) Error() string {
	if e == nil {
		return "plugin run failed"
	}
	if e.RunID.Valid() {
		return fmt.Sprintf("plugin run %s %s", e.RunID.String(), e.Status)
	}
	return "plugin run " + string(e.Status)
}

// Wait polls /runs until completion, executing client-side tools when the run enters waiting status.
func (r *PluginRunner) Wait(ctx context.Context, runID RunID, cfg PluginRunConfig) (*PluginRunResult, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("plugin runner: client required")
	}
	if !runID.Valid() {
		return nil, errors.New("plugin runner: run id required")
	}

	poll := cfg.PollInterval
	if poll <= 0 {
		poll = 150 * time.Millisecond
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		snap, err := r.client.Runs.Get(ctx, runID)
		if err != nil {
			return nil, err
		}

		switch snap.Status {
		case RunStatusSucceeded:
			events, _ := r.client.Runs.ListEvents(ctx, runID)
			return &PluginRunResult{
				RunID:       snap.RunID,
				Status:      snap.Status,
				Outputs:     snap.Outputs,
				CostSummary: snap.CostSummary,
				Events:      events,
			}, nil
		case RunStatusFailed, RunStatusCanceled:
			events, _ := r.client.Runs.ListEvents(ctx, runID)
			return nil, &PluginRunError{
				RunID:  snap.RunID,
				Status: snap.Status,
				Events: events,
			}
		case RunStatusWaiting:
			if cfg.ToolHandler == nil {
				return nil, errors.New("plugin runner: tool handler required for client tool execution (run is waiting)")
			}
			if err := r.handlePendingTools(ctx, runID, cfg.ToolHandler); err != nil {
				return nil, err
			}
		default:
			// pending/running: just wait
		}

		timer := time.NewTimer(poll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (r *PluginRunner) handlePendingTools(ctx context.Context, runID RunID, registry *ToolRegistry) error {
	pending, err := r.client.Runs.PendingTools(ctx, runID)
	if err != nil {
		return err
	}
	for _, node := range pending.Pending {
		if node.NodeID.String() == "" || node.Step < 0 || node.RequestID == "" {
			continue
		}
		var results []RunsToolResultItemV0
		for _, call := range node.ToolCalls {
			tc := llm.ToolCall{
				ID:   call.ToolCallID,
				Type: llm.ToolTypeFunction,
				Function: &llm.FunctionCall{
					Name:      call.Name,
					Arguments: call.Arguments,
				},
			}
			execRes := registry.Execute(tc)
			results = append(results, RunsToolResultItemV0{
				ToolCallID: call.ToolCallID,
				Name:       call.Name,
				Output:     toolExecutionOutput(execRes),
			})
		}
		if len(results) == 0 {
			continue
		}
		_, err := r.client.Runs.SubmitToolResults(ctx, runID, RunsToolResultsRequest{
			NodeID:    node.NodeID,
			Step:      node.Step,
			RequestID: node.RequestID,
			Results:   results,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func toolExecutionOutput(res ToolExecutionResult) string {
	if res.Error != nil {
		return "Error: " + res.Error.Error()
	}
	if res.Result == nil {
		return ""
	}
	switch v := res.Result.(type) {
	case string:
		return v
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "Error: failed to marshal tool result"
		}
		return string(b)
	}
}
