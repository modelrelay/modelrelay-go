package sdk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelrelay/modelrelay/platform/headers"
	"github.com/modelrelay/modelrelay/platform/routes"
	llm "github.com/modelrelay/modelrelay/providers"
)

// ResponsesClient calls the /responses endpoint.
type ResponsesClient struct {
	client *Client
}

// Create performs a blocking /responses request.
func (c *ResponsesClient) Create(ctx context.Context, req ResponseRequest, options ...ResponseOption) (*Response, error) {
	callOpts := buildResponseCallOptions(options)
	if callOpts.retry == nil {
		cfg := c.client.retryCfg
		cfg.RetryPost = true
		callOpts.retry = &cfg
	}

	requireModel := callOpts.headers == nil || strings.TrimSpace(callOpts.headers.Get(headers.CustomerID)) == ""
	if err := req.validate(requireModel); err != nil {
		return nil, err
	}

	reqPayload := newResponseRequestPayload(req)
	httpReq, err := c.client.newJSONRequest(ctx, http.MethodPost, routes.Responses, reqPayload)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json")
	applyResponseHeaders(httpReq, callOpts)
	resp, retryMeta, err := c.client.send(httpReq, callOpts.timeout, callOpts.retry)
	if err != nil {
		c.client.telemetry.log(ctx, LogLevelError, "responses_create_failed", map[string]any{"error": err.Error(), "retries": retryMeta})
		return nil, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	var respPayload Response
	if err := json.NewDecoder(resp.Body).Decode(&respPayload); err != nil {
		return nil, err
	}
	respPayload.RequestID = requestIDFromHeaders(resp.Header)
	return &respPayload, nil
}

// Stream opens a streaming connection for /responses.
func (c *ResponsesClient) Stream(ctx context.Context, req ResponseRequest, options ...ResponseOption) (*StreamHandle, error) {
	callOpts := buildResponseCallOptions(options)
	if callOpts.retry == nil {
		cfg := c.client.retryCfg
		cfg.RetryPost = true
		callOpts.retry = &cfg
	}

	requireModel := callOpts.headers == nil || strings.TrimSpace(callOpts.headers.Get(headers.CustomerID)) == ""
	if err := req.validate(requireModel); err != nil {
		return nil, err
	}

	payload := newResponseRequestPayload(req)
	httpReq, err := c.client.newJSONRequest(ctx, http.MethodPost, routes.Responses, payload)
	if err != nil {
		return nil, err
	}
	// All streaming uses unified NDJSON format
	httpReq.Header.Set("Accept", "application/x-ndjson")
	applyResponseHeaders(httpReq, callOpts)
	startedAt := time.Now()
	//nolint:bodyclose // resp.Body is transferred to stream and will be closed by stream.Close()
	resp, _, err := c.client.send(httpReq, callOpts.timeout, callOpts.retry)
	if err != nil {
		return nil, err
	}
	requestID := requestIDFromHeaders(resp.Header)
	reqCtx := newRequestContext(httpReq.Method, httpReq.URL.Path, req.model, requestID)
	stream := newNDJSONStream(ctx, resp.Body, c.client.telemetry, startedAt, reqCtx)
	return &StreamHandle{
		stream:    stream,
		RequestID: requestID,
	}, nil
}

// StreamJSON streams structured JSON responses for requests that set
// output_format.type=json_schema. It negotiates NDJSON per the structured
// streaming contract and decodes each update/completion payload into T.
func StreamJSON[T any](ctx context.Context, c *ResponsesClient, req ResponseRequest, options ...ResponseOption) (*StructuredJSONStream[T], error) {
	if req.outputFormat == nil || !req.outputFormat.IsStructured() {
		return nil, ConfigError{Reason: "output_format with type=json_schema is required for structured streaming"}
	}

	callOpts := buildResponseCallOptions(options)
	if callOpts.retry == nil {
		cfg := c.client.retryCfg
		cfg.RetryPost = true
		callOpts.retry = &cfg
	}

	requireModel := callOpts.headers == nil || strings.TrimSpace(callOpts.headers.Get(headers.CustomerID)) == ""
	if err := req.validate(requireModel); err != nil {
		return nil, err
	}

	payload := newResponseRequestPayload(req)
	httpReq, err := c.client.newJSONRequest(ctx, http.MethodPost, routes.Responses, payload)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/x-ndjson")
	applyResponseHeaders(httpReq, callOpts)

	//nolint:bodyclose // resp.Body is owned by the StructuredJSONStream
	resp, retryMeta, err := c.client.send(httpReq, callOpts.timeout, callOpts.retry)
	if err != nil {
		return nil, err
	}
	contentType := resp.Header.Get("Content-Type")
	if !isNDJSONContentType(contentType) {
		// Best-effort cleanup before returning a typed transport error.
		//nolint:errcheck // best-effort cleanup on protocol violation
		_ = resp.Body.Close()
		return nil, TransportError{
			Message: fmt.Sprintf("expected NDJSON structured stream, got Content-Type %q", contentType),
			Retry:   retryMeta,
		}
	}

	return newStructuredJSONStream[T](ctx, resp.Body, requestIDFromHeaders(resp.Header), retryMeta), nil
}

type responseRequestPayload struct {
	Provider        string            `json:"provider,omitempty"`
	Model           string            `json:"model,omitempty"`
	Input           []llm.InputItem   `json:"input"`
	OutputFormat    *llm.OutputFormat `json:"output_format,omitempty"`
	MaxOutputTokens int64             `json:"max_output_tokens,omitempty"`
	Temperature     *float64          `json:"temperature,omitempty"`
	Stop            []string          `json:"stop,omitempty"`
	Tools           []llm.Tool        `json:"tools,omitempty"`
	ToolChoice      *llm.ToolChoice   `json:"tool_choice,omitempty"`
}

func newResponseRequestPayload(req ResponseRequest) responseRequestPayload {
	payload := responseRequestPayload{
		Input:           req.input,
		OutputFormat:    req.outputFormat,
		MaxOutputTokens: req.maxOutputTokens,
		Temperature:     req.temperature,
		ToolChoice:      req.toolChoice,
	}
	if !req.provider.IsEmpty() {
		payload.Provider = req.provider.String()
	}
	if !req.model.IsEmpty() {
		payload.Model = req.model.String()
	}
	if len(req.stop) > 0 {
		payload.Stop = req.stop
	}
	if len(req.tools) > 0 {
		payload.Tools = req.tools
	}
	return payload
}

type ndjsonStream struct {
	ctx       context.Context
	reader    *bufio.Reader
	body      io.ReadCloser
	telemetry TelemetryHooks
	closed    bool
	closeOnce sync.Once
	mu        sync.Mutex
	done      chan struct{}

	// Metadata from start record propagated to subsequent events
	startRequestID string
	startModel     string

	startedAt       time.Time
	firstTokenFired bool
	requestCtx      RequestContext
}

func newNDJSONStream(ctx context.Context, body io.ReadCloser, telemetry TelemetryHooks, startedAt time.Time, reqCtx RequestContext) *ndjsonStream {
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	stream := &ndjsonStream{
		ctx:        ctx,
		reader:     bufio.NewReader(body),
		body:       body,
		telemetry:  telemetry,
		done:       make(chan struct{}),
		startedAt:  startedAt,
		requestCtx: reqCtx,
	}
	go func() {
		select {
		case <-ctx.Done():
			//nolint:errcheck // best-effort cleanup in goroutine
			_ = stream.Close()
		case <-stream.done:
			return
		}
	}()
	return stream
}

func (s *ndjsonStream) Next() (StreamEvent, bool, error) {
	if s.isClosed() {
		return StreamEvent{}, false, nil
	}
	for {
		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && len(line) == 0 {
				//nolint:errcheck // best-effort cleanup on EOF
				_ = s.Close()
				return StreamEvent{}, false, nil
			}
			if errors.Is(err, context.Canceled) {
				return StreamEvent{}, false, nil
			}
			if len(line) == 0 {
				return StreamEvent{}, false, err
			}
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		event, perr := parseNDJSONEvent(line)
		if perr != nil {
			return StreamEvent{}, false, perr
		}

		// Capture metadata from start record
		if event.Kind == llm.StreamEventKindMessageStart {
			if event.ResponseID != "" {
				s.startRequestID = event.ResponseID
			}
			if !event.Model.IsEmpty() {
				s.startModel = event.Model.String()
			}
		}

		// Propagate metadata from start record to subsequent events
		if event.ResponseID == "" && s.startRequestID != "" {
			event.ResponseID = s.startRequestID
		}
		if event.Model.IsEmpty() && s.startModel != "" {
			event.Model = NewModelID(s.startModel)
		}

		// Update request context with response metadata.
		if s.requestCtx.ResponseID == nil && event.ResponseID != "" {
			respID := event.ResponseID
			s.requestCtx.ResponseID = &respID
		}
		if s.requestCtx.Model == nil && !event.Model.IsEmpty() {
			m := event.Model
			s.requestCtx.Model = &m
		}

		// First-token latency hook (TTFT).
		if !s.firstTokenFired && event.TextDelta != "" {
			s.firstTokenFired = true
			if s.telemetry.OnStreamFirstToken != nil {
				latency := time.Since(s.startedAt)
				s.telemetry.OnStreamFirstToken(s.ctx, latency, s.requestCtx)
			}
		}

		if s.telemetry.OnStreamEvent != nil {
			s.telemetry.OnStreamEvent(s.ctx, event)
		}
		s.telemetry.metric(s.ctx, "sdk_stream_events_total", 1, map[string]string{"event": event.EventName()})
		return event, true, nil
	}
}

func (s *ndjsonStream) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		close(s.done)
		if cwe, ok := s.body.(interface{ CloseWithError(error) error }); ok {
			//nolint:errcheck // best-effort cleanup
			_ = cwe.CloseWithError(context.Canceled)
		}
		err = s.body.Close()
	})
	return err
}

func (s *ndjsonStream) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func parseNDJSONEvent(line []byte) (StreamEvent, error) {
	// Unified NDJSON format with record types (start, update, completion, error, tool_use_*)
	var envelope struct {
		// Unified format fields
		Type           string          `json:"type"`
		Payload        json.RawMessage `json:"payload,omitempty"`
		CompleteFields []string        `json:"complete_fields,omitempty"`
		Code           string          `json:"code,omitempty"`
		Message        string          `json:"message,omitempty"`
		Status         int             `json:"status,omitempty"`
		RequestID      string          `json:"request_id,omitempty"`
		Provider       string          `json:"provider,omitempty"`
		Model          string          `json:"model,omitempty"`
		StopReason     string          `json:"stop_reason,omitempty"`
		Usage          *Usage          `json:"usage,omitempty"`
		// Tool use fields
		ToolCallDelta *llm.ToolCallDelta `json:"tool_call_delta,omitempty"`
		ToolCalls     []llm.ToolCall     `json:"tool_calls,omitempty"`
		ToolCall      *llm.ToolCall      `json:"tool_call,omitempty"` // Single tool call (alternative to array)
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return StreamEvent{}, err
	}

	// Map unified record types to event kinds
	var kind llm.StreamEventKind
	switch envelope.Type {
	case "start":
		kind = llm.StreamEventKindMessageStart
	case "update":
		kind = llm.StreamEventKindMessageDelta
	case "completion":
		kind = llm.StreamEventKindMessageStop
	case "error":
		kind = llm.StreamEventKindCustom // Error records are handled specially
	case "keepalive":
		kind = llm.StreamEventKindPing
	case "tool_use_start":
		kind = llm.StreamEventKindToolUseStart
	case "tool_use_delta":
		kind = llm.StreamEventKindToolUseDelta
	case "tool_use_stop":
		kind = llm.StreamEventKindToolUseStop
	default:
		kind = llm.StreamEventKindCustom
	}

	// Extract content from payload if available
	var textDelta string
	if len(envelope.Payload) > 0 {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(envelope.Payload, &obj); err == nil {
			if rawContent, ok := obj["content"]; ok {
				if err := json.Unmarshal(rawContent, &textDelta); err != nil {
					return StreamEvent{}, fmt.Errorf("invalid stream payload content: %w", err)
				}
			}
		}
	}

	// Handle tool_calls array or single tool_call
	toolCalls := envelope.ToolCalls
	if envelope.ToolCall != nil && len(toolCalls) == 0 {
		toolCalls = []llm.ToolCall{*envelope.ToolCall}
	}

	event := StreamEvent{
		Kind:           kind,
		Name:           envelope.Type,
		Data:           append([]byte(nil), envelope.Payload...),
		ResponseID:     envelope.RequestID,
		Model:          NewModelID(envelope.Model),
		TextDelta:      textDelta,
		CompleteFields: envelope.CompleteFields,
		StopReason:     ParseStopReason(envelope.StopReason),
		Usage:          envelope.Usage,
		ToolCallDelta:  envelope.ToolCallDelta,
		ToolCalls:      toolCalls,
	}

	// Handle error records
	if envelope.Type == "error" {
		event.ErrorCode = envelope.Code
		event.ErrorMessage = envelope.Message
		event.ErrorStatus = envelope.Status
	}

	return event, nil
}

func newRequestContext(method, path string, model ModelID, requestID string) RequestContext {
	ctx := RequestContext{
		Method: method,
		Path:   path,
	}
	if !model.IsEmpty() {
		m := model
		ctx.Model = &m
	}
	if rid := strings.TrimSpace(requestID); rid != "" {
		ridCopy := rid
		ctx.RequestID = &ridCopy
	}
	return ctx
}

func requestIDFromHeaders(h http.Header) string {
	if h == nil {
		return ""
	}
	if id := h.Get(headers.RequestID); id != "" {
		return id
	}
	return ""
}

func isNDJSONContentType(value string) bool {
	if value == "" {
		return false
	}
	v := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(v, "application/x-ndjson") || strings.HasPrefix(v, "application/ndjson")
}
