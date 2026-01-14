package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
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

const orchestrationPlanKindV1 = "orchestration.plan.v1"

type OrchestrationPlanV1 struct {
	Kind           string                    `json:"kind" enum:"orchestration.plan.v1"`
	MaxParallelism *int64                    `json:"max_parallelism,omitempty" minimum:"1"`
	Steps          []OrchestrationPlanStepV1 `json:"steps"`
}

type OrchestrationPlanStepV1 struct {
	ID        string                     `json:"id,omitempty" minLength:"1"`
	DependsOn []string                   `json:"depends_on,omitempty"`
	Agents    []OrchestrationPlanAgentV1 `json:"agents"`
}

type OrchestrationPlanAgentV1 struct {
	ID     string `json:"id" minLength:"1"`
	Reason string `json:"reason" minLength:"1"`
}

// OrchestrationErrorCode represents a typed orchestration error code.
type OrchestrationErrorCode string

const (
	OrchestrationErrInvalidPlan       OrchestrationErrorCode = "INVALID_PLAN"
	OrchestrationErrUnknownAgent      OrchestrationErrorCode = "UNKNOWN_AGENT"
	OrchestrationErrMissingDesc       OrchestrationErrorCode = "MISSING_DESCRIPTION"
	OrchestrationErrUnknownTool       OrchestrationErrorCode = "UNKNOWN_TOOL"
	OrchestrationErrInvalidDependency OrchestrationErrorCode = "INVALID_DEPENDENCY"
	OrchestrationErrInvalidToolConfig OrchestrationErrorCode = "INVALID_TOOL_CONFIG"
)

// PluginOrchestrationError represents a structured orchestration error.
type PluginOrchestrationError struct {
	Code    OrchestrationErrorCode
	Message string
}

func (e *PluginOrchestrationError) Error() string {
	if e == nil {
		return "plugin orchestration error"
	}
	return "plugin orchestration: " + e.Message
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

// ToWorkflowDynamic selects agents based on descriptions and builds a workflow spec.
func (c *PluginConverter) ToWorkflowDynamic(ctx context.Context, plugin *Plugin, cmd string, task string) (*WorkflowSpec, error) {
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

	candidates, candidateLookup, err := buildOrchestrationCandidates(plugin, command)
	if err != nil {
		return nil, err
	}

	plan, err := c.planOrchestration(ctx, *plugin, command, task, candidates)
	if err != nil {
		return nil, err
	}
	if validateErr := validateOrchestrationPlanV1(plan, candidateLookup); validateErr != nil {
		return nil, validateErr
	}

	spec, err := buildDynamicWorkflowFromPlan(*plugin, command, task, plan, candidateLookup, c.converterModel)
	if err != nil {
		return nil, err
	}
	if err := validatePluginWorkflowTargetsToolsIntent(spec); err != nil {
		return nil, err
	}
	return spec, nil
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
  - Prefer fs_* tools for reading/listing/searching the workspace (use bash only when necessary).
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

var pluginOrchestrationSystemPrompt = `You plan which plugin agents to run based only on their descriptions.

Rules:
- Output MUST be a single JSON object that matches orchestration.plan.v1.
- Do NOT output markdown, commentary, or code fences.
- Select only from the provided agent IDs.
- Prefer minimal agents needed to satisfy the user task.
- Use multiple steps only when later agents must build on earlier results.
- Each step can run agents in parallel.
- Use "id" + "depends_on" if you need non-sequential step ordering.
`

type orchestrationCandidate struct {
	Name        PluginAgentName
	Description string
	Agent       PluginAgent
}

func buildOrchestrationCandidates(plugin *Plugin, command PluginCommand) ([]orchestrationCandidate, map[PluginAgentName]PluginAgent, error) {
	if plugin == nil {
		return nil, nil, errors.New("plugin converter: plugin required")
	}
	var names []PluginAgentName
	if len(command.AgentRefs) > 0 {
		names = append(names, command.AgentRefs...)
	} else {
		names = make([]PluginAgentName, 0, len(plugin.Agents))
		for name := range plugin.Agents {
			names = append(names, name)
		}
		sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	}
	if len(names) == 0 {
		return nil, nil, &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: "no agents available for dynamic orchestration"}
	}
	candidates := make([]orchestrationCandidate, 0, len(names))
	lookup := make(map[PluginAgentName]PluginAgent, len(names))
	for _, name := range names {
		agent, ok := plugin.Agents[name]
		if !ok {
			return nil, nil, &PluginOrchestrationError{Code: OrchestrationErrUnknownAgent, Message: fmt.Sprintf("agent %q not found", name)}
		}
		desc := strings.TrimSpace(agent.Description)
		if desc == "" {
			return nil, nil, &PluginOrchestrationError{Code: OrchestrationErrMissingDesc, Message: fmt.Sprintf("agent %q missing description", name)}
		}
		candidates = append(candidates, orchestrationCandidate{
			Name:        name,
			Description: desc,
			Agent:       agent,
		})
		lookup[name] = agent
	}
	return candidates, lookup, nil
}

func (c *PluginConverter) planOrchestration(ctx context.Context, plugin Plugin, command PluginCommand, task string, candidates []orchestrationCandidate) (OrchestrationPlanV1, error) {
	var out OrchestrationPlanV1
	if c == nil || c.client == nil || c.client.Responses == nil {
		return out, errors.New("plugin converter: client required")
	}
	if c.converterModel.IsEmpty() {
		return out, errors.New("plugin converter: converter model required")
	}
	task = strings.TrimSpace(task)
	if task == "" {
		return out, errors.New("plugin converter: task required")
	}

	rf := MustOutputFormatFromType[OrchestrationPlanV1]("orchestration_plan")
	prompt := buildPluginOrchestrationPrompt(plugin, command, task, candidates)

	req, callOpts, err := c.client.Responses.New().
		Model(c.converterModel).
		System(pluginOrchestrationSystemPrompt).
		User(prompt).
		MaxOutputTokens(1024).
		OutputFormat(*rf).
		Build()
	if err != nil {
		return out, err
	}
	resp, err := c.client.Responses.Create(ctx, req, callOpts...)
	if err != nil {
		return out, err
	}

	raw := strings.TrimSpace(resp.AssistantText())
	if raw == "" {
		return out, TransportError{Kind: TransportErrorOther, Message: "orchestrator returned empty output"}
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return out, TransportError{Kind: TransportErrorOther, Message: "orchestrator returned invalid plan JSON"}
	}
	return out, nil
}

func buildPluginOrchestrationPrompt(plugin Plugin, cmd PluginCommand, userTask string, candidates []orchestrationCandidate) string {
	var b strings.Builder
	if strings.TrimSpace(plugin.Manifest.Name) != "" {
		b.WriteString("PLUGIN_NAME: ")
		b.WriteString(strings.TrimSpace(plugin.Manifest.Name))
		b.WriteString("\n")
	}
	if strings.TrimSpace(plugin.Manifest.Description) != "" {
		b.WriteString("PLUGIN_DESCRIPTION: ")
		b.WriteString(strings.TrimSpace(plugin.Manifest.Description))
		b.WriteString("\n")
	}
	b.WriteString("COMMAND: ")
	b.WriteString(cmd.Name.String())
	b.WriteString("\n")
	b.WriteString("USER_TASK:\n")
	b.WriteString(strings.TrimSpace(userTask))
	b.WriteString("\n\n")

	if strings.TrimSpace(cmd.Prompt) != "" {
		b.WriteString("COMMAND_MARKDOWN:\n")
		b.WriteString(cmd.Prompt)
		b.WriteString("\n\n")
	}

	b.WriteString("CANDIDATE_AGENTS:\n")
	for _, c := range candidates {
		b.WriteString("- id: ")
		b.WriteString(c.Name.String())
		b.WriteString("\n  description: ")
		b.WriteString(c.Description)
		b.WriteString("\n")
	}
	return b.String()
}

func validateOrchestrationPlanV1(plan OrchestrationPlanV1, candidates map[PluginAgentName]PluginAgent) error {
	if plan.Kind != orchestrationPlanKindV1 {
		return &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: fmt.Sprintf("orchestration plan kind must be %q", orchestrationPlanKindV1)}
	}
	if len(plan.Steps) == 0 {
		return &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: "orchestration plan must include at least one step"}
	}
	if plan.MaxParallelism != nil && *plan.MaxParallelism < 1 {
		return &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: "max_parallelism must be >= 1"}
	}
	hasExplicitDeps := false
	stepIDs := make(map[string]int, len(plan.Steps))
	for i := range plan.Steps {
		step := plan.Steps[i]
		if len(step.DependsOn) > 0 {
			hasExplicitDeps = true
		}
		if strings.TrimSpace(step.ID) != "" {
			id := strings.TrimSpace(step.ID)
			if _, ok := stepIDs[id]; ok {
				return &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: fmt.Sprintf("duplicate step id %q", id)}
			}
			stepIDs[id] = i
		}
	}
	if hasExplicitDeps {
		for i := range plan.Steps {
			if strings.TrimSpace(plan.Steps[i].ID) == "" {
				return &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: "step id required when depends_on is used"}
			}
		}
	}
	seen := map[PluginAgentName]struct{}{}
	for i := range plan.Steps {
		step := plan.Steps[i]
		if len(step.Agents) == 0 {
			return &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: fmt.Sprintf("orchestration plan step %d must include at least one agent", i+1)}
		}
		if len(step.DependsOn) > 0 {
			for _, dep := range step.DependsOn {
				depID := strings.TrimSpace(dep)
				if depID == "" {
					return &PluginOrchestrationError{Code: OrchestrationErrInvalidDependency, Message: fmt.Sprintf("orchestration plan step %d has empty depends_on id", i+1)}
				}
				if depIndex, ok := stepIDs[depID]; !ok {
					return &PluginOrchestrationError{Code: OrchestrationErrInvalidDependency, Message: fmt.Sprintf("orchestration plan step %d depends on unknown step %q", i+1, depID)}
				} else if depIndex >= i {
					return &PluginOrchestrationError{Code: OrchestrationErrInvalidDependency, Message: fmt.Sprintf("orchestration plan step %d depends on future step %q", i+1, depID)}
				}
			}
		}
		for j := range step.Agents {
			agent := step.Agents[j]
			rawID := strings.TrimSpace(agent.ID)
			if rawID == "" {
				return &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: fmt.Sprintf("orchestration plan step %d agent %d id required", i+1, j+1)}
			}
			id := PluginAgentName(rawID)
			if _, ok := candidates[id]; !ok {
				return &PluginOrchestrationError{Code: OrchestrationErrUnknownAgent, Message: fmt.Sprintf("orchestration plan references unknown agent %q", id)}
			}
			if strings.TrimSpace(agent.Reason) == "" {
				return &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: fmt.Sprintf("orchestration plan agent %q must include a reason", id)}
			}
			if _, ok := seen[id]; ok {
				return &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: fmt.Sprintf("orchestration plan references agent %q more than once", id)}
			}
			seen[id] = struct{}{}
		}
	}
	return nil
}

type stepDependency struct {
	StepID string
	NodeID string
}

func buildDynamicWorkflowFromPlan(plugin Plugin, command PluginCommand, task string, plan OrchestrationPlanV1, agents map[PluginAgentName]PluginAgent, model ModelID) (*WorkflowSpec, error) {
	if len(plan.Steps) == 0 {
		return nil, &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: "orchestration plan must include at least one step"}
	}

	hasExplicitDeps := false
	for i := range plan.Steps {
		if len(plan.Steps[i].DependsOn) > 0 {
			hasExplicitDeps = true
			break
		}
	}

	stepKeys := make([]string, len(plan.Steps))
	stepOutputNodes := make(map[string]string, len(plan.Steps))
	stepOrderByKey := make(map[string]int, len(plan.Steps))
	for i := range plan.Steps {
		rawID := strings.TrimSpace(plan.Steps[i].ID)
		if rawID == "" {
			rawID = fmt.Sprintf("step_%d", i+1)
		}
		if _, exists := stepOrderByKey[rawID]; exists {
			return nil, &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: fmt.Sprintf("duplicate step id %q", rawID)}
		}
		stepKeys[i] = rawID
		stepOrderByKey[rawID] = i
	}

	nodes := make([]WorkflowIntentNode, 0)
	usedNodeIDs := map[string]struct{}{}

	for i := range plan.Steps {
		step := plan.Steps[i]
		stepKey := stepKeys[i]

		var depKeys []string
		if hasExplicitDeps {
			depKeys = step.DependsOn
		} else if i > 0 {
			depKeys = []string{stepKeys[i-1]}
		}

		deps := make([]stepDependency, 0, len(depKeys))
		for _, depKeyRaw := range depKeys {
			depKey := strings.TrimSpace(depKeyRaw)
			if depKey == "" {
				return nil, &PluginOrchestrationError{Code: OrchestrationErrInvalidDependency, Message: fmt.Sprintf("step %q has empty depends_on", stepKey)}
			}
			depIndex, ok := stepOrderByKey[depKey]
			if !ok {
				return nil, &PluginOrchestrationError{Code: OrchestrationErrInvalidDependency, Message: fmt.Sprintf("step %q depends on unknown step %q", stepKey, depKey)}
			}
			if depIndex >= i {
				return nil, &PluginOrchestrationError{Code: OrchestrationErrInvalidDependency, Message: fmt.Sprintf("step %q depends on future step %q", stepKey, depKey)}
			}
			outputNodeID := stepOutputNodes[depKey]
			if outputNodeID == "" {
				return nil, &PluginOrchestrationError{Code: OrchestrationErrInvalidDependency, Message: fmt.Sprintf("missing output for dependency %q", depKey)}
			}
			deps = append(deps, stepDependency{StepID: depKey, NodeID: outputNodeID})
		}

		stepNodeIDs := make([]string, 0, len(step.Agents))
		for j := range step.Agents {
			selection := step.Agents[j]
			agentName := PluginAgentName(strings.TrimSpace(selection.ID))
			agent, ok := agents[agentName]
			if !ok {
				return nil, &PluginOrchestrationError{Code: OrchestrationErrUnknownAgent, Message: fmt.Sprintf("orchestration plan references unknown agent %q", agentName)}
			}
			nodeID, idErr := formatAgentNodeID(agentName)
			if idErr != nil {
				return nil, idErr
			}
			if _, ok := usedNodeIDs[nodeID]; ok {
				return nil, &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: fmt.Sprintf("duplicate node id %q", nodeID)}
			}

			toolRefs, toolErr := buildAgentToolRefs(agent, command)
			if toolErr != nil {
				return nil, toolErr
			}

			node := WorkflowIntentNode{
				ID:        nodeID,
				Type:      WorkflowNodeTypeLLM,
				System:    strings.TrimSpace(agent.SystemPrompt),
				User:      buildDynamicAgentUserPrompt(command, task, deps),
				Tools:     toolRefs,
				DependsOn: nil,
			}
			if len(toolRefs) > 0 {
				node.ToolExecution = &workflowintent.ToolExecution{
					Mode: workflowintent.ToolExecutionModeClient,
				}
			}
			if len(deps) > 0 {
				node.DependsOn = dependencyNodeIDs(deps)
			}
			nodes = append(nodes, node)
			stepNodeIDs = append(stepNodeIDs, nodeID)
			usedNodeIDs[nodeID] = struct{}{}
		}

		if len(stepNodeIDs) == 0 {
			return nil, &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: fmt.Sprintf("step %q produced no nodes", stepKey)}
		}

		outputNodeID := stepNodeIDs[0]
		if len(stepNodeIDs) > 1 {
			joinID, joinErr := formatStepJoinNodeID(stepKey)
			if joinErr != nil {
				return nil, joinErr
			}
			if _, ok := usedNodeIDs[joinID]; ok {
				return nil, &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: fmt.Sprintf("duplicate node id %q", joinID)}
			}
			nodes = append(nodes, WorkflowIntentNode{
				ID:        joinID,
				Type:      WorkflowNodeTypeJoinAll,
				DependsOn: stepNodeIDs,
			})
			usedNodeIDs[joinID] = struct{}{}
			outputNodeID = joinID
		}

		stepOutputNodes[stepKey] = outputNodeID
	}

	if len(stepOutputNodes) == 0 {
		return nil, &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: "orchestration plan produced no nodes"}
	}

	terminalOutputs := terminalStepOutputs(stepKeys, plan.Steps, stepOutputNodes, hasExplicitDeps)
	if len(terminalOutputs) == 0 {
		return nil, &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: "orchestration plan produced no terminal outputs"}
	}

	synthID := "orchestrator_synthesize"
	if _, ok := usedNodeIDs[synthID]; ok {
		return nil, &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: fmt.Sprintf("duplicate node id %q", synthID)}
	}
	synth := WorkflowIntentNode{
		ID:        synthID,
		Type:      WorkflowNodeTypeLLM,
		User:      buildDynamicSynthesisPrompt(command, task, terminalOutputs),
		DependsOn: terminalOutputs,
	}
	nodes = append(nodes, synth)

	spec := WorkflowSpec{
		Kind:    WorkflowKindIntent,
		Name:    strings.TrimSpace(plugin.Manifest.Name),
		Model:   strings.TrimSpace(model.String()),
		Nodes:   nodes,
		Outputs: []WorkflowIntentOutputRef{{Name: "result", From: synthID}},
	}
	if plan.MaxParallelism != nil {
		spec.MaxParallelism = plan.MaxParallelism
	}
	if spec.Name == "" {
		spec.Name = command.Name.String()
	}
	return &spec, nil
}

func buildDynamicAgentUserPrompt(command PluginCommand, task string, deps []stepDependency) string {
	var b strings.Builder
	if strings.TrimSpace(command.Prompt) != "" {
		b.WriteString(strings.TrimSpace(command.Prompt))
		b.WriteString("\n\n")
	}
	b.WriteString("USER_TASK:\n")
	b.WriteString(strings.TrimSpace(task))
	if len(deps) > 0 {
		b.WriteString("\n\nPREVIOUS_STEP_OUTPUTS:\n")
		for _, dep := range deps {
			if strings.TrimSpace(dep.StepID) != "" {
				b.WriteString("- ")
				b.WriteString(strings.TrimSpace(dep.StepID))
				b.WriteString(": {{")
				b.WriteString(dep.NodeID)
				b.WriteString("}}\n")
			} else {
				b.WriteString("- {{")
				b.WriteString(dep.NodeID)
				b.WriteString("}}\n")
			}
		}
	}
	return b.String()
}

func buildDynamicSynthesisPrompt(command PluginCommand, task string, outputs []string) string {
	var b strings.Builder
	b.WriteString("Synthesize the results and complete the task.")
	if strings.TrimSpace(command.Prompt) != "" {
		b.WriteString("\n\nCOMMAND:\n")
		b.WriteString(strings.TrimSpace(command.Prompt))
	}
	b.WriteString("\n\nUSER_TASK:\n")
	b.WriteString(strings.TrimSpace(task))
	if len(outputs) > 0 {
		b.WriteString("\n\nRESULTS:\n")
		for _, nodeID := range outputs {
			b.WriteString("- {{")
			b.WriteString(nodeID)
			b.WriteString("}}\n")
		}
	}
	return b.String()
}

func dependencyNodeIDs(deps []stepDependency) []string {
	if len(deps) == 0 {
		return nil
	}
	out := make([]string, 0, len(deps))
	for _, dep := range deps {
		if dep.NodeID == "" {
			continue
		}
		out = append(out, dep.NodeID)
	}
	return out
}

func terminalStepOutputs(stepKeys []string, steps []OrchestrationPlanStepV1, outputs map[string]string, explicit bool) []string {
	if len(stepKeys) == 0 {
		return nil
	}
	if !explicit {
		lastKey := stepKeys[len(stepKeys)-1]
		if out := outputs[lastKey]; out != "" {
			return []string{out}
		}
		return nil
	}
	depended := map[string]struct{}{}
	for i := range steps {
		for _, dep := range steps[i].DependsOn {
			id := strings.TrimSpace(dep)
			if id == "" {
				continue
			}
			depended[id] = struct{}{}
		}
	}
	var terminal []string
	for _, key := range stepKeys {
		if _, ok := depended[key]; ok {
			continue
		}
		if out := outputs[key]; out != "" {
			terminal = append(terminal, out)
		}
	}
	return terminal
}

func buildAgentToolRefs(agent PluginAgent, command PluginCommand) ([]workflowintent.ToolRef, error) {
	var names []ToolName
	if len(agent.Tools) > 0 {
		names = agent.Tools
	} else if len(command.Tools) > 0 {
		names = command.Tools
	} else {
		names = defaultDynamicToolNames()
	}
	if len(names) == 0 {
		return nil, nil
	}
	allowed := AllowedToolNamesSet()
	refs := make([]workflowintent.ToolRef, 0, len(names))
	seen := map[ToolName]struct{}{}
	for _, name := range names {
		if name == "" {
			continue
		}
		if _, ok := allowed[name]; !ok {
			return nil, &PluginOrchestrationError{Code: OrchestrationErrUnknownTool, Message: fmt.Sprintf("unknown tool %q", name)}
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		refs = append(refs, workflowintent.ToolRef{
			Tool: llm.Tool{
				Type: llm.ToolTypeFunction,
				Function: &llm.FunctionTool{
					Name: name,
				},
			},
		})
	}
	return refs, nil
}

func defaultDynamicToolNames() []ToolName {
	return []ToolName{
		ToolNameFSReadFile,
		ToolNameFSListFiles,
		ToolNameFSSearch,
	}
}

func formatAgentNodeID(name PluginAgentName) (string, error) {
	token := sanitizeNodeToken(name.String())
	if token == "" {
		return "", &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: "agent id must contain alphanumeric characters"}
	}
	return "agent_" + token, nil
}

func formatStepJoinNodeID(stepKey string) (string, error) {
	token := sanitizeNodeToken(stepKey)
	if token == "" {
		return "", &PluginOrchestrationError{Code: OrchestrationErrInvalidPlan, Message: "step id must contain alphanumeric characters"}
	}
	if strings.HasPrefix(token, "step_") {
		return token + "_join", nil
	}
	return "step_" + token + "_join", nil
}

func sanitizeNodeToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	out = strings.Trim(out, "_")
	return out
}

func specRequiresTools(spec *WorkflowSpec) bool {
	if spec == nil {
		return false
	}
	var checkNode func(node WorkflowIntentNode) bool
	checkNode = func(node WorkflowIntentNode) bool {
		if node.Type == WorkflowNodeTypeLLM && len(node.Tools) > 0 {
			return true
		}
		if node.Type == WorkflowNodeTypeMapFanout && node.SubNode != nil {
			return checkNode(*node.SubNode)
		}
		return false
	}
	for i := range spec.Nodes {
		if checkNode(spec.Nodes[i]) {
			return true
		}
	}
	return false
}

func ensureModelSupportsTools(ctx context.Context, client *Client, model ModelID) error {
	if client == nil {
		return errors.New("plugin converter: client required")
	}
	modelID := strings.TrimSpace(model.String())
	if modelID == "" {
		return errors.New("plugin converter: model required")
	}
	path := routes.Models + "?capability=" + string(ModelCapabilityTools)
	var resp generated.ModelsResponse
	if err := client.sendAndDecode(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return err
	}
	for i := range resp.Models {
		entry := resp.Models[i]
		if strings.TrimSpace(entry.ModelId) == modelID {
			return nil
		}
	}
	return &PluginOrchestrationError{
		Code:    OrchestrationErrInvalidToolConfig,
		Message: fmt.Sprintf("model %q does not support tool calling", modelID),
	}
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
	return "", fmt.Errorf("unknown tool %q for mode detection; expected one of: bash, write_file, fs_read_file, fs_search, fs_list_files", name)
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
