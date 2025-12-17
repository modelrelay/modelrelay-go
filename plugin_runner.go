package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

	var allEvents []RunEventV0
	var lastSeq int64
	handledToolCallIDs := make(map[string]struct{})

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		events, err := r.client.Runs.ListEvents(ctx, runID, WithRunEventsAfterSeq(lastSeq))
		if err != nil {
			return nil, err
		}

		var status RunStatusV0
		var waiting []RunEventNodeWaitingV0
	for _, ev := range events {
			allEvents = append(allEvents, ev)
			if seq, ok := ev.(interface{ seqNum() int64 }); ok {
				lastSeq = maxInt64(lastSeq, seq.seqNum())
			}

			switch e := ev.(type) {
			case RunEventRunCompletedV0:
				status = RunStatusSucceeded
			case RunEventRunFailedV0:
				status = RunStatusFailed
			case RunEventRunCanceledV0:
				status = RunStatusCanceled
			case RunEventNodeWaitingV0:
				waiting = append(waiting, e)
				status = RunStatusWaiting
			case RunEventNodeToolResultV0:
				if strings.TrimSpace(e.ToolResult.ToolCallID) != "" {
					handledToolCallIDs[e.ToolResult.ToolCallID] = struct{}{}
				}
			}
		}

		switch status {
		case RunStatusSucceeded:
			snap, err := r.client.Runs.Get(ctx, runID)
			if err != nil {
				return nil, err
			}
			return &PluginRunResult{
				RunID:       snap.RunID,
				Status:      snap.Status,
				Outputs:     snap.Outputs,
				CostSummary: snap.CostSummary,
				Events:      allEvents,
			}, nil
		case RunStatusFailed, RunStatusCanceled:
			return nil, &PluginRunError{
				RunID:  runID,
				Status: status,
				Events: allEvents,
			}
		case RunStatusWaiting:
			if cfg.ToolHandler == nil {
				return nil, errors.New("plugin runner: tool handler required for client tool execution (run is waiting)")
			}
			if err := r.handleWaitingEvents(ctx, runID, waiting, cfg.ToolHandler, handledToolCallIDs); err != nil {
				return nil, err
			}
			// Tools submitted; poll again immediately.
			continue
		default:
			// pending/running/no new events: just wait
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

func (r *PluginRunner) handleWaitingEvents(ctx context.Context, runID RunID, waiting []RunEventNodeWaitingV0, registry *ToolRegistry, handled map[string]struct{}) error {
	for i := range waiting {
		ev := waiting[i]
		if !ev.NodeID.Valid() {
			continue
		}
		if ev.Waiting.Step < 0 || strings.TrimSpace(ev.Waiting.RequestID) == "" {
			continue
		}
		if len(ev.Waiting.PendingToolCalls) == 0 {
			continue
		}

		var results []RunsToolResultItemV0
		for _, call := range ev.Waiting.PendingToolCalls {
			toolCallID := strings.TrimSpace(call.ToolCallID)
			if toolCallID == "" {
				continue
			}
			if _, ok := handled[toolCallID]; ok {
				continue
			}
			name := strings.TrimSpace(call.Name)
			if name == "" {
				continue
			}

			tc := llm.ToolCall{
				ID:       toolCallID,
				Type:     llm.ToolTypeFunction,
				Function: &llm.FunctionCall{Name: name, Arguments: call.Arguments},
			}
			execRes := registry.Execute(tc)
			results = append(results, RunsToolResultItemV0{
				ToolCallID: toolCallID,
				Name:       name,
				Output:     toolExecutionOutput(execRes),
			})
		}
		if len(results) == 0 {
			continue
		}

		if _, err := r.client.Runs.SubmitToolResults(ctx, runID, RunsToolResultsRequest{
			NodeID:    ev.NodeID,
			Step:      ev.Waiting.Step,
			RequestID: ev.Waiting.RequestID,
			Results:   results,
		}); err != nil {
			return err
		}
		for _, res := range results {
			if strings.TrimSpace(res.ToolCallID) != "" {
				handled[res.ToolCallID] = struct{}{}
			}
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

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
