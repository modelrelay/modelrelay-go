package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
	"github.com/modelrelay/modelrelay/sdk/go/routes"
)

// PluginsClient loads, converts, and executes plugins via workflows and the /runs API.
type PluginsClient struct {
	client *Client
	runner *PluginRunner
}

func newPluginsClient(c *Client) *PluginsClient {
	runner := NewPluginRunner(c)
	return &PluginsClient{
		client: c,
		runner: runner,
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

// WithPluginPollInterval overrides the poll interval for run status updates.
func WithPluginPollInterval(d time.Duration) PluginQuickRunOption {
	return func(o *pluginQuickRunOptions) {
		o.cfg.PollInterval = d
	}
}

// doJSONPost performs a JSON POST request and decodes the response into the provided output pointer.
func (c *Client) doJSONPost(ctx context.Context, route string, payload any, out any) error {
	req, err := c.newJSONRequest(ctx, http.MethodPost, route, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, _, err := c.send(req, nil, nil)
	if err != nil {
		return err
	}
	body, readErr := io.ReadAll(resp.Body)
	//nolint:errcheck // best-effort cleanup on return
	_ = resp.Body.Close()
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode >= 400 {
		return decodeAPIErrorFromBytes(resp.StatusCode, body, nil)
	}
	return json.Unmarshal(body, out)
}

func (p *PluginsClient) Load(ctx context.Context, pluginURL string) (*Plugin, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("plugins client: not initialized")
	}
	pluginURL = strings.TrimSpace(pluginURL)
	if pluginURL == "" {
		return nil, errors.New("plugins client: plugin url required")
	}

	payload := generated.PluginsLoadRequest{SourceUrl: pluginURL}
	var out Plugin
	if err := p.client.doJSONPost(ctx, routes.PluginsLoad, payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (p *PluginsClient) Run(ctx context.Context, plugin *Plugin, command string, cfg PluginRunConfig) (*PluginRunResult, error) {
	if p == nil || p.client == nil || p.runner == nil {
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

	sourceURL := strings.TrimSpace(plugin.URL.String())
	if sourceURL == "" {
		return nil, errors.New("plugins client: plugin url missing (load plugin from server first)")
	}

	runReq := generated.PluginsRunRequest{
		SourceUrl:      sourceURL,
		Command:        command,
		UserTask:       cfg.UserTask,
		Model:          optionalModelID(cfg.Model),
		ConverterModel: optionalModelID(cfg.ConverterModel),
	}
	var start generated.PluginsRunResponseV0
	if err := p.client.doJSONPost(ctx, routes.PluginsRuns, runReq, &start); err != nil {
		return nil, err
	}

	runID, err := ParseRunID(start.RunId.String())
	if err != nil {
		return nil, err
	}
	result, err := p.runner.Wait(ctx, runID, cfg)
	if err != nil {
		return nil, err
	}
	applyConversionMeta(result, start)
	return result, nil
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

func applyConversionMeta(out *PluginRunResult, start generated.PluginsRunResponseV0) {
	if out == nil {
		return
	}
	if start.ConversionModel != nil {
		out.ConversionModel = NewModelID(string(*start.ConversionModel))
	}
	if start.ConversionResponseId != nil {
		out.ConversionResponseID = strings.TrimSpace(*start.ConversionResponseId)
	}
	if start.ConversionUsage != nil {
		out.ConversionUsage = usageFromGenerated(*start.ConversionUsage)
	}
}

// optionalModelID converts a ModelID to a pointer for optional API fields.
func optionalModelID(m ModelID) *generated.ModelId {
	if m.IsEmpty() {
		return nil
	}
	s := m.String()
	return (*generated.ModelId)(&s)
}

func usageFromGenerated(u generated.Usage) Usage {
	var out Usage
	if u.InputTokens != nil {
		//nolint:gosec // G115: token counts are small values, overflow not possible in practice
		out.InputTokens = int64(*u.InputTokens)
	}
	if u.OutputTokens != nil {
		//nolint:gosec // G115: token counts are small values, overflow not possible in practice
		out.OutputTokens = int64(*u.OutputTokens)
	}
	if u.TotalTokens != nil {
		//nolint:gosec // G115: token counts are small values, overflow not possible in practice
		out.TotalTokens = int64(*u.TotalTokens)
	}
	return out
}
