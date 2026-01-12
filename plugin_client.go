package sdk

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// PluginsClient loads plugins from GitHub, converts them to workflow, and executes via /runs.
type PluginsClient struct {
	client    *Client
	loader    *PluginLoader
	converter *PluginConverter
	runner    *PluginRunner
}

func newPluginsClient(c *Client) *PluginsClient {
	return &PluginsClient{
		client:    c,
		loader:    NewPluginLoader(),
		converter: NewPluginConverter(c),
		runner:    NewPluginRunner(c),
	}
}

// Plugins returns the plugin helper client (lazily initialized).
func (c *Client) Plugins() *PluginsClient {
	if c == nil {
		return nil
	}
	c.pluginsOnce.Do(func() {
		c.plugins = newPluginsClient(c)
	})
	return c.plugins
}

type pluginQuickRunOptions struct {
	cfg PluginRunConfig
}

type PluginQuickRunOption func(*pluginQuickRunOptions)

// WithToolRegistry configures the tool registry used for client-side tool execution.
func WithToolRegistry(reg *ToolRegistry) PluginQuickRunOption {
	return func(o *pluginQuickRunOptions) {
		o.cfg.ToolHandler = reg
	}
}

// WithConverterModel overrides the model used for plugin→workflow conversion.
func WithConverterModel(model string) PluginQuickRunOption {
	return func(o *pluginQuickRunOptions) {
		o.cfg.ConverterModel = NewModelID(model)
	}
}

// WithPluginModel overrides the model used to execute the generated workflow.
func WithPluginModel(model string) PluginQuickRunOption {
	return func(o *pluginQuickRunOptions) {
		o.cfg.Model = NewModelID(model)
	}
}

// WithOrchestrationMode sets how agents are selected and orchestrated.
func WithOrchestrationMode(mode OrchestrationMode) PluginQuickRunOption {
	return func(o *pluginQuickRunOptions) {
		o.cfg.OrchestrationMode = mode
	}
}

func (p *PluginsClient) Load(ctx context.Context, pluginURL string) (*Plugin, error) {
	if p == nil || p.client == nil || p.loader == nil {
		return nil, errors.New("plugins client: not initialized")
	}
	pluginURL = strings.TrimSpace(pluginURL)
	if pluginURL == "" {
		return nil, errors.New("plugins client: plugin url required")
	}

	return p.loader.Load(ctx, pluginURL)
}

func (p *PluginsClient) Run(ctx context.Context, plugin *Plugin, command string, cfg PluginRunConfig) (*PluginRunResult, error) {
	if p == nil || p.client == nil || p.runner == nil || p.converter == nil {
		return nil, errors.New("plugins client: not initialized")
	}
	if plugin == nil {
		return nil, errors.New("plugins client: plugin required")
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, errors.New("plugins client: command required")
	}
	cfg.UserTask = strings.TrimSpace(cfg.UserTask)
	if cfg.UserTask == "" {
		return nil, errors.New("plugins client: user task required")
	}

	mode, err := normalizeOrchestrationMode(cfg.OrchestrationMode)
	if err != nil {
		return nil, err
	}

	converter := p.converter
	if !cfg.ConverterModel.IsEmpty() {
		converter = NewPluginConverter(p.client, WithPluginConverterModel(cfg.ConverterModel.String()))
	}

	var spec *WorkflowSpec
	switch mode {
	case OrchestrationModeDynamic:
		spec, err = converter.ToWorkflowDynamic(ctx, plugin, command, cfg.UserTask)
	default:
		spec, err = converter.ToWorkflow(ctx, plugin, command, cfg.UserTask)
	}
	if err != nil {
		return nil, err
	}
	if err := applyWorkflowModelOverride(spec, cfg.Model); err != nil {
		return nil, err
	}
	// Validate tool capability against the final execution model (after override).
	if specRequiresTools(spec) {
		execModel := cfg.Model
		if execModel.IsEmpty() {
			execModel = NewModelID(spec.Model)
		}
		if !execModel.IsEmpty() {
			if toolErr := ensureModelSupportsTools(ctx, p.client, execModel); toolErr != nil {
				return nil, toolErr
			}
		}
	}
	return p.runner.Run(ctx, spec, cfg)
}

// QuickRun is a convenience method that loads a plugin and executes a command in one call.
func (p *PluginsClient) QuickRun(ctx context.Context, pluginURL, command, task string, opts ...PluginQuickRunOption) (*PluginRunResult, error) {
	if p == nil {
		return nil, errors.New("plugins client: not initialized")
	}
	var o pluginQuickRunOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	o.cfg.UserTask = task
	plugin, err := p.Load(ctx, pluginURL)
	if err != nil {
		return nil, err
	}
	return p.Run(ctx, plugin, command, o.cfg)
}

func normalizeOrchestrationMode(mode OrchestrationMode) (OrchestrationMode, error) {
	if strings.TrimSpace(string(mode)) == "" {
		return OrchestrationModeDAG, nil
	}
	if !mode.Valid() {
		return "", fmt.Errorf("plugins client: invalid orchestration mode %q", mode)
	}
	return mode, nil
}
