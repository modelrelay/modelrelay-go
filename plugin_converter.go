package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

type PluginConverter struct {
	client         *Client
	converterModel ModelID
}

type PluginConverterOption func(*PluginConverter)

// WithPluginConverterModel overrides the default model used for plugin → workflow conversion.
func WithPluginConverterModel(model string) PluginConverterOption {
	return func(c *PluginConverter) {
		c.converterModel = NewModelID(model)
	}
}

func NewPluginConverter(client *Client, opts ...PluginConverterOption) *PluginConverter {
	pc := &PluginConverter{
		client:         client,
		converterModel: NewModelID("claude-3-5-haiku-latest"),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(pc)
		}
	}
	return pc
}

func (c *PluginConverter) ToWorkflow(ctx context.Context, plugin *Plugin, cmd string, task string) (*WorkflowSpecV1, error) {
	if c == nil || c.client == nil || c.client.Responses == nil {
		return nil, errors.New("plugin converter: client required")
	}
	if plugin == nil {
		return nil, errors.New("plugin converter: plugin required")
	}
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return nil, errors.New("plugin converter: command required")
	}
	task = strings.TrimSpace(task)
	if task == "" {
		return nil, errors.New("plugin converter: task required")
	}

	if c.converterModel.IsEmpty() {
		return nil, errors.New("plugin converter: converter model required")
	}

	cmdName := PluginCommandName(cmd)
	command, ok := plugin.Commands[cmdName]
	if !ok {
		return nil, errors.New("plugin converter: unknown command")
	}

	rf := MustOutputFormatFromType[WorkflowSpecV1]("workflow_v1")
	prompt, err := buildPluginConversionPrompt(*plugin, command, task)
	if err != nil {
		return nil, err
	}

	req, callOpts, err := c.client.Responses.New().
		Model(c.converterModel).
		System(pluginToWorkflowSystemPrompt).
		User(prompt).
		MaxOutputTokens(4096).
		OutputFormat(*rf).
		Build()
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Responses.Create(ctx, req, callOpts...)
	if err != nil {
		return nil, err
	}

	raw := strings.TrimSpace(resp.AssistantText())
	if raw == "" {
		return nil, TransportError{Kind: TransportErrorOther, Message: "converter returned empty output"}
	}

	var spec WorkflowSpecV1
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return nil, TransportError{Kind: TransportErrorOther, Message: "converter returned invalid workflow JSON"}
	}
	if spec.Kind != WorkflowKindV1 {
		return nil, TransportError{Kind: TransportErrorOther, Message: "converter returned wrong kind"}
	}
	if err := normalizeWorkflowToolExecutionModes(&spec); err != nil {
		return nil, err
	}
	if err := validatePluginWorkflowTargetsToolsV1(&spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

var pluginToWorkflowSystemPrompt = `You convert a ModelRelay plugin (markdown files) into a single workflow.v1 JSON spec.

Rules:
- Output MUST be a single JSON object and MUST validate as workflow.v1.
- Do NOT output markdown, commentary, or code fences.
- Use a DAG with parallelism when multiple agents are independent.
- Use join.all to aggregate parallel branches and then a final synthesizer node.
- Bind node outputs using bindings when passing data forward.
- Tool contract:
  - Target tools.v0 client tools (see docs/reference/tools.md).
  - Workspace access MUST use these exact function tool names:
    - ` + AllowedToolNamesString() + `
  - Prefer fs.* tools for reading/listing/searching the workspace (use bash only when necessary).
  - Do NOT invent ad-hoc tool names (no repo.*, github.*, filesystem.*, etc.).
  - All client tools MUST be represented as type="function" tools.
  - Any node that includes request.tools MUST set tool_execution.mode="client".
- Prefer minimal nodes needed to satisfy the task.
`

func buildPluginConversionPrompt(plugin Plugin, cmd PluginCommand, userTask string) (string, error) {
	var b strings.Builder
	b.WriteString("PLUGIN_URL: ")
	b.WriteString(plugin.URL.String())
	b.WriteString("\n")
	b.WriteString("COMMAND: ")
	b.WriteString(cmd.Name.String())
	b.WriteString("\n")
	b.WriteString("USER_TASK:\n")
	b.WriteString(userTask)
	b.WriteString("\n\n")

	b.WriteString("PLUGIN_MANIFEST:\n")
	enc, err := json.Marshal(plugin.Manifest)
	if err != nil {
		return "", err
	}
	b.Write(enc)
	b.WriteString("\n\n")

	b.WriteString("COMMAND_MARKDOWN (commands/")
	b.WriteString(cmd.Name.String())
	b.WriteString(".md):\n")
	b.WriteString(cmd.Prompt)
	b.WriteString("\n\n")

	agentNames := make([]PluginAgentName, 0, len(plugin.Agents))
	for name := range plugin.Agents {
		agentNames = append(agentNames, name)
	}
	sort.Slice(agentNames, func(i, j int) bool { return agentNames[i] < agentNames[j] })
	if len(agentNames) > 0 {
		b.WriteString("AGENTS_MARKDOWN:\n")
		for _, name := range agentNames {
			agent := plugin.Agents[name]
			b.WriteString("---- agents/")
			b.WriteString(name.String())
			b.WriteString(".md ----\n")
			b.WriteString(agent.SystemPrompt)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

func normalizeWorkflowToolExecutionModes(spec *WorkflowSpecV1) error {
	if spec == nil {
		return errors.New("workflow spec required")
	}
	for i := range spec.Nodes {
		if spec.Nodes[i].Type != WorkflowNodeTypeV1LLMResponses {
			continue
		}
		var input llmResponsesNodeInputV1
		if err := json.Unmarshal(spec.Nodes[i].Input, &input); err != nil {
			return fmt.Errorf("node %q: invalid input JSON: %w", spec.Nodes[i].ID, err)
		}
		mode, err := desiredToolExecutionMode(input.Request.Tools)
		if err != nil {
			return fmt.Errorf("node %q: %w", spec.Nodes[i].ID, err)
		}
		if mode == "" {
			continue
		}
		if input.ToolExecution != nil && input.ToolExecution.Mode == mode {
			continue
		}
		input.ToolExecution = &ToolExecutionV1{Mode: ToolExecutionModeV1(mode)}
		raw, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("node %q: failed to marshal input: %w", spec.Nodes[i].ID, err)
		}
		spec.Nodes[i].Input = raw
	}
	return nil
}

func desiredToolExecutionMode(tools []llm.Tool) (ToolExecutionModeV1, error) {
	var wanted ToolExecutionModeV1
	for _, tool := range tools {
		if tool.Type != llm.ToolTypeFunction || tool.Function == nil {
			return ToolExecutionModeClientV1, nil
		}
		name := tool.Function.Name
		if name == "" {
			continue
		}
		mode := toolNameMode(name)
		if wanted == "" {
			wanted = mode
			continue
		}
		if wanted != mode {
			return "", errors.New("mixed server and client tools in a single node")
		}
	}
	return wanted, nil
}

func toolNameMode(name ToolName) ToolExecutionModeV1 {
	switch name {
	case ToolNameBash, ToolNameWriteFile:
		return ToolExecutionModeClientV1
	case ToolNameFSReadFile, ToolNameFSSearch, ToolNameFSListFiles:
		return ToolExecutionModeClientV1
	default:
		return ToolExecutionModeClientV1
	}
}

func validatePluginWorkflowTargetsToolsV1(spec *WorkflowSpecV1) error {
	if spec == nil {
		return errors.New("workflow spec required")
	}

	allowed := AllowedToolNamesSet()

	for i := range spec.Nodes {
		if spec.Nodes[i].Type != WorkflowNodeTypeV1LLMResponses {
			continue
		}
		var input llmResponsesNodeInputV1
		if err := json.Unmarshal(spec.Nodes[i].Input, &input); err != nil {
			return fmt.Errorf("node %q: invalid input JSON: %w", spec.Nodes[i].ID, err)
		}
		if len(input.Request.Tools) == 0 {
			continue
		}
		if input.ToolExecution == nil || input.ToolExecution.Mode != ToolExecutionModeClientV1 {
			return ProtocolError{Message: fmt.Sprintf("node %q: tool_execution.mode must be %q for plugin conversion", spec.Nodes[i].ID, ToolExecutionModeClientV1)}
		}
		for _, tool := range input.Request.Tools {
			if tool.Type != llm.ToolTypeFunction || tool.Function == nil {
				return ProtocolError{Message: fmt.Sprintf("node %q: plugin conversion only supports tools.v0 function tools (got type=%q)", spec.Nodes[i].ID, tool.Type)}
			}
			name := tool.Function.Name
			if name == "" {
				return ProtocolError{Message: fmt.Sprintf("node %q: tool name required", spec.Nodes[i].ID)}
			}
			if _, ok := allowed[name]; !ok {
				return ProtocolError{Message: fmt.Sprintf("node %q: unsupported tool %q (plugin conversion targets tools.v0)", spec.Nodes[i].ID, name.String())}
			}
		}
	}
	return nil
}
