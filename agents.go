package sdk

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/modelrelay/modelrelay/sdk/go/generated"
	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

// Agent represents a project-scoped agent.
type Agent = generated.Agent

// AgentVersion is an immutable snapshot of agent configuration.
type AgentVersion = generated.AgentVersion

// AgentResource combines agent metadata with its current version snapshot.
type AgentResource = generated.AgentResource

// AgentToolRef references a tool definition, hook, or fragment.
type AgentToolRef = generated.AgentToolRef

// AgentFragmentRef references a fragment tool definition.
type AgentFragmentRef = generated.AgentFragmentRef

// AgentCreateRequest creates a new agent and initial version.
type AgentCreateRequest = generated.AgentCreateRequest

// AgentUpdateRequest updates agent metadata or creates a new version.
type AgentUpdateRequest = generated.AgentUpdateRequest

// AgentRunOptions overrides agent execution parameters at runtime.
type AgentRunOptions = generated.AgentRunOptions

// AgentRunOptionsToolFailurePolicy enumerates tool failure behaviors.
type AgentRunOptionsToolFailurePolicy = generated.AgentRunOptionsToolFailurePolicy

// AgentRunResponse contains the output, usage, and optional step trace.
type AgentRunResponse = generated.AgentRunResponse

// AgentRunRequest runs an agent with the provided input.
type AgentRunRequest struct {
	Input      []llm.InputItem  `json:"input"`
	Options    *AgentRunOptions `json:"options,omitempty"`
	CustomerID *string          `json:"customer_id,omitempty"`
}

// AgentTestRequest runs an agent with mocked tool outputs.
type AgentTestRequest struct {
	Input     []llm.InputItem  `json:"input"`
	MockTools map[string]any   `json:"mock_tools,omitempty"`
	Options   *AgentRunOptions `json:"options,omitempty"`
}

// AgentsClient provides methods for managing and running agents.
type AgentsClient struct {
	client *Client
}

// ensureInitialized returns an error if the client is not properly initialized.
func (c *AgentsClient) ensureInitialized() error {
	if c == nil || c.client == nil {
		return fmt.Errorf("sdk: agents client not initialized")
	}
	return nil
}

// List returns all agents for a project.
func (c *AgentsClient) List(ctx context.Context, projectID uuid.UUID) ([]AgentResource, error) {
	if err := c.ensureInitialized(); err != nil {
		return nil, err
	}
	if projectID == uuid.Nil {
		return nil, fmt.Errorf("sdk: project_id is required")
	}
	path := fmt.Sprintf("/projects/%s/agents", projectID.String())
	var resp generated.AgentListResponse
	if err := c.client.sendAndDecode(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	if resp.Agents == nil {
		return []AgentResource{}, nil
	}
	return *resp.Agents, nil
}

// Get returns a single agent by slug.
func (c *AgentsClient) Get(ctx context.Context, projectID uuid.UUID, slug string) (AgentResource, error) {
	if err := c.ensureInitialized(); err != nil {
		return AgentResource{}, err
	}
	if projectID == uuid.Nil {
		return AgentResource{}, fmt.Errorf("sdk: project_id is required")
	}
	s := strings.TrimSpace(slug)
	if s == "" {
		return AgentResource{}, fmt.Errorf("sdk: slug is required")
	}
	path := fmt.Sprintf("/projects/%s/agents/%s", projectID.String(), s)
	var resp generated.AgentResponse
	if err := c.client.sendAndDecode(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return AgentResource{}, err
	}
	if resp.Agent == nil {
		return AgentResource{}, fmt.Errorf("sdk: agent not found")
	}
	return *resp.Agent, nil
}

// Create creates a new agent and initial version.
func (c *AgentsClient) Create(ctx context.Context, projectID uuid.UUID, req AgentCreateRequest) (AgentResource, error) {
	if err := c.ensureInitialized(); err != nil {
		return AgentResource{}, err
	}
	if projectID == uuid.Nil {
		return AgentResource{}, fmt.Errorf("sdk: project_id is required")
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Slug) == "" {
		return AgentResource{}, fmt.Errorf("sdk: name and slug are required")
	}
	path := fmt.Sprintf("/projects/%s/agents", projectID.String())
	var resp generated.AgentResponse
	if err := c.client.sendAndDecode(ctx, http.MethodPost, path, req, &resp); err != nil {
		return AgentResource{}, err
	}
	if resp.Agent == nil {
		return AgentResource{}, fmt.Errorf("sdk: agent not created")
	}
	return *resp.Agent, nil
}

// Update updates agent metadata or creates a new version when configuration changes.
func (c *AgentsClient) Update(ctx context.Context, projectID uuid.UUID, slug string, req AgentUpdateRequest) (AgentResource, error) {
	if err := c.ensureInitialized(); err != nil {
		return AgentResource{}, err
	}
	if projectID == uuid.Nil {
		return AgentResource{}, fmt.Errorf("sdk: project_id is required")
	}
	s := strings.TrimSpace(slug)
	if s == "" {
		return AgentResource{}, fmt.Errorf("sdk: slug is required")
	}
	path := fmt.Sprintf("/projects/%s/agents/%s", projectID.String(), s)
	var resp generated.AgentResponse
	if err := c.client.sendAndDecode(ctx, http.MethodPatch, path, req, &resp); err != nil {
		return AgentResource{}, err
	}
	if resp.Agent == nil {
		return AgentResource{}, fmt.Errorf("sdk: agent not updated")
	}
	return *resp.Agent, nil
}

// Delete deletes an agent by slug.
func (c *AgentsClient) Delete(ctx context.Context, projectID uuid.UUID, slug string) error {
	if err := c.ensureInitialized(); err != nil {
		return err
	}
	if projectID == uuid.Nil {
		return fmt.Errorf("sdk: project_id is required")
	}
	s := strings.TrimSpace(slug)
	if s == "" {
		return fmt.Errorf("sdk: slug is required")
	}
	path := fmt.Sprintf("/projects/%s/agents/%s", projectID.String(), s)
	return c.client.sendAndDecode(ctx, http.MethodDelete, path, nil, nil)
}

// Run executes an agent and returns the output plus usage summary.
func (c *AgentsClient) Run(ctx context.Context, projectID uuid.UUID, slug string, req AgentRunRequest) (AgentRunResponse, error) {
	if err := c.ensureInitialized(); err != nil {
		return AgentRunResponse{}, err
	}
	if projectID == uuid.Nil {
		return AgentRunResponse{}, fmt.Errorf("sdk: project_id is required")
	}
	s := strings.TrimSpace(slug)
	if s == "" {
		return AgentRunResponse{}, fmt.Errorf("sdk: slug is required")
	}
	if len(req.Input) == 0 {
		return AgentRunResponse{}, fmt.Errorf("sdk: input is required")
	}
	path := fmt.Sprintf("/projects/%s/agents/%s/run", projectID.String(), s)
	var resp AgentRunResponse
	if err := c.client.sendAndDecode(ctx, http.MethodPost, path, req, &resp); err != nil {
		return AgentRunResponse{}, err
	}
	return resp, nil
}

// Test executes an agent with mocked tools and returns the full trace.
func (c *AgentsClient) Test(ctx context.Context, projectID uuid.UUID, slug string, req AgentTestRequest) (AgentRunResponse, error) {
	if err := c.ensureInitialized(); err != nil {
		return AgentRunResponse{}, err
	}
	if projectID == uuid.Nil {
		return AgentRunResponse{}, fmt.Errorf("sdk: project_id is required")
	}
	s := strings.TrimSpace(slug)
	if s == "" {
		return AgentRunResponse{}, fmt.Errorf("sdk: slug is required")
	}
	if len(req.Input) == 0 {
		return AgentRunResponse{}, fmt.Errorf("sdk: input is required")
	}
	path := fmt.Sprintf("/projects/%s/agents/%s/test", projectID.String(), s)
	var resp AgentRunResponse
	if err := c.client.sendAndDecode(ctx, http.MethodPost, path, req, &resp); err != nil {
		return AgentRunResponse{}, err
	}
	return resp, nil
}

// Replay executes an agent using mocked tools intended for deterministic replay.
func (c *AgentsClient) Replay(ctx context.Context, projectID uuid.UUID, slug string, req AgentTestRequest) (AgentRunResponse, error) {
	if err := c.ensureInitialized(); err != nil {
		return AgentRunResponse{}, err
	}
	if projectID == uuid.Nil {
		return AgentRunResponse{}, fmt.Errorf("sdk: project_id is required")
	}
	s := strings.TrimSpace(slug)
	if s == "" {
		return AgentRunResponse{}, fmt.Errorf("sdk: slug is required")
	}
	if len(req.Input) == 0 {
		return AgentRunResponse{}, fmt.Errorf("sdk: input is required")
	}
	path := fmt.Sprintf("/projects/%s/agents/%s/replay", projectID.String(), s)
	var resp AgentRunResponse
	if err := c.client.sendAndDecode(ctx, http.MethodPost, path, req, &resp); err != nil {
		return AgentRunResponse{}, err
	}
	return resp, nil
}
