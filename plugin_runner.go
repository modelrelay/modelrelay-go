package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
}

type PluginRunResult struct {
	RunID       RunID                          `json:"run_id"`
	Status      RunStatus                      `json:"status"`
	Outputs     map[OutputName]json.RawMessage `json:"outputs,omitempty"`
	CostSummary RunCostSummary                 `json:"cost_summary"`
	Events      []RunEvent                     `json:"events,omitempty"`
}

func (r *PluginRunner) Run(ctx context.Context, spec *WorkflowSpec, cfg PluginRunConfig) (*PluginRunResult, error) {
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
	Status RunStatus
	Events []RunEvent
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

// Wait streams /runs/{run_id}/events until completion, executing client-side tools when the run enters waiting status.
func (r *PluginRunner) Wait(ctx context.Context, runID RunID, cfg PluginRunConfig) (*PluginRunResult, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("plugin runner: client required")
	}
	if !runID.Valid() {
		return nil, errors.New("plugin runner: run id required")
	}

	var allEvents []RunEvent
	var lastSeq int64
	handledToolCallIDs := make(map[ToolCallID]struct{})

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		stream, err := r.client.Runs.StreamEvents(ctx, runID, WithRunEventsAfterSeq(lastSeq))
		if err != nil {
			return nil, err
		}

		for {
			ev, ok, err := stream.Next()
			if err != nil {
				_ = stream.Close()
				return nil, err
			}
			if !ok {
				_ = stream.Close()
				break
			}

			allEvents = append(allEvents, ev)
			if seq, ok := ev.(interface{ seqNum() int64 }); ok {
				lastSeq = maxInt64(lastSeq, seq.seqNum())
			}

			switch e := ev.(type) {
			case RunEventRunCompletedV0:
				_ = stream.Close()
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
			case RunEventRunFailedV0:
				_ = stream.Close()
				return nil, &PluginRunError{
					RunID:  runID,
					Status: RunStatusFailed,
					Events: allEvents,
				}
			case RunEventRunCanceledV0:
				_ = stream.Close()
				return nil, &PluginRunError{
					RunID:  runID,
					Status: RunStatusCanceled,
					Events: allEvents,
				}
			case RunEventNodeWaitingV0:
				if cfg.ToolHandler == nil {
					_ = stream.Close()
					return nil, errors.New("plugin runner: tool handler required for client tool execution (run is waiting)")
				}
				if err := r.handleWaitingEvents(ctx, runID, []RunEventNodeWaitingV0{e}, cfg.ToolHandler, handledToolCallIDs); err != nil {
					_ = stream.Close()
					return nil, err
				}
			case RunEventNodeToolResultV0:
				if e.ToolResult.ToolCall.ID != "" {
					handledToolCallIDs[e.ToolResult.ToolCall.ID] = struct{}{}
				}
			}
		}

		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
}

func (r *PluginRunner) handleWaitingEvents(ctx context.Context, runID RunID, waiting []RunEventNodeWaitingV0, registry *ToolRegistry, handled map[ToolCallID]struct{}) error {
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
			toolCallID := call.ToolCall.ID
			if toolCallID == "" {
				continue
			}
			if _, ok := handled[toolCallID]; ok {
				continue
			}
			name := call.ToolCall.Name
			if name == "" {
				continue
			}

			tc := llm.ToolCall{
				ID:       toolCallID,
				Type:     llm.ToolTypeFunction,
				Function: &llm.FunctionCall{Name: name, Arguments: call.ToolCall.Arguments},
			}
			execRes := registry.Execute(tc)
			results = append(results, RunsToolResultItemV0{
				ToolCall: ToolCall{
					ID:   toolCallID,
					Name: ToolName(name),
				},
				Output: toolExecutionOutput(execRes),
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
			if res.ToolCall.ID != "" {
				handled[res.ToolCall.ID] = struct{}{}
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
			return fmt.Sprintf("Error: failed to marshal tool result (type=%T): %v", v, err)
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
