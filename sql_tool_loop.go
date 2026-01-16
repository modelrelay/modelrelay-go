package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

// SQLTableInfo describes a table in the database.
type SQLTableInfo struct {
	Name   string `json:"name"`
	Schema string `json:"schema,omitempty"`
}

// SQLColumnInfo describes a column in a table.
type SQLColumnInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable,omitempty"`
}

// SQLTableDescription describes a table's structure.
type SQLTableDescription struct {
	Table   string          `json:"table"`
	Columns []SQLColumnInfo `json:"columns"`
}

// SQLToolLoopHandlers provides database operations for sql tool loops.
type SQLToolLoopHandlers struct {
	ListTables    func(context.Context) ([]SQLTableInfo, error)
	DescribeTable func(context.Context, SQLDescribeTableArgs) (*SQLTableDescription, error)
	SampleRows    func(context.Context, SQLSampleRowsArgs) (*SQLExecuteResult, error)
	ExecuteSQL    func(context.Context, SQLExecuteArgs) (*SQLExecuteResult, error)
}

// SQLToolLoopOptions configures the SQL tool loop.
type SQLToolLoopOptions struct {
	Prompt                   string
	Model                    string
	System                   string
	Policy                   *SQLPolicy
	ProfileID                *uuid.UUID
	MaxAttempts              int
	RequireSchemaInspection  *bool
	SampleRows               *bool
	SampleRowsLimit           int
	ResultLimit              int
}

// SQLDescribeTableArgs identifies a table to describe.
type SQLDescribeTableArgs struct {
	Table string `json:"table"`
}

// SQLSampleRowsArgs identifies a table and sample limit.
type SQLSampleRowsArgs struct {
	Table string `json:"table"`
	Limit int    `json:"limit"`
}

// SQLExecuteArgs identifies a query to execute.
type SQLExecuteArgs struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

// SQLRow represents a single row of data as column-value pairs.
type SQLRow map[string]any

// SQLExecuteResult contains columns and rows returned by execute_sql.
type SQLExecuteResult struct {
	Columns []string `json:"columns"`
	Rows    []SQLRow `json:"rows"`
}

// SQLToolLoopResult summarizes a completed SQL tool loop.
type SQLToolLoopResult struct {
	Summary  string
	SQL      string
	Columns  []string
	Rows     []SQLRow
	Usage    AgentUsage
	Attempts int
	Notes    string
}

const (
	sqlLoopDefaultMaxAttempts     = 3
	sqlLoopDefaultSampleRowsLimit = 3
	sqlLoopMaxSampleRowsLimit     = 10
	sqlLoopDefaultResultLimit     = 100
	sqlLoopMaxResultLimit         = 1000
)

// sqlToolLoopConfig holds normalized configuration for a SQL tool loop.
type sqlToolLoopConfig struct {
	maxAttempts             int
	resultLimit             int
	sampleRowsLimit         int
	requireSchemaInspection bool
	sampleRowsEnabled       bool
	profileID               *uuid.UUID
	policy                  *SQLPolicy
}

// sqlToolLoopState tracks progress during a SQL tool loop execution.
type sqlToolLoopState struct {
	attempts        int
	listTablesCalled bool
	describedTables map[string]struct{}
	lastSQL         string
	lastColumns     []string
	lastRows        []SQLRow
	lastNotes       string
}

func newSQLToolLoopState() *sqlToolLoopState {
	return &sqlToolLoopState{
		describedTables: make(map[string]struct{}),
		lastColumns:     []string{},
		lastRows:        []SQLRow{},
	}
}

func (s *sqlToolLoopState) toResult(summary string, usage AgentUsage) *SQLToolLoopResult {
	notes := s.lastNotes
	if notes == "" && s.lastSQL == "" {
		notes = "no SQL executed"
	}
	return &SQLToolLoopResult{
		Summary:  summary,
		SQL:      s.lastSQL,
		Columns:  s.lastColumns,
		Rows:     s.lastRows,
		Usage:    usage,
		Attempts: s.attempts,
		Notes:    notes,
	}
}

// validateSQLToolLoopOptions validates required options and handlers.
func validateSQLToolLoopOptions(opts SQLToolLoopOptions, handlers SQLToolLoopHandlers) error {
	if handlers.ListTables == nil || handlers.DescribeTable == nil || handlers.ExecuteSQL == nil {
		return ConfigError{Reason: "handlers for list_tables, describe_table, and execute_sql are required"}
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		return ConfigError{Reason: "prompt is required"}
	}
	if strings.TrimSpace(opts.Model) == "" {
		return ConfigError{Reason: "model is required"}
	}
	if opts.Policy == nil && opts.ProfileID == nil {
		return ConfigError{Reason: "policy or profile_id is required"}
	}
	if opts.SampleRows != nil && *opts.SampleRows && handlers.SampleRows == nil {
		return ConfigError{Reason: "sample_rows handler is required when sample_rows is enabled"}
	}
	return nil
}

// normalizeSQLToolLoopConfig normalizes options into a config struct.
func normalizeSQLToolLoopConfig(opts SQLToolLoopOptions, handlers SQLToolLoopHandlers) sqlToolLoopConfig {
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = sqlLoopDefaultMaxAttempts
	}
	requireSchemaInspection := true
	if opts.RequireSchemaInspection != nil {
		requireSchemaInspection = *opts.RequireSchemaInspection
	}
	sampleRowsEnabled := handlers.SampleRows != nil
	if opts.SampleRows != nil {
		sampleRowsEnabled = *opts.SampleRows
	}
	return sqlToolLoopConfig{
		maxAttempts:             maxAttempts,
		resultLimit:             clampLimit(opts.ResultLimit, sqlLoopDefaultResultLimit, sqlLoopMaxResultLimit),
		sampleRowsLimit:         clampLimit(opts.SampleRowsLimit, sqlLoopDefaultSampleRowsLimit, sqlLoopMaxSampleRowsLimit),
		requireSchemaInspection: requireSchemaInspection,
		sampleRowsEnabled:       sampleRowsEnabled,
		profileID:               opts.ProfileID,
		policy:                  opts.Policy,
	}
}

// buildSQLToolRegistry creates the tool builder with all SQL tool handlers.
func buildSQLToolRegistry(
	ctx context.Context,
	cfg sqlToolLoopConfig,
	state *sqlToolLoopState,
	handlers SQLToolLoopHandlers,
	sqlClient *SQLClient,
) *ToolBuilder {
	tools := NewToolBuilder()

	tools.Add(ToolNameListTables, "List available tables in the database.",
		json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		func(args map[string]any, call llm.ToolCall) (any, error) {
			state.listTablesCalled = true
			return handlers.ListTables(ctx)
		})

	tools.Add(ToolNameDescribeTable, "Describe a table's columns and types.",
		json.RawMessage(`{"type":"object","properties":{"table":{"type":"string"}},"required":["table"],"additionalProperties":false}`),
		func(args map[string]any, call llm.ToolCall) (any, error) {
			table, ok := args["table"].(string)
			if !ok || strings.TrimSpace(table) == "" {
				return nil, newToolArgsError(call, "describe_table requires table")
			}
			state.describedTables[strings.ToLower(strings.TrimSpace(table))] = struct{}{}
			return handlers.DescribeTable(ctx, SQLDescribeTableArgs{Table: table})
		})

	if cfg.sampleRowsEnabled && handlers.SampleRows != nil {
		tools.Add(ToolNameSampleRows, "Return a small sample of rows from a table.",
			json.RawMessage(`{"type":"object","properties":{"table":{"type":"string"},"limit":{"type":"integer"}},"required":["table"],"additionalProperties":false}`),
			func(args map[string]any, call llm.ToolCall) (any, error) {
				table, ok := args["table"].(string)
				if !ok || strings.TrimSpace(table) == "" {
					return nil, newToolArgsError(call, "sample_rows requires table")
				}
				limit := cfg.sampleRowsLimit
				if v, ok := args["limit"].(float64); ok {
					limit = clampLimit(int(v), cfg.sampleRowsLimit, cfg.sampleRowsLimit)
				}
				return handlers.SampleRows(ctx, SQLSampleRowsArgs{Table: table, Limit: limit})
			})
	}

	tools.Add(ToolNameExecuteSQL, "Execute a read-only SQL query against the database.",
		json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer"}},"required":["query"],"additionalProperties":false}`),
		func(args map[string]any, call llm.ToolCall) (any, error) {
			if state.attempts >= cfg.maxAttempts {
				return nil, newToolArgsError(call, "max_attempts exceeded for execute_sql")
			}
			query, ok := args["query"].(string)
			if !ok || strings.TrimSpace(query) == "" {
				return nil, newToolArgsError(call, "execute_sql requires query")
			}
			limit := cfg.resultLimit
			if v, ok := args["limit"].(float64); ok {
				limit = clampLimit(int(v), cfg.resultLimit, cfg.resultLimit)
			}

			validateReq := SQLValidateRequest{Sql: query}
			if cfg.profileID != nil {
				id := *cfg.profileID
				validateReq.ProfileId = &id
			}
			if cfg.policy != nil {
				validateReq.Policy = cfg.policy
			}
			validation, err := sqlClient.Validate(ctx, validateReq)
			if err != nil {
				return nil, newToolArgsError(call, fmt.Sprintf("sql.validate failed: %v", err))
			}
			if strings.TrimSpace(validation.NormalizedSql) == "" {
				return nil, newToolArgsError(call, "sql.validate rejected query: missing normalized_sql")
			}
			if !validation.Valid {
				return nil, newToolArgsError(call, "sql.validate rejected query")
			}
			if !validation.ReadOnly {
				return nil, newToolArgsError(call, "sql.validate rejected query: read_only=false")
			}

			if cfg.requireSchemaInspection {
				if !state.listTablesCalled {
					return nil, newToolArgsError(call, "list_tables must be called before execute_sql")
				}
				if validation.Tables != nil {
					missing := make([]string, 0)
					for _, table := range *validation.Tables {
						if _, ok := state.describedTables[strings.ToLower(table)]; !ok {
							missing = append(missing, table)
						}
					}
					if len(missing) > 0 {
						return nil, newToolArgsError(call, "describe_table required for: "+strings.Join(missing, ", "))
					}
				}
			}

			state.attempts++
			state.lastSQL = validation.NormalizedSql
			result, err := handlers.ExecuteSQL(ctx, SQLExecuteArgs{Query: validation.NormalizedSql, Limit: limit})
			if err != nil {
				return nil, err
			}
			state.lastColumns = result.Columns
			state.lastRows = result.Rows
			if len(state.lastRows) == 0 {
				state.lastNotes = "query returned no rows"
			} else {
				state.lastNotes = ""
			}
			return result, nil
		})

	return tools
}

// SQLToolLoop runs a SQL tool loop with validation and execution.
func (c *Client) SQLToolLoop(ctx context.Context, opts SQLToolLoopOptions, handlers SQLToolLoopHandlers) (*SQLToolLoopResult, error) {
	if err := validateSQLToolLoopOptions(opts, handlers); err != nil {
		return nil, err
	}

	cfg := normalizeSQLToolLoopConfig(opts, handlers)
	state := newSQLToolLoopState()
	tools := buildSQLToolRegistry(ctx, cfg, state, handlers, c.SQL)
	definitions, registry := tools.Build()

	systemPrompt := sqlLoopSystemPrompt(cfg.maxAttempts, cfg.resultLimit, cfg.sampleRowsEnabled, cfg.requireSchemaInspection, opts.System)
	input := []llm.InputItem{llm.NewUserText(opts.Prompt)}
	if strings.TrimSpace(systemPrompt) != "" {
		input = append([]llm.InputItem{llm.NewSystemText(systemPrompt)}, input...)
	}

	usage := AgentUsage{}
	maxTurns := DefaultMaxTurns
	var lastResp *Response

	for turn := 0; turn < maxTurns; turn++ {
		builder := c.Responses.New().Model(NewModelID(opts.Model)).Input(input)
		if len(definitions) > 0 {
			builder = builder.Tools(definitions)
		}

		req, callOpts, err := builder.Build()
		if err != nil {
			return nil, err
		}
		resp, err := c.Responses.Create(ctx, req, callOpts...)
		if err != nil {
			return nil, err
		}
		lastResp = resp
		usage.LLMCalls++
		usage.InputTokens += resp.Usage.InputTokens
		usage.OutputTokens += resp.Usage.OutputTokens
		usage.TotalTokens += resp.Usage.TotalTokens

		toolCalls := resp.ToolCalls()
		if len(toolCalls) == 0 {
			break
		}
		usage.ToolCalls += len(toolCalls)
		input = append(input, AssistantMessageWithToolCalls(resp.AssistantText(), toolCalls))

		results := registry.ExecuteAll(toolCalls)
		input = append(input, registry.ResultsToMessages(results)...)
	}

	if lastResp == nil {
		return nil, ConfigError{Reason: "no response generated"}
	}

	summary := strings.TrimSpace(lastResp.AssistantText())
	return state.toResult(summary, usage), nil
}

// SQLToolLoopQuickstart runs a SQL tool loop with minimal configuration.
func (c *Client) SQLToolLoopQuickstart(
	ctx context.Context,
	model string,
	prompt string,
	handlers SQLToolLoopHandlers,
	policy *SQLPolicy,
	profileID *uuid.UUID,
) (*SQLToolLoopResult, error) {
	return c.SQLToolLoop(ctx, SQLToolLoopOptions{
		Model:     model,
		Prompt:    prompt,
		Policy:    policy,
		ProfileID: profileID,
	}, handlers)
}

func clampLimit(value int, fallback int, maxValue int) int {
	if value <= 0 {
		return fallback
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func sqlLoopSystemPrompt(maxAttempts int, resultLimit int, sampleRows bool, requireSchemaInspection bool, extra string) string {
	steps := []string{
		"Use list_tables to see available tables.",
		"Use describe_table on any table you query.",
	}
	if sampleRows {
		steps = append(steps, "Use sample_rows for quick context if needed.")
	}
	steps = append(steps, "Generate a read-only SELECT query.", "Call execute_sql to run it.")
	lines := []string{
		"You are a SQL assistant that must follow this workflow:",
		"- " + strings.Join(steps, "\n- "),
		fmt.Sprintf("- Maximum SQL attempts: %d.", maxAttempts),
		fmt.Sprintf("- Always keep result size <= %d rows.", resultLimit),
	}
	if requireSchemaInspection {
		lines = append(lines, "- Do not execute SQL until schema inspection is complete.")
	} else {
		lines = append(lines, "- Schema inspection is optional but recommended.")
	}
	lines = append(lines, "Return a concise summary of the results when done.")
	if strings.TrimSpace(extra) != "" {
		lines = append(lines, "", strings.TrimSpace(extra))
	}
	return strings.Join(lines, "\n")
}

func newToolArgsError(call llm.ToolCall, message string) *ToolArgsError {
	rawArgs := ""
	toolName := ToolName("")
	if call.Function != nil {
		rawArgs = call.Function.Arguments
		toolName = call.Function.Name
	}
	return &ToolArgsError{
		Message:      message,
		ToolCallID:   call.ID,
		ToolName:     toolName,
		RawArguments: rawArgs,
	}
}
