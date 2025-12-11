package sdk

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// MockClient provides an in-memory client for unit tests without hitting the API.
// It currently supports the LLM surface area (ProxyMessage/ProxyStream).
type MockClient struct {
	LLM *MockLLMClient
}

// MockClientError is returned when a mock client is used without configuration.
type MockClientError struct {
	Reason string
}

func (e MockClientError) Error() string { return "mock client: " + e.Reason }

type mockProxyResult struct {
	resp ProxyResponse
	err  error
}

type mockStreamResult struct {
	events []StreamEvent
	err    error
}

// MockLLMClient implements LLMClient methods using preconfigured responses.
type MockLLMClient struct {
	mu          sync.Mutex
	proxyQueue  []mockProxyResult
	streamQueue []mockStreamResult
}

// NewMockClient creates an empty mock client.
func NewMockClient() *MockClient {
	llmClient := &MockLLMClient{}
	return &MockClient{LLM: llmClient}
}

// WithProxyResponse enqueues a blocking ProxyResponse for the next ProxyMessage call.
func (c *MockClient) WithProxyResponse(resp ProxyResponse) *MockClient {
	c.LLM.enqueueProxy(resp, nil)
	return c
}

// WithProxyError enqueues an error for the next ProxyMessage call.
func (c *MockClient) WithProxyError(err error) *MockClient {
	c.LLM.enqueueProxy(ProxyResponse{}, err)
	return c
}

// WithStreamEvents enqueues a stream of events for the next ProxyStream call.
func (c *MockClient) WithStreamEvents(events []StreamEvent) *MockClient {
	c.LLM.enqueueStream(events, nil)
	return c
}

// WithStreamError enqueues an error for the next ProxyStream call.
func (c *MockClient) WithStreamError(err error) *MockClient {
	c.LLM.enqueueStream(nil, err)
	return c
}

func (c *MockLLMClient) enqueueProxy(resp ProxyResponse, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.proxyQueue = append(c.proxyQueue, mockProxyResult{resp: resp, err: err})
}

func (c *MockLLMClient) enqueueStream(events []StreamEvent, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	copied := append([]StreamEvent(nil), events...)
	c.streamQueue = append(c.streamQueue, mockStreamResult{events: copied, err: err})
}

func (c *MockLLMClient) dequeueProxy() (mockProxyResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.proxyQueue) == 0 {
		return mockProxyResult{}, MockClientError{Reason: "no proxy responses configured"}
	}
	res := c.proxyQueue[0]
	c.proxyQueue = c.proxyQueue[1:]
	return res, nil
}

func (c *MockLLMClient) dequeueStream() (mockStreamResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.streamQueue) == 0 {
		return mockStreamResult{}, MockClientError{Reason: "no stream events configured"}
	}
	res := c.streamQueue[0]
	c.streamQueue = c.streamQueue[1:]
	return res, nil
}

// ProxyMessage returns the next queued ProxyResponse or error.
func (c *MockLLMClient) ProxyMessage(ctx context.Context, req ProxyRequest, _ ...ProxyOption) (*ProxyResponse, error) {
	if err := req.Validate(); err != nil {
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

// ProxyCustomerMessage behaves like ProxyMessage but does not require a model.
func (c *MockLLMClient) ProxyCustomerMessage(ctx context.Context, customerID string, req ProxyRequest, _ ...ProxyOption) (*ProxyResponse, error) {
	if strings.TrimSpace(customerID) == "" {
		return nil, fmt.Errorf("customer ID is required")
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("at least one message is required")
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

// ProxyStream returns a StreamHandle that yields the next queued events.
func (c *MockLLMClient) ProxyStream(ctx context.Context, req ProxyRequest, _ ...ProxyOption) (*StreamHandle, error) {
	if err := req.Validate(); err != nil {
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

// ProxyCustomerStream behaves like ProxyStream but does not require a model.
func (c *MockLLMClient) ProxyCustomerStream(ctx context.Context, customerID string, req ProxyRequest, _ ...ProxyOption) (*StreamHandle, error) {
	if strings.TrimSpace(customerID) == "" {
		return nil, fmt.Errorf("customer ID is required")
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("at least one message is required")
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
