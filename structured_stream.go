package sdk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
)

// StructuredRecordType identifies the NDJSON record kind in structured
// streaming responses from /llm/proxy.
type StructuredRecordType string

const (
	StructuredRecordTypeStart      StructuredRecordType = "start"
	StructuredRecordTypeUpdate     StructuredRecordType = "update"
	StructuredRecordTypeCompletion StructuredRecordType = "completion"
	StructuredRecordTypeError      StructuredRecordType = "error"
)

// StructuredJSONEvent is a typed view over structured streaming records. It
// only surfaces update and completion payloads; start and unknown records are
// ignored by the iterator.
type StructuredJSONEvent[T any] struct {
	Type      StructuredRecordType
	Payload   *T
	RequestID string
	// CompleteFields contains the set of field paths that are complete
	// (have their closing delimiters). Use dot notation for nested fields
	// (e.g., "metadata.author"). Check with CompleteFields["fieldName"].
	CompleteFields map[string]bool
}

// StructuredJSONStream drives decoding of NDJSON structured-output streams
// into caller-supplied types. It is created by LLMClient.ProxyStreamJSON.
type StructuredJSONStream[T any] struct {
	ctx       context.Context
	reader    *bufio.Reader
	body      io.ReadCloser
	requestID string
	retry     *RetryMetadata

	mu        sync.Mutex
	closed    bool
	closeOnce sync.Once
	done      chan struct{}
	terminal  bool // completion or error observed
}

func newStructuredJSONStream[T any](ctx context.Context, body io.ReadCloser, requestID string, retry *RetryMetadata) *StructuredJSONStream[T] {
	stream := &StructuredJSONStream[T]{
		ctx:       ctx,
		reader:    bufio.NewReader(body),
		body:      body,
		requestID: requestID,
		retry:     retry,
		done:      make(chan struct{}),
	}
	go func() {
		select {
		case <-ctx.Done():
			//nolint:errcheck // best-effort cleanup on context cancellation
			_ = stream.Close()
		case <-stream.done:
			return
		}
	}()
	return stream
}

// Next advances the stream and decodes the next update or completion record.
// It returns ok=false when the stream has ended. Any protocol violations
// (missing completion/error) are surfaced as TransportError.
func (s *StructuredJSONStream[T]) Next() (StructuredJSONEvent[T], bool, error) {
	if s.isClosed() {
		return StructuredJSONEvent[T]{}, false, nil
	}

	for {
		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			trimmed := bytes.TrimSpace(line)
			if errors.Is(err, io.EOF) && len(trimmed) == 0 {
				// End-of-stream: if no completion/error was seen, treat as
				// a protocol violation per the structured streaming contract.
				//nolint:errcheck // best-effort cleanup
				_ = s.Close()
				if s.terminal {
					return StructuredJSONEvent[T]{}, false, nil
				}
				return StructuredJSONEvent[T]{}, false, s.protocolError("structured stream ended without completion or error")
			}
			if len(trimmed) == 0 {
				return StructuredJSONEvent[T]{}, false, s.transportError("structured stream read failed", err)
			}
			// For io.EOF with a partial line, fall through to attempt
			// decoding the remaining bytes.
			line = trimmed
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		var raw struct {
			Type           string          `json:"type"`
			Payload        json.RawMessage `json:"payload,omitempty"`
			CompleteFields []string        `json:"complete_fields,omitempty"`
			Code           string          `json:"code,omitempty"`
			Message        string          `json:"message,omitempty"`
			Status         int             `json:"status,omitempty"`
		}
		if err := json.Unmarshal(line, &raw); err != nil {
			return StructuredJSONEvent[T]{}, false, s.transportError("invalid structured stream record", err)
		}
		recordType := StructuredRecordType(strings.TrimSpace(strings.ToLower(raw.Type)))
		switch recordType {
		case StructuredRecordTypeStart, "":
			// Ignore warm-up and malformed records; continue reading.
			continue
		case StructuredRecordTypeUpdate, StructuredRecordTypeCompletion:
			if len(bytes.TrimSpace(raw.Payload)) == 0 {
				return StructuredJSONEvent[T]{}, false, s.protocolError("structured stream record missing payload")
			}
			var payload T
			if err := json.Unmarshal(raw.Payload, &payload); err != nil {
				return StructuredJSONEvent[T]{}, false, s.transportError("failed to decode structured payload", err)
			}
			if recordType == StructuredRecordTypeCompletion {
				s.markTerminal()
				//nolint:errcheck // best-effort cleanup after completion
				_ = s.Close()
			}
			// Convert complete_fields array to map for O(1) lookups.
			// Skip empty strings and trim whitespace for robustness.
			completeFields := make(map[string]bool, len(raw.CompleteFields))
			for _, field := range raw.CompleteFields {
				field = strings.TrimSpace(field)
				if field != "" {
					completeFields[field] = true
				}
			}
			return StructuredJSONEvent[T]{
				Type:           recordType,
				Payload:        &payload,
				RequestID:      s.requestID,
				CompleteFields: completeFields,
			}, true, nil
		case StructuredRecordTypeError:
			s.markTerminal()
			//nolint:errcheck // best-effort cleanup after error
			_ = s.Close()
			status := raw.Status
			if status == 0 {
				status = http.StatusInternalServerError
			}
			msg := strings.TrimSpace(raw.Message)
			if msg == "" {
				msg = "structured stream error"
			}
			return StructuredJSONEvent[T]{}, false, APIError{
				Status:    status,
				Code:      raw.Code,
				Message:   msg,
				RequestID: s.requestID,
				Retry:     s.retry,
			}
		default:
			// Ignore unknown record types for forward compatibility.
			continue
		}
	}
}

// Collect drains the stream until a completion record is observed and returns
// the final payload. It closes the stream before returning.
func (s *StructuredJSONStream[T]) Collect(ctx context.Context) (T, error) {
	var zero T
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = s.Close() }()

	for {
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		default:
		}

		event, ok, err := s.Next()
		if err != nil {
			return zero, err
		}
		if !ok {
			return zero, s.protocolError("structured stream ended without completion or error")
		}
		if event.Type == StructuredRecordTypeCompletion && event.Payload != nil {
			return *event.Payload, nil
		}
	}
}

// RequestID returns the X-ModelRelay-Chat-Request-Id associated with this stream.
func (s *StructuredJSONStream[T]) RequestID() string {
	return s.requestID
}

// Close terminates the underlying HTTP response body and signals completion.
func (s *StructuredJSONStream[T]) Close() error {
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

func (s *StructuredJSONStream[T]) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *StructuredJSONStream[T]) markTerminal() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.terminal = true
}

func (s *StructuredJSONStream[T]) transportError(message string, cause error) error {
	return TransportError{
		Message: message,
		Cause:   cause,
		Retry:   s.retry,
	}
}

func (s *StructuredJSONStream[T]) protocolError(message string) error {
	return TransportError{
		Message: message,
		Cause:   nil,
		Retry:   s.retry,
	}
}
