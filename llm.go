package sdk

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	llm "github.com/modelrelay/modelrelay/llmproxy"
)

// LLMClient proxies chat completions through the SaaS API.
type LLMClient struct {
	client *Client
}

// ProxyMessage performs a blocking completion and returns the aggregated response.
func (c *LLMClient) ProxyMessage(ctx context.Context, req ProxyRequest, options ...ProxyOption) (*ProxyResponse, error) {
	reqPayload, err := newProxyRequestPayload(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.client.newJSONRequest(ctx, http.MethodPost, "/llm/proxy", reqPayload)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "application/json")
	applyProxyOptions(httpReq, options)
	resp, err := c.client.send(httpReq)
	if err != nil {
		c.client.telemetry.log(ctx, LogLevelError, "proxy_message_failed", map[string]any{"error": err.Error()})
		return nil, err
	}
	defer resp.Body.Close()
	var respPayload ProxyResponse
	if err := json.NewDecoder(resp.Body).Decode(&respPayload); err != nil {
		return nil, err
	}
	respPayload.RequestID = requestIDFromHeaders(resp.Header)
	return &respPayload, nil
}

// ProxyStream opens a streaming SSE connection for chat completions.
func (c *LLMClient) ProxyStream(ctx context.Context, req ProxyRequest, options ...ProxyOption) (*StreamHandle, error) {
	payload, err := newProxyRequestPayload(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.client.newJSONRequest(ctx, http.MethodPost, "/llm/proxy", payload)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	applyProxyOptions(httpReq, options)
	resp, err := c.client.send(httpReq)
	if err != nil {
		return nil, err
	}
	return &StreamHandle{
		stream:    newSSEStream(ctx, resp.Body, c.client.telemetry),
		RequestID: requestIDFromHeaders(resp.Header),
	}, nil
}

type proxyRequestPayload struct {
	Provider    string             `json:"provider,omitempty"`
	Model       string             `json:"model"`
	MaxTokens   int64              `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	Messages    []llm.ProxyMessage `json:"messages"`
	Metadata    map[string]string  `json:"metadata,omitempty"`
	Stop        []string           `json:"stop,omitempty"`
	StopSeqs    []string           `json:"stop_sequences,omitempty"`
}

func newProxyRequestPayload(req ProxyRequest) (proxyRequestPayload, error) {
	if err := req.Validate(); err != nil {
		return proxyRequestPayload{}, err
	}

	payload := proxyRequestPayload{
		Provider:    req.Provider.String(),
		Model:       req.Model.String(),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Messages:    req.Messages,
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
	return payload, nil
}

type sseStream struct {
	ctx       context.Context
	reader    *bufio.Reader
	body      io.ReadCloser
	telemetry TelemetryHooks
	closed    bool
}

func newSSEStream(ctx context.Context, body io.ReadCloser, telemetry TelemetryHooks) *sseStream {
	return &sseStream{
		ctx:       ctx,
		reader:    bufio.NewReader(body),
		body:      body,
		telemetry: telemetry,
	}
}

func (s *sseStream) Next() (StreamEvent, bool, error) {
	if s.closed {
		return StreamEvent{}, false, nil
	}
	for {
		eventName, data, err := s.readEvent()
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.Close()
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
	if s.closed {
		return nil
	}
	s.closed = true
	return s.body.Close()
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
		_ = json.Unmarshal(data, &meta)
	}
	event.ResponseID = meta.ResponseID
	event.Model = ParseModelID(meta.Model)
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
	if id := h.Get(requestIDHeader); id != "" {
		return id
	}
	return ""
}
