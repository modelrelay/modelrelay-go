package sdk

import (
	"context"
	"strings"
	"sync"

	"github.com/modelrelay/modelrelay/platform/headers"
)

// MockClient provides an in-memory client for unit tests without hitting the API.
// It currently supports the Responses surface area (Create/Stream).
type MockClient struct {
	Responses *MockResponsesClient
}

// MockClientError is returned when a mock client is used without configuration.
type MockClientError struct {
	Reason string
}

func (e MockClientError) Error() string { return "mock client: " + e.Reason }

type mockProxyResult struct {
	resp Response
	err  error
}

type mockStreamResult struct {
	events []StreamEvent
	err    error
}

// MockResponsesClient implements ResponsesClient methods using preconfigured responses.
type MockResponsesClient struct {
	mu          sync.Mutex
	proxyQueue  []mockProxyResult
	streamQueue []mockStreamResult
}

// NewMockClient creates an empty mock client.
func NewMockClient() *MockClient {
	responses := &MockResponsesClient{}
	return &MockClient{Responses: responses}
}

// WithResponse enqueues a blocking Response for the next Create call.
func (c *MockClient) WithResponse(resp Response) *MockClient {
	c.Responses.enqueueProxy(resp, nil)
	return c
}

// WithResponseError enqueues an error for the next Create call.
func (c *MockClient) WithResponseError(err error) *MockClient {
	c.Responses.enqueueProxy(Response{}, err)
	return c
}

// WithStreamEvents enqueues a stream of events for the next Stream call.
func (c *MockClient) WithStreamEvents(events []StreamEvent) *MockClient {
	c.Responses.enqueueStream(events, nil)
	return c
}

// WithStreamError enqueues an error for the next Stream call.
func (c *MockClient) WithStreamError(err error) *MockClient {
	c.Responses.enqueueStream(nil, err)
	return c
}

func (c *MockResponsesClient) enqueueProxy(resp Response, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.proxyQueue = append(c.proxyQueue, mockProxyResult{resp: resp, err: err})
}

func (c *MockResponsesClient) enqueueStream(events []StreamEvent, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	copied := append([]StreamEvent(nil), events...)
	c.streamQueue = append(c.streamQueue, mockStreamResult{events: copied, err: err})
}

func (c *MockResponsesClient) dequeueProxy() (mockProxyResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.proxyQueue) == 0 {
		return mockProxyResult{}, MockClientError{Reason: "no proxy responses configured"}
	}
	res := c.proxyQueue[0]
	c.proxyQueue = c.proxyQueue[1:]
	return res, nil
}

func (c *MockResponsesClient) dequeueStream() (mockStreamResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.streamQueue) == 0 {
		return mockStreamResult{}, MockClientError{Reason: "no stream events configured"}
	}
	res := c.streamQueue[0]
	c.streamQueue = c.streamQueue[1:]
	return res, nil
}

// Create returns the next queued Response or error.
func (c *MockResponsesClient) Create(ctx context.Context, req ResponseRequest, opts ...ResponseOption) (*Response, error) {
	callOpts := buildResponseCallOptions(opts)
	requireModel := strings.TrimSpace(callOpts.headers.Get(headers.CustomerID)) == ""
	if err := req.validate(requireModel); err != nil {
		return nil, err
	}
	res, err := c.dequeueProxy()
	if err != nil {
		return nil, err
	}
	if res.err != nil {
		return nil, res.err
	}
	respCopy := res.resp
	return &respCopy, nil
}

// Stream returns a StreamHandle that yields the next queued events.
func (c *MockResponsesClient) Stream(ctx context.Context, req ResponseRequest, opts ...ResponseOption) (*StreamHandle, error) {
	callOpts := buildResponseCallOptions(opts)
	requireModel := strings.TrimSpace(callOpts.headers.Get(headers.CustomerID)) == ""
	if err := req.validate(requireModel); err != nil {
		return nil, err
	}
	res, err := c.dequeueStream()
	if err != nil {
		return nil, err
	}
	if res.err != nil {
		return nil, res.err
	}
	return &StreamHandle{stream: &mockStreamReader{events: res.events}}, nil
}

type mockStreamReader struct {
	events []StreamEvent
	idx    int
	closed bool
}

func (m *mockStreamReader) Next() (StreamEvent, bool, error) {
	if m.closed || m.idx >= len(m.events) {
		return StreamEvent{}, false, nil
	}
	ev := m.events[m.idx]
	m.idx++
	return ev, true, nil
}

func (m *mockStreamReader) Close() error {
	m.closed = true
	return nil
}
