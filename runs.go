package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/modelrelay/modelrelay/platform/routes"
	"github.com/modelrelay/modelrelay/platform/workflow"
)

// WorkflowSpecV0 is the request payload shape for workflow.v0.
//
// This is a type alias to the server's canonical type, so it cannot drift.
type WorkflowSpecV0 = workflow.SpecV0

const (
	WorkflowKindV0 workflow.Kind = workflow.KindWorkflowV0

	WorkflowNodeTypeLLMResponses  workflow.NodeType = workflow.NodeTypeLLMResponses
	WorkflowNodeTypeJoinAll       workflow.NodeType = workflow.NodeTypeJoinAll
	WorkflowNodeTypeTransformJSON workflow.NodeType = workflow.NodeTypeTransformJSON
)

// RunID is the workflow run identifier type.
type RunID = workflow.RunID

// PlanHash is the hash of a compiled workflow plan.
type PlanHash = workflow.PlanHash

// RunEventV0Envelope is the stable, append-only wire envelope for workflow run history.
type RunEventV0Envelope = workflow.RunEventV0

// RunEventV0 is the strongly-typed (discriminated) run event union.
type RunEventV0 interface {
	isRunEventV0()
}

type RunEventV0Base struct {
	EnvelopeVersion string
	RunID           RunID
	Seq             int64
	TS              time.Time
}

type RunEventRunCompiledV0 struct {
	RunEventV0Base
	PlanHash PlanHash
}

type RunEventRunStartedV0 struct {
	RunEventV0Base
	PlanHash PlanHash
}

type RunEventRunCompletedV0 struct {
	RunEventV0Base
	PlanHash           PlanHash
	OutputsArtifactKey string
	OutputsInfo        workflow.PayloadInfoV0
}

type RunEventRunFailedV0 struct {
	RunEventV0Base
	PlanHash PlanHash
	Error    workflow.NodeErrorV0
}

type RunEventRunCanceledV0 struct {
	RunEventV0Base
	PlanHash PlanHash
	Error    workflow.NodeErrorV0
}

type RunEventNodeStartedV0 struct {
	RunEventV0Base
	NodeID workflow.NodeID
}

type RunEventNodeSucceededV0 struct {
	RunEventV0Base
	NodeID workflow.NodeID
}

type RunEventNodeFailedV0 struct {
	RunEventV0Base
	NodeID workflow.NodeID
	Error  workflow.NodeErrorV0
}

type RunEventNodeOutputV0 struct {
	RunEventV0Base
	NodeID      workflow.NodeID
	ArtifactKey string
	OutputInfo  workflow.PayloadInfoV0
}

func (RunEventRunCompiledV0) isRunEventV0()   {}
func (RunEventRunStartedV0) isRunEventV0()    {}
func (RunEventRunCompletedV0) isRunEventV0()  {}
func (RunEventRunFailedV0) isRunEventV0()     {}
func (RunEventRunCanceledV0) isRunEventV0()   {}
func (RunEventNodeStartedV0) isRunEventV0()   {}
func (RunEventNodeSucceededV0) isRunEventV0() {}
func (RunEventNodeFailedV0) isRunEventV0()    {}
func (RunEventNodeOutputV0) isRunEventV0()    {}

func decodeRunEventV0(env RunEventV0Envelope) (RunEventV0, error) {
	if strings.TrimSpace(env.EnvelopeVersion) == "" {
		return nil, ProtocolError{Message: "run event is missing envelope_version"}
	}
	if env.EnvelopeVersion != workflow.RunEventEnvelopeVersionV0 {
		return nil, ProtocolError{Message: "unsupported run event envelope_version: " + env.EnvelopeVersion}
	}
	if !env.RunID.Valid() {
		return nil, ProtocolError{Message: "run event has invalid run_id"}
	}
	if env.Seq < 1 {
		return nil, ProtocolError{Message: "run event has invalid seq"}
	}
	if env.TS.IsZero() {
		return nil, ProtocolError{Message: "run event has missing ts"}
	}

	base := RunEventV0Base{
		EnvelopeVersion: env.EnvelopeVersion,
		RunID:           env.RunID,
		Seq:             env.Seq,
		TS:              env.TS,
	}

	switch env.Type {
	case workflow.RunEventRunCompiled, workflow.RunEventRunStarted, workflow.RunEventRunCompleted, workflow.RunEventRunFailed, workflow.RunEventRunCanceled:
		if env.NodeID.Valid() {
			return nil, ProtocolError{Message: "run-scoped event must not include node_id"}
		}
		if env.PlanHash == nil || !env.PlanHash.Valid() {
			return nil, ProtocolError{Message: "run-scoped event must include plan_hash"}
		}
		planHash := *env.PlanHash

		switch env.Type {
		case workflow.RunEventRunCompiled:
			if env.Error != nil || env.OutputInfo != nil || env.ArtifactKey != "" || env.OutputsInfo != nil || env.OutputsArtifactKey != "" {
				return nil, ProtocolError{Message: "run_compiled must not include error/output_info/artifact fields"}
			}
			return RunEventRunCompiledV0{RunEventV0Base: base, PlanHash: planHash}, nil
		case workflow.RunEventRunStarted:
			if env.Error != nil || env.OutputInfo != nil || env.ArtifactKey != "" || env.OutputsInfo != nil || env.OutputsArtifactKey != "" {
				return nil, ProtocolError{Message: "run_started must not include error/output_info/artifact fields"}
			}
			return RunEventRunStartedV0{RunEventV0Base: base, PlanHash: planHash}, nil
		case workflow.RunEventRunCompleted:
			if env.Error != nil || env.OutputInfo != nil || env.ArtifactKey != "" {
				return nil, ProtocolError{Message: "run_completed must not include error/output_info/node artifact fields"}
			}
			if strings.TrimSpace(env.OutputsArtifactKey) == "" || env.OutputsInfo == nil {
				return nil, ProtocolError{Message: "run_completed must include outputs_artifact_key and outputs_info"}
			}
			if env.OutputsInfo.Included {
				return nil, ProtocolError{Message: "run_completed outputs_info.included must be false"}
			}
			return RunEventRunCompletedV0{RunEventV0Base: base, PlanHash: planHash, OutputsArtifactKey: env.OutputsArtifactKey, OutputsInfo: *env.OutputsInfo}, nil
		case workflow.RunEventRunFailed:
			if env.OutputInfo != nil || env.ArtifactKey != "" || env.OutputsInfo != nil || env.OutputsArtifactKey != "" {
				return nil, ProtocolError{Message: "run_failed must not include output_info/artifact fields"}
			}
			if env.Error == nil || strings.TrimSpace(env.Error.Message) == "" {
				return nil, ProtocolError{Message: "run_failed must include error"}
			}
			return RunEventRunFailedV0{RunEventV0Base: base, PlanHash: planHash, Error: *env.Error}, nil
		case workflow.RunEventRunCanceled:
			if env.OutputInfo != nil || env.ArtifactKey != "" || env.OutputsInfo != nil || env.OutputsArtifactKey != "" {
				return nil, ProtocolError{Message: "run_canceled must not include output_info/artifact fields"}
			}
			if env.Error == nil || strings.TrimSpace(env.Error.Message) == "" {
				return nil, ProtocolError{Message: "run_canceled must include error"}
			}
			return RunEventRunCanceledV0{RunEventV0Base: base, PlanHash: planHash, Error: *env.Error}, nil
		default:
			return nil, ProtocolError{Message: "unknown run event type"}
		}

	case workflow.RunEventNodeStarted, workflow.RunEventNodeSucceeded, workflow.RunEventNodeFailed, workflow.RunEventNodeOutput:
		if env.PlanHash != nil {
			return nil, ProtocolError{Message: "node-scoped event must not include plan_hash"}
		}
		if env.OutputsInfo != nil || env.OutputsArtifactKey != "" {
			return nil, ProtocolError{Message: "node-scoped event must not include outputs fields"}
		}
		if !env.NodeID.Valid() {
			return nil, ProtocolError{Message: "node-scoped event must include node_id"}
		}

		switch env.Type {
		case workflow.RunEventNodeStarted:
			if env.Error != nil || env.OutputInfo != nil || env.ArtifactKey != "" {
				return nil, ProtocolError{Message: "node_started must not include error/output_info/artifact_key"}
			}
			return RunEventNodeStartedV0{RunEventV0Base: base, NodeID: env.NodeID}, nil
		case workflow.RunEventNodeSucceeded:
			if env.Error != nil || env.OutputInfo != nil || env.ArtifactKey != "" {
				return nil, ProtocolError{Message: "node_succeeded must not include error/output_info/artifact_key"}
			}
			return RunEventNodeSucceededV0{RunEventV0Base: base, NodeID: env.NodeID}, nil
		case workflow.RunEventNodeFailed:
			if env.OutputInfo != nil || env.ArtifactKey != "" {
				return nil, ProtocolError{Message: "node_failed must not include output_info/artifact_key"}
			}
			if env.Error == nil || strings.TrimSpace(env.Error.Message) == "" {
				return nil, ProtocolError{Message: "node_failed must include error"}
			}
			return RunEventNodeFailedV0{RunEventV0Base: base, NodeID: env.NodeID, Error: *env.Error}, nil
		case workflow.RunEventNodeOutput:
			if env.Error != nil {
				return nil, ProtocolError{Message: "node_output must not include error"}
			}
			if env.OutputInfo == nil {
				return nil, ProtocolError{Message: "node_output must include output_info"}
			}
			if strings.TrimSpace(env.ArtifactKey) == "" {
				return nil, ProtocolError{Message: "node_output must include artifact_key"}
			}
			if env.OutputInfo.Included {
				return nil, ProtocolError{Message: "node_output output_info.included must be false"}
			}
			return RunEventNodeOutputV0{
				RunEventV0Base: base,
				NodeID:         env.NodeID,
				ArtifactKey:    env.ArtifactKey,
				OutputInfo:     *env.OutputInfo,
			}, nil
		default:
			return nil, ProtocolError{Message: "unknown run event type"}
		}
	default:
		return nil, ProtocolError{Message: "unknown run event type: " + string(env.Type)}
	}
}

// RunsClient calls the /runs endpoints.
type RunsClient struct {
	client *Client
}

// SchemaV0 fetches the canonical workflow.v0 JSON Schema from the API.
func (c *RunsClient) SchemaV0(ctx context.Context) (json.RawMessage, error) {
	req, err := c.client.newJSONRequest(ctx, http.MethodGet, routes.WorkflowV0Schema, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/schema+json")
	resp, _, err := c.client.send(req, nil, nil)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, decodeAPIError(resp, nil)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// RunEventSchemaV0 fetches the canonical run event envelope v0 JSON Schema from the API.
func (c *RunsClient) RunEventSchemaV0(ctx context.Context) (json.RawMessage, error) {
	req, err := c.client.newJSONRequest(ctx, http.MethodGet, routes.RunEventV0Schema, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/schema+json")
	resp, _, err := c.client.send(req, nil, nil)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, decodeAPIError(resp, nil)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

type runsCreateRequest struct {
	Spec WorkflowSpecV0 `json:"spec"`
}

type RunsCreateResponse struct {
	RunID  RunID  `json:"run_id"`
	Status string `json:"status"`
}

type RunsGetResponse struct {
	RunID       RunID                                   `json:"run_id"`
	Status      workflow.RunStatusV0                    `json:"status"`
	PlanHash    PlanHash                                `json:"plan_hash"`
	CostSummary workflow.RunCostSummaryV0               `json:"cost_summary"`
	Nodes       []workflow.NodeResultV0                 `json:"nodes,omitempty"`
	Outputs     map[workflow.OutputName]json.RawMessage `json:"outputs,omitempty"`
}

type RunsEventStream struct {
	body io.ReadCloser
	dec  *json.Decoder
}

func (s *RunsEventStream) Close() error {
	if s == nil || s.body == nil {
		return nil
	}
	return s.body.Close()
}

// Next returns the next run event, or ok=false when the stream is complete.
func (s *RunsEventStream) Next() (ev RunEventV0, ok bool, err error) {
	if s == nil || s.dec == nil {
		return nil, false, nil
	}
	var wire RunEventV0Envelope
	if decodeErr := s.dec.Decode(&wire); decodeErr != nil {
		if errors.Is(decodeErr, io.EOF) {
			return nil, false, nil
		}
		return nil, false, decodeErr
	}
	next, err := decodeRunEventV0(wire)
	if err != nil {
		return nil, false, err
	}
	return next, true, nil
}

// StreamEvents opens a streaming connection for /runs/{run_id}/events (NDJSON).
//
// The server may also support SSE, but the SDK always requests NDJSON for consistency.
func (c *RunsClient) StreamEvents(ctx context.Context, runID RunID, opts ...RunEventsOption) (*RunsEventStream, error) {
	if !runID.Valid() {
		return nil, ConfigError{Reason: "run id is required"}
	}
	options := buildRunEventsOptions(opts)

	path := routes.RunsEvents
	path = strings.ReplaceAll(path, "{run_id}", url.PathEscape(runID.String()))
	q := url.Values{}
	if options.afterSeq > 0 {
		q.Set("after_seq", strconv.FormatInt(options.afterSeq, 10))
	}
	if options.limit > 0 {
		q.Set("limit", strconv.Itoa(options.limit))
	}
	if !options.wait {
		q.Set("wait", "0")
	}
	if enc := q.Encode(); enc != "" {
		path = path + "?" + enc
	}

	req, err := c.client.newJSONRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/x-ndjson")
	resp, _, err := c.client.sendStreaming(req, options.retry)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		//nolint:errcheck // best-effort cleanup on return
		defer func() { _ = resp.Body.Close() }()
		return nil, decodeAPIError(resp, nil)
	}
	contentType := resp.Header.Get("Content-Type")
	if !isNDJSONContentType(contentType) {
		//nolint:errcheck // best-effort cleanup on protocol violation
		_ = resp.Body.Close()
		return nil, StreamProtocolError{
			ExpectedContentType: "application/x-ndjson",
			ReceivedContentType: contentType,
			Status:              resp.StatusCode,
		}
	}

	return &RunsEventStream{body: resp.Body, dec: json.NewDecoder(resp.Body)}, nil
}

// ListEvents performs a non-blocking fetch of /runs/{run_id}/events using wait=0.
func (c *RunsClient) ListEvents(ctx context.Context, runID RunID, opts ...RunEventsOption) ([]RunEventV0, error) {
	events, err := c.StreamEvents(ctx, runID, append(opts, WithRunEventsWait(false))...)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = events.Close() }()

	var out []RunEventV0
	for {
		ev, ok, err := events.Next()
		if err != nil {
			return nil, err
		}
		if !ok {
			return out, nil
		}
		out = append(out, ev)
	}
}

// Create starts a workflow run and returns its run id.
func (c *RunsClient) Create(ctx context.Context, spec WorkflowSpecV0, opts ...RunCreateOption) (*RunsCreateResponse, error) {
	options := buildRunCreateOptions(opts)

	payload := runsCreateRequest{Spec: spec}
	req, err := c.client.newJSONRequest(ctx, http.MethodPost, routes.Runs, payload)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, _, err := c.client.send(req, options.timeout, options.retry)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, decodeAPIError(resp, nil)
	}

	var out RunsCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Get returns the derived snapshot state for a run.
func (c *RunsClient) Get(ctx context.Context, runID RunID, opts ...RunGetOption) (*RunsGetResponse, error) {
	if !runID.Valid() {
		return nil, ConfigError{Reason: "run id is required"}
	}
	options := buildRunGetOptions(opts)

	path := strings.ReplaceAll(routes.RunsByID, "{run_id}", url.PathEscape(runID.String()))
	req, err := c.client.newJSONRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, _, err := c.client.send(req, options.timeout, options.retry)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, decodeAPIError(resp, nil)
	}
	var out RunsGetResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// WorkflowV0JSONSchema returns the JSON Schema (draft-07) for workflow.v0 as raw JSON.
func WorkflowV0JSONSchema() (json.RawMessage, error) {
	return json.Marshal(workflow.WorkflowV0JSONSchema())
}

type runCreateOptions struct {
	timeout *time.Duration
	retry   *RetryConfig
}

type RunCreateOption func(*runCreateOptions)

func buildRunCreateOptions(opts []RunCreateOption) runCreateOptions {
	var out runCreateOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&out)
		}
	}
	return out
}

func WithRunCreateTimeout(d time.Duration) RunCreateOption {
	return func(o *runCreateOptions) { o.timeout = &d }
}

func WithRunCreateRetry(cfg RetryConfig) RunCreateOption {
	return func(o *runCreateOptions) { o.retry = &cfg }
}

type runGetOptions struct {
	timeout *time.Duration
	retry   *RetryConfig
}

type RunGetOption func(*runGetOptions)

func buildRunGetOptions(opts []RunGetOption) runGetOptions {
	var out runGetOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&out)
		}
	}
	return out
}

func WithRunGetTimeout(d time.Duration) RunGetOption {
	return func(o *runGetOptions) { o.timeout = &d }
}

func WithRunGetRetry(cfg RetryConfig) RunGetOption {
	return func(o *runGetOptions) { o.retry = &cfg }
}

type runEventsOptions struct {
	afterSeq int64
	limit    int
	wait     bool
	retry    *RetryConfig
}

type RunEventsOption func(*runEventsOptions)

func buildRunEventsOptions(opts []RunEventsOption) runEventsOptions {
	out := runEventsOptions{wait: true}
	for _, opt := range opts {
		if opt != nil {
			opt(&out)
		}
	}
	if out.afterSeq < 0 {
		out.afterSeq = 0
	}
	if out.limit < 0 {
		out.limit = 0
	}
	return out
}

func WithRunEventsAfterSeq(seq int64) RunEventsOption {
	return func(o *runEventsOptions) { o.afterSeq = seq }
}

func WithRunEventsLimit(limit int) RunEventsOption {
	return func(o *runEventsOptions) { o.limit = limit }
}

func WithRunEventsWait(wait bool) RunEventsOption {
	return func(o *runEventsOptions) { o.wait = wait }
}

func WithRunEventsRetry(cfg RetryConfig) RunEventsOption {
	return func(o *runEventsOptions) { o.retry = &cfg }
}
