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

func (c *PluginConverter) ToWorkflow(ctx context.Context, plugin *Plugin, cmd string, task string) (*WorkflowSpecV0, error) {
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

	rf := MustOutputFormatFromType[WorkflowSpecV0]("workflow_v0")
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
		return nil, TransportError{Message: "converter returned empty output"}
	}

	var spec WorkflowSpecV0
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return nil, TransportError{Message: "converter returned invalid workflow JSON"}
	}
	if spec.Kind != WorkflowKindV0 {
		return nil, TransportError{Message: "converter returned wrong kind"}
	}
	if err := normalizeWorkflowToolExecutionModes(&spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

const pluginToWorkflowSystemPrompt = `You convert a ModelRelay plugin (markdown files) into a single workflow.v0 JSON spec.

Rules:
- Output MUST be a single JSON object and MUST validate as workflow.v0.
- Do NOT output markdown, commentary, or code fences.
- Use a DAG with parallelism when multiple agents are independent.
- Use join.all to aggregate parallel branches and then a final synthesizer node.
- Bind node outputs using bindings when passing data forward.
- Tools:
  - Client tools: fs.read_file, fs.list_files, fs.search, bash, write_file, and any other custom tools (tool_execution.mode="client")
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

func normalizeWorkflowToolExecutionModes(spec *WorkflowSpecV0) error {
	if spec == nil {
		return errors.New("workflow spec required")
	}
	for i := range spec.Nodes {
		if spec.Nodes[i].Type != WorkflowNodeTypeLLMResponses {
			continue
		}
		var input llmResponsesNodeInputV0
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
		input.ToolExecution = &ToolExecutionV0{Mode: mode}
		raw, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("node %q: failed to marshal input: %w", spec.Nodes[i].ID, err)
		}
		spec.Nodes[i].Input = raw
	}
	return nil
}

func desiredToolExecutionMode(tools []llm.Tool) (ToolExecutionModeV0, error) {
	var wanted ToolExecutionModeV0
	for _, tool := range tools {
		if tool.Type != llm.ToolTypeFunction || tool.Function == nil {
			return ToolExecutionModeClient, nil
		}
		name := strings.TrimSpace(tool.Function.Name)
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

func toolNameMode(name string) ToolExecutionModeV0 {
	switch strings.TrimSpace(name) {
	case "bash", "write_file":
		return ToolExecutionModeClient
	case "fs.read_file", "fs.search", "fs.list_files":
		return ToolExecutionModeClient
	default:
		return ToolExecutionModeClient
	}
}
