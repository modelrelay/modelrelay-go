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

	"github.com/modelrelay/modelrelay/platform/headers"
	llm "github.com/modelrelay/modelrelay/providers"
)

// LLMClient proxies chat completions through the SaaS API.
type LLMClient struct {
	client *Client
}

// ProxyMessage performs a blocking completion and returns the aggregated response.
func (c *LLMClient) ProxyMessage(ctx context.Context, req ProxyRequest, options ...ProxyOption) (*ProxyResponse, error) {
	callOpts := buildProxyCallOptions(options)
	if callOpts.retry == nil {
		cfg := c.client.retryCfg
		cfg.RetryPost = true
		callOpts.retry = &cfg
	}
	req.Metadata = mergeMetadataMaps(c.client.defaultMetadata, req.Metadata, callOpts.metadata)
	reqPayload, err := newProxyRequestPayload(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.client.newJSONRequest(ctx, http.MethodPost, "/llm/proxy", reqPayload)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json")
	applyProxyHeaders(httpReq, callOpts)
	resp, retryMeta, err := c.client.send(httpReq, callOpts.timeout, callOpts.retry)
	if err != nil {
		c.client.telemetry.log(ctx, LogLevelError, "proxy_message_failed", map[string]any{"error": err.Error(), "retries": retryMeta})
		return nil, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	var respPayload ProxyResponse
	if err := json.NewDecoder(resp.Body).Decode(&respPayload); err != nil {
		return nil, err
	}
	respPayload.RequestID = requestIDFromHeaders(resp.Header)
	return &respPayload, nil
}

// ProxyCustomerMessage performs a blocking completion for customer-attributed requests.
// Unlike ProxyMessage, this does not require a model because the customer's tier determines it.
// The customerID is sent via the X-ModelRelay-Customer-Id header.
func (c *LLMClient) ProxyCustomerMessage(ctx context.Context, customerID string, req ProxyRequest, options ...ProxyOption) (*ProxyResponse, error) {
	if customerID == "" {
		return nil, fmt.Errorf("customer ID is required")
	}
	// Prepend customer ID option so it's applied first
	options = append([]ProxyOption{WithCustomerID(customerID)}, options...)
	callOpts := buildProxyCallOptions(options)
	if callOpts.retry == nil {
		cfg := c.client.retryCfg
		cfg.RetryPost = true
		callOpts.retry = &cfg
	}
	req.Metadata = mergeMetadataMaps(c.client.defaultMetadata, req.Metadata, callOpts.metadata)
	// Validate without requiring model
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("at least one message is required")
	}
	payload := newCustomerProxyRequestPayload(req)
	httpReq, err := c.client.newJSONRequest(ctx, http.MethodPost, "/llm/proxy", payload)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json")
	applyProxyHeaders(httpReq, callOpts)
	resp, retryMeta, err := c.client.send(httpReq, callOpts.timeout, callOpts.retry)
	if err != nil {
		c.client.telemetry.log(ctx, LogLevelError, "proxy_customer_message_failed", map[string]any{"error": err.Error(), "retries": retryMeta})
		return nil, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	var respPayload ProxyResponse
	if err := json.NewDecoder(resp.Body).Decode(&respPayload); err != nil {
		return nil, err
	}
	respPayload.RequestID = requestIDFromHeaders(resp.Header)
	return &respPayload, nil
}

// ProxyCustomerStream opens a streaming connection for customer-attributed chat completions.
// Unlike ProxyStream, this does not require a model because the customer's tier determines it.
// The customerID is sent via the X-ModelRelay-Customer-Id header.
func (c *LLMClient) ProxyCustomerStream(ctx context.Context, customerID string, req ProxyRequest, options ...ProxyOption) (*StreamHandle, error) {
	if customerID == "" {
		return nil, fmt.Errorf("customer ID is required")
	}
	// Prepend customer ID option so it's applied first
	options = append([]ProxyOption{WithCustomerID(customerID)}, options...)
	callOpts := buildProxyCallOptions(options)
	if callOpts.retry == nil {
		cfg := c.client.retryCfg
		cfg.RetryPost = true
		callOpts.retry = &cfg
	}
	req.Metadata = mergeMetadataMaps(c.client.defaultMetadata, req.Metadata, callOpts.metadata)
	// Validate without requiring model
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("at least one message is required")
	}
	payload := newCustomerProxyRequestPayload(req)
	httpReq, err := c.client.newJSONRequest(ctx, http.MethodPost, "/llm/proxy", payload)
	if err != nil {
		return nil, err
	}
	if callOpts.stream == StreamFormatNDJSON {
		httpReq.Header.Set("Accept", "application/x-ndjson")
	} else {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	applyProxyHeaders(httpReq, callOpts)
	//nolint:bodyclose // resp.Body is transferred to stream and will be closed by stream.Close()
	resp, _, err := c.client.send(httpReq, callOpts.timeout, callOpts.retry)
	if err != nil {
		return nil, err
	}
	var stream streamReader
	if callOpts.stream == StreamFormatNDJSON {
		stream = newNDJSONStream(ctx, resp.Body, c.client.telemetry)
	} else {
		stream = newSSEStream(ctx, resp.Body, c.client.telemetry)
	}
	return &StreamHandle{
		stream:    stream,
		RequestID: requestIDFromHeaders(resp.Header),
	}, nil
}

// ProxyStream opens a streaming connection for chat completions.
func (c *LLMClient) ProxyStream(ctx context.Context, req ProxyRequest, options ...ProxyOption) (*StreamHandle, error) {
	callOpts := buildProxyCallOptions(options)
	if callOpts.retry == nil {
		cfg := c.client.retryCfg
		cfg.RetryPost = true
		callOpts.retry = &cfg
	}
	req.Metadata = mergeMetadataMaps(c.client.defaultMetadata, req.Metadata, callOpts.metadata)
	payload, err := newProxyRequestPayload(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.client.newJSONRequest(ctx, http.MethodPost, "/llm/proxy", payload)
	if err != nil {
		return nil, err
	}
	if callOpts.stream == StreamFormatNDJSON {
		httpReq.Header.Set("Accept", "application/x-ndjson")
	} else {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	applyProxyHeaders(httpReq, callOpts)
	//nolint:bodyclose // resp.Body is transferred to stream and will be closed by stream.Close()
	resp, _, err := c.client.send(httpReq, callOpts.timeout, callOpts.retry)
	if err != nil {
		return nil, err
	}
	var stream streamReader
	if callOpts.stream == StreamFormatNDJSON {
		stream = newNDJSONStream(ctx, resp.Body, c.client.telemetry)
	} else {
		stream = newSSEStream(ctx, resp.Body, c.client.telemetry)
	}
	return &StreamHandle{
		stream:    stream,
		RequestID: requestIDFromHeaders(resp.Header),
	}, nil
}

// ProxyStreamJSON streams structured JSON responses for requests that use
// response_format with structured outputs (json_object or json_schema). It
// negotiates NDJSON per the /llm/proxy structured streaming contract and
// decodes each update/completion payload into T. The caller is responsible
// for driving the stream via Next or Collect.
func ProxyStreamJSON[T any](ctx context.Context, c *LLMClient, req ProxyRequest, options ...ProxyOption) (*StructuredJSONStream[T], error) {
	if req.ResponseFormat == nil || !req.ResponseFormat.IsStructured() {
		return nil, ConfigError{Reason: "response_format with type=json_object or json_schema is required for structured streaming"}
	}

	callOpts := buildProxyCallOptions(options)
	if callOpts.retry == nil {
		cfg := c.client.retryCfg
		cfg.RetryPost = true
		callOpts.retry = &cfg
	}
	req.Metadata = mergeMetadataMaps(c.client.defaultMetadata, req.Metadata, callOpts.metadata)
	payload, err := newProxyRequestPayload(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.client.newJSONRequest(ctx, http.MethodPost, "/llm/proxy", payload)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/x-ndjson")
	applyProxyHeaders(httpReq, callOpts)

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

type proxyRequestPayload struct {
	Model          string              `json:"model"`
	MaxTokens      int64               `json:"max_tokens"`
	Temperature    *float64            `json:"temperature,omitempty"`
	Messages       []llm.ProxyMessage  `json:"messages"`
	Metadata       map[string]string   `json:"metadata,omitempty"`
	Stop           []string            `json:"stop,omitempty"`
	StopSeqs       []string            `json:"stop_sequences,omitempty"`
	ResponseFormat *llm.ResponseFormat `json:"response_format,omitempty"`
	Tools          []llm.Tool          `json:"tools,omitempty"`
	ToolChoice     *llm.ToolChoice     `json:"tool_choice,omitempty"`
}

func newProxyRequestPayload(req ProxyRequest) (proxyRequestPayload, error) {
	if err := req.Validate(); err != nil {
		return proxyRequestPayload{}, err
	}

	payload := proxyRequestPayload{
		Model:          req.Model.String(),
		MaxTokens:      req.MaxTokens,
		Temperature:    req.Temperature,
		Messages:       req.Messages,
		ResponseFormat: req.ResponseFormat,
	}
	if len(req.Metadata) > 0 {
		payload.Metadata = req.Metadata
	}
	if len(req.Stop) > 0 {
		payload.Stop = req.Stop
	}
	if len(req.StopSequences) > 0 {
		payload.StopSeqs = req.StopSequences
	}
	if len(req.Tools) > 0 {
		payload.Tools = req.Tools
	}
	if req.ToolChoice != nil {
		payload.ToolChoice = req.ToolChoice
	}
	return payload, nil
}

// newCustomerProxyRequestPayload builds a payload for customer-attributed requests.
// Unlike newProxyRequestPayload, this does NOT call Validate() because model validation
// would fail - the customer's tier determines the model on the server side.
// Callers must validate required fields (e.g., messages) before calling this function.
func newCustomerProxyRequestPayload(req ProxyRequest) proxyRequestPayload {
	payload := proxyRequestPayload{
		// Model is intentionally omitted - tier determines it
		MaxTokens:      req.MaxTokens,
		Temperature:    req.Temperature,
		Messages:       req.Messages,
		ResponseFormat: req.ResponseFormat,
	}
	if len(req.Metadata) > 0 {
		payload.Metadata = req.Metadata
	}
	if len(req.Stop) > 0 {
		payload.Stop = req.Stop
	}
	if len(req.StopSequences) > 0 {
		payload.StopSeqs = req.StopSequences
	}
	if len(req.Tools) > 0 {
		payload.Tools = req.Tools
	}
	if req.ToolChoice != nil {
		payload.ToolChoice = req.ToolChoice
	}
	return payload
}

func mergeMetadataMaps(defaults, req, overrides map[string]string) map[string]string {
	merged := make(map[string]string)
	addMetadata(merged, defaults)
	addMetadata(merged, req)
	addMetadata(merged, overrides)
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func addMetadata(dst map[string]string, src map[string]string) {
	for key, value := range src {
		k := strings.TrimSpace(key)
		v := strings.TrimSpace(value)
		if k == "" || v == "" {
			continue
		}
		dst[k] = v
	}
}

type sseStream struct {
	ctx       context.Context
	reader    *bufio.Reader
	body      io.ReadCloser
	telemetry TelemetryHooks
	closed    bool
	closeOnce sync.Once
	mu        sync.Mutex
	done      chan struct{}
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
}

func newSSEStream(ctx context.Context, body io.ReadCloser, telemetry TelemetryHooks) *sseStream {
	stream := &sseStream{
		ctx:       ctx,
		reader:    bufio.NewReader(body),
		body:      body,
		telemetry: telemetry,
		done:      make(chan struct{}),
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

func (s *sseStream) Next() (StreamEvent, bool, error) {
	if s.isClosed() {
		return StreamEvent{}, false, nil
	}
	for {
		eventName, data, err := s.readEvent()
		if err != nil {
			if errors.Is(err, io.EOF) {
				//nolint:errcheck // best-effort cleanup on EOF
				_ = s.Close()
				return StreamEvent{}, false, nil
			}
			return StreamEvent{}, false, err
		}
		if eventName == "" && len(data) == 0 {
			continue
		}
		event := buildStreamEvent(eventName, data)
		if s.telemetry.OnStreamEvent != nil {
			s.telemetry.OnStreamEvent(s.ctx, event)
		}
		s.telemetry.metric(s.ctx, "sdk_stream_events_total", 1, map[string]string{"event": event.EventName()})
		return event, true, nil
	}
}

func (s *sseStream) Close() error {
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

func (s *sseStream) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *sseStream) readEvent() (string, []byte, error) {
	var eventName string
	var dataBuilder strings.Builder
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && line == "" {
				return "", nil, io.EOF
			}
			if errors.Is(err, io.EOF) {
				line = strings.TrimRight(line, "\r\n")
				if line == "" {
					return eventName, []byte(dataBuilder.String()), nil
				}
			}
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			return eventName, []byte(dataBuilder.String()), nil
		}
		switch {
		case strings.HasPrefix(line, ":"):
			continue
		case strings.HasPrefix(line, "event:"):
			eventName = strings.TrimSpace(line[len("event:"):])
		case strings.HasPrefix(line, "data:"):
			if dataBuilder.Len() > 0 {
				dataBuilder.WriteByte('\n')
			}
			dataBuilder.WriteString(strings.TrimSpace(line[len("data:"):]))
		}
	}
}

func newNDJSONStream(ctx context.Context, body io.ReadCloser, telemetry TelemetryHooks) *ndjsonStream {
	stream := &ndjsonStream{
		ctx:       ctx,
		reader:    bufio.NewReader(body),
		body:      body,
		telemetry: telemetry,
		done:      make(chan struct{}),
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
	var envelope struct {
		Event      string          `json:"event"`
		Data       json.RawMessage `json:"data"`
		ResponseID string          `json:"response_id"`
		Model      string          `json:"model"`
		StopReason string          `json:"stop_reason"`
		Usage      *Usage          `json:"usage"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return StreamEvent{}, err
	}
	event := StreamEvent{
		Kind:       streamEventKind(envelope.Event),
		Name:       envelope.Event,
		Data:       append([]byte(nil), envelope.Data...),
		ResponseID: envelope.ResponseID,
		Model:      NewModelID(envelope.Model),
		StopReason: ParseStopReason(envelope.StopReason),
		Usage:      envelope.Usage,
	}
	return event, nil
}

func buildStreamEvent(name string, data []byte) StreamEvent {
	event := StreamEvent{
		Kind: streamEventKind(name),
		Name: name,
		Data: append([]byte(nil), data...),
	}
	var meta struct {
		ResponseID string `json:"response_id"`
		Model      string `json:"model"`
		StopReason string `json:"stop_reason"`
		Usage      *Usage `json:"usage"`
	}
	if len(data) > 0 {
		//nolint:errcheck // best-effort metadata extraction
		_ = json.Unmarshal(data, &meta)
	}
	event.ResponseID = meta.ResponseID
	event.Model = NewModelID(meta.Model)
	event.StopReason = ParseStopReason(meta.StopReason)
	event.Usage = meta.Usage
	return event
}

func streamEventKind(name string) llm.StreamEventKind {
	switch name {
	case string(llm.StreamEventKindMessageStart):
		return llm.StreamEventKindMessageStart
	case string(llm.StreamEventKindMessageDelta):
		return llm.StreamEventKindMessageDelta
	case string(llm.StreamEventKindMessageStop):
		return llm.StreamEventKindMessageStop
	case string(llm.StreamEventKindPing):
		return llm.StreamEventKindPing
	case string(llm.StreamEventKindCustom):
		return llm.StreamEventKindCustom
	default:
		return llm.StreamEventKindCustom
	}
}

func requestIDFromHeaders(h http.Header) string {
	if h == nil {
		return ""
	}
	if id := h.Get(headers.ChatRequestID); id != "" {
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
