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
	"time"
)

// StructuredRecordType identifies the NDJSON record kind in structured
// streaming responses from /responses.
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
	cancel    context.CancelFunc
	reader    *bufio.Reader
	body      io.ReadCloser
	requestID string
	retry     *RetryMetadata

	timeouts     StreamTimeouts
	activity     chan struct{}
	firstContent chan struct{} // closed on first update/completion/error record
	timeoutErrMu sync.Mutex
	timeoutErr   error

	mu        sync.Mutex
	closed    bool
	closeOnce sync.Once
	done      chan struct{}
	terminal  bool // completion or error observed
}

func newStructuredJSONStream[T any](ctx context.Context, body io.ReadCloser, requestID string, retry *RetryMetadata, timeouts StreamTimeouts) *StructuredJSONStream[T] {
	streamCtx, cancel := context.WithCancel(ctx)
	stream := &StructuredJSONStream[T]{
		ctx:       streamCtx,
		cancel:    cancel,
		reader:    bufio.NewReader(body),
		body:      body,
		requestID: requestID,
		retry:     retry,
		done:      make(chan struct{}),
		timeouts:  timeouts,
		activity:  make(chan struct{}, 1),
		// firstContent is only used when TTFT is configured.
		firstContent: make(chan struct{}),
	}
	go func() {
		select {
		case <-streamCtx.Done():
			//nolint:errcheck // best-effort cleanup on context cancellation
			_ = stream.Close()
		case <-stream.done:
			return
		}
	}()
	stream.startTimeoutMonitor()
	return stream
}

func (s *StructuredJSONStream[T]) startTimeoutMonitor() {
	if s.timeouts.TTFT <= 0 && s.timeouts.Idle <= 0 && s.timeouts.Total <= 0 {
		return
	}

	go func() {
		var ttftTimer *time.Timer
		var ttftC <-chan time.Time
		if s.timeouts.TTFT > 0 {
			ttftTimer = time.NewTimer(s.timeouts.TTFT)
			ttftC = ttftTimer.C
		}

		var idleTimer *time.Timer
		var idleC <-chan time.Time
		if s.timeouts.Idle > 0 {
			idleTimer = time.NewTimer(s.timeouts.Idle)
			idleC = idleTimer.C
		}

		var totalTimer *time.Timer
		var totalC <-chan time.Time
		if s.timeouts.Total > 0 {
			totalTimer = time.NewTimer(s.timeouts.Total)
			totalC = totalTimer.C
		}

		firstCh := s.firstContent

		defer func() {
			if ttftTimer != nil {
				ttftTimer.Stop()
			}
			if idleTimer != nil {
				idleTimer.Stop()
			}
			if totalTimer != nil {
				totalTimer.Stop()
			}
		}()

		for {
			select {
			case <-s.done:
				return
			case <-s.ctx.Done():
				return
			case <-firstCh:
				firstCh = nil
				if ttftTimer != nil {
					ttftTimer.Stop()
					ttftC = nil
				}
			case <-s.activity:
				if idleTimer != nil {
					if !idleTimer.Stop() {
						select {
						case <-idleTimer.C:
						default:
						}
					}
					idleTimer.Reset(s.timeouts.Idle)
					idleC = idleTimer.C
				}
			case <-ttftC:
				s.setTimeoutErr(StreamTimeoutError{Kind: StreamTimeoutTTFT, Timeout: s.timeouts.TTFT})
				s.cancel()
				return
			case <-idleC:
				s.setTimeoutErr(StreamTimeoutError{Kind: StreamTimeoutIdle, Timeout: s.timeouts.Idle})
				s.cancel()
				return
			case <-totalC:
				s.setTimeoutErr(StreamTimeoutError{Kind: StreamTimeoutTotal, Timeout: s.timeouts.Total})
				s.cancel()
				return
			}
		}
	}()
}

func (s *StructuredJSONStream[T]) setTimeoutErr(err error) {
	s.timeoutErrMu.Lock()
	defer s.timeoutErrMu.Unlock()
	if s.timeoutErr == nil {
		s.timeoutErr = err
	}
}

func (s *StructuredJSONStream[T]) getTimeoutErr() error {
	s.timeoutErrMu.Lock()
	defer s.timeoutErrMu.Unlock()
	return s.timeoutErr
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
			if terr := s.getTimeoutErr(); terr != nil && s.ctx.Err() != nil {
				//nolint:errcheck // best-effort cleanup after timeout
				_ = s.Close()
				return StructuredJSONEvent[T]{}, false, terr
			}
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
		select {
		case s.activity <- struct{}{}:
		default:
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
		case StructuredRecordTypeStart:
			// Ignore warm-up records; continue reading.
			continue
		case "":
			return StructuredJSONEvent[T]{}, false, s.protocolError("structured stream record missing type")
		case "keepalive":
			// Keepalive records are not part of the structured-output contract.
			// They are used to keep long-lived connections from timing out.
			continue
		case StructuredRecordTypeUpdate, StructuredRecordTypeCompletion:
			if len(bytes.TrimSpace(raw.Payload)) == 0 {
				return StructuredJSONEvent[T]{}, false, s.protocolError("structured stream record missing payload")
			}
			var payload T
			if err := json.Unmarshal(raw.Payload, &payload); err != nil {
				return StructuredJSONEvent[T]{}, false, s.transportError("failed to decode structured payload", err)
			}
			select {
			case <-s.firstContent:
			default:
				close(s.firstContent)
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
			select {
			case <-s.firstContent:
			default:
				close(s.firstContent)
			}
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

// RequestID returns the X-ModelRelay-Request-Id associated with this stream.
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
		if s.cancel != nil {
			s.cancel()
		}
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
