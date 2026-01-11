package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
	"github.com/modelrelay/modelrelay/sdk/go/workflowintent"
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

func (c *PluginConverter) ToWorkflow(ctx context.Context, plugin *Plugin, cmd string, task string) (*WorkflowSpec, error) {
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

	rf := MustOutputFormatFromType[WorkflowSpec]("workflow")
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

	var spec WorkflowSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return nil, TransportError{Kind: TransportErrorOther, Message: "converter returned invalid workflow JSON"}
	}
	if spec.Kind != WorkflowKindIntent {
		return nil, TransportError{Kind: TransportErrorOther, Message: "converter returned wrong kind"}
	}
	if strings.TrimSpace(c.converterModel.String()) != "" {
		spec.Model = c.converterModel.String()
	}
	if err := normalizeWorkflowIntentToolExecutionModes(&spec); err != nil {
		return nil, err
	}
	if err := validatePluginWorkflowTargetsToolsIntent(&spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

var pluginToWorkflowSystemPrompt = `You convert a ModelRelay plugin (markdown files) into a single workflow JSON spec.

Rules:
- Output MUST be a single JSON object and MUST validate as workflow.
- Do NOT output markdown, commentary, or code fences.
- Use a DAG with parallelism when multiple agents are independent.
- Use join.all to aggregate parallel branches and then a final synthesizer node.
- Use depends_on for edges between nodes.
- Bind node outputs using {{placeholders}} when passing data forward.
- Tool contract:
  - Target tools.v0 client tools (see docs/reference/tools.md).
  - Workspace access MUST use these exact function tool names:
    - ` + AllowedToolNamesString() + `
  - Prefer fs.* tools for reading/listing/searching the workspace (use bash only when necessary).
  - Do NOT invent ad-hoc tool names (no repo.*, github.*, filesystem.*, etc.).
  - All client tools MUST be represented as type="function" tools.
  - Any node that includes tools MUST set tool_execution.mode="client".
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

func normalizeWorkflowIntentToolExecutionModes(spec *WorkflowSpec) error {
	if spec == nil {
		return errors.New("workflow spec required")
	}
	for i := range spec.Nodes {
		if spec.Nodes[i].Type != WorkflowNodeTypeLLM {
			continue
		}
		mode, err := desiredToolExecutionMode(toolRefsToTools(spec.Nodes[i].Tools))
		if err != nil {
			return fmt.Errorf("node %q: %w", spec.Nodes[i].ID, err)
		}
		if mode == "" {
			continue
		}
		if spec.Nodes[i].ToolExecution != nil && spec.Nodes[i].ToolExecution.Mode == mode {
			continue
		}
		spec.Nodes[i].ToolExecution = &workflowintent.ToolExecution{Mode: mode}
	}
	return nil
}

func desiredToolExecutionMode(tools []llm.Tool) (workflowintent.ToolExecutionMode, error) {
	var wanted workflowintent.ToolExecutionMode
	for i, tool := range tools {
		if tool.Type != llm.ToolTypeFunction || tool.Function == nil {
			return workflowintent.ToolExecutionModeClient, nil
		}
		name := tool.Function.Name
		if name == "" {
			return "", fmt.Errorf("tool at index %d has empty name", i)
		}
		mode, err := toolNameMode(name)
		if err != nil {
			return "", err
		}
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

// knownClientTools is the set of tools that support client-side execution.
var knownClientTools = map[ToolName]struct{}{
	ToolNameBash:        {},
	ToolNameWriteFile:   {},
	ToolNameFSReadFile:  {},
	ToolNameFSSearch:    {},
	ToolNameFSListFiles: {},
}

func toolNameMode(name ToolName) (workflowintent.ToolExecutionMode, error) {
	if _, ok := knownClientTools[name]; ok {
		return workflowintent.ToolExecutionModeClient, nil
	}
	return "", fmt.Errorf("unknown tool %q for mode detection; expected one of: bash, write_file, fs.read_file, fs.search, fs.list_files", name)
}

func validatePluginWorkflowTargetsToolsIntent(spec *WorkflowSpec) error {
	if spec == nil {
		return errors.New("workflow spec required")
	}

	allowed := AllowedToolNamesSet()

	for i := range spec.Nodes {
		if spec.Nodes[i].Type != WorkflowNodeTypeLLM {
			continue
		}
		if len(spec.Nodes[i].Tools) == 0 {
			continue
		}
		if spec.Nodes[i].ToolExecution == nil || spec.Nodes[i].ToolExecution.Mode != "client" {
			return ProtocolError{Message: fmt.Sprintf("node %q: tool_execution.mode must be %q for plugin conversion", spec.Nodes[i].ID, "client")}
		}
		for _, tool := range toolRefsToTools(spec.Nodes[i].Tools) {
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

func toolRefsToTools(refs []workflowintent.ToolRef) []llm.Tool {
	if len(refs) == 0 {
		return nil
	}
	out := make([]llm.Tool, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref.Tool)
	}
	return out
}
