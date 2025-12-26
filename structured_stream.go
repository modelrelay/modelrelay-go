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

	jsonpatch "github.com/evanphx/json-patch/v5"
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
	Usage     *Usage
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

	monitor *streamTimeoutMonitor

	mu        sync.Mutex
	closed    bool
	closeOnce sync.Once
	done      chan struct{}
	terminal  bool // completion or error observed

	currentPayload json.RawMessage
}

func newStructuredJSONStream[T any](ctx context.Context, body io.ReadCloser, requestID string, retry *RetryMetadata, timeouts StreamTimeouts) *StructuredJSONStream[T] {
	streamCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	stream := &StructuredJSONStream[T]{
		ctx:       streamCtx,
		cancel:    cancel,
		reader:    bufio.NewReader(body),
		body:      body,
		requestID: requestID,
		retry:     retry,
		done:      done,
		monitor:   newStreamTimeoutMonitor(streamCtx, timeouts, done, cancel),
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
	stream.monitor.Start()
	return stream
}

// structuredRecord holds the parsed fields from a structured NDJSON record.
// This is a pure data structure used for intermediate parsing.
type structuredRecord struct {
	recordType     StructuredRecordType
	payload        json.RawMessage
	patch          json.RawMessage
	completeFields []string
	code           string
	message        string
	status         int
	usage          *Usage
}

// parseStructuredRecord parses a single NDJSON line into a typed record.
// This is a pure function with no side effects.
func parseStructuredRecord(line []byte) (structuredRecord, error) {
	var raw struct {
		Type           string          `json:"type"`
		Patch          json.RawMessage `json:"patch,omitempty"`
		Payload        json.RawMessage `json:"payload,omitempty"`
		CompleteFields []string        `json:"complete_fields,omitempty"`
		Code           string          `json:"code,omitempty"`
		Message        string          `json:"message,omitempty"`
		Status         int             `json:"status,omitempty"`
		Usage          *Usage          `json:"usage,omitempty"`
	}
	if err := json.Unmarshal(line, &raw); err != nil {
		return structuredRecord{}, err
	}
	return structuredRecord{
		recordType:     StructuredRecordType(strings.TrimSpace(strings.ToLower(raw.Type))),
		patch:          raw.Patch,
		payload:        raw.Payload,
		completeFields: raw.CompleteFields,
		code:           raw.Code,
		message:        raw.Message,
		status:         raw.Status,
		usage:          raw.Usage,
	}, nil
}

// buildCompleteFieldsMap converts a complete_fields array to a map for O(1) lookups.
// This is a pure function with no side effects.
func buildCompleteFieldsMap(fields []string) map[string]bool {
	result := make(map[string]bool, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			result[field] = true
		}
	}
	return result
}

func (s *StructuredJSONStream[T]) materializePayload(record structuredRecord) (json.RawMessage, error) {
	switch record.recordType {
	case StructuredRecordTypeUpdate:
		if len(bytes.TrimSpace(record.patch)) == 0 {
			return nil, s.protocolError("structured stream update missing patch")
		}
		next, err := s.applyPatch(record.patch)
		if err != nil {
			return nil, s.transportError("failed to apply structured patch", err)
		}
		return next, nil
	case StructuredRecordTypeCompletion:
		if len(bytes.TrimSpace(record.payload)) == 0 {
			return nil, s.protocolError("structured stream completion missing payload")
		}
		s.currentPayload = append([]byte(nil), record.payload...)
		return s.currentPayload, nil
	default:
		return nil, s.protocolError("structured stream record missing payload")
	}
}

func (s *StructuredJSONStream[T]) applyPatch(patch json.RawMessage) (json.RawMessage, error) {
	base := s.currentPayload
	if len(bytes.TrimSpace(base)) == 0 {
		base = []byte("{}")
	}
	ops, err := jsonpatch.DecodePatch(patch)
	if err != nil {
		return nil, err
	}
	next, err := ops.Apply(base)
	if err != nil {
		return nil, err
	}
	s.currentPayload = next
	return next, nil
}

// Next advances the stream and decodes the next update or completion record.
// It returns ok=false when the stream has ended. Any protocol violations
// (missing completion/error) are surfaced as TransportError.
func (s *StructuredJSONStream[T]) Next() (StructuredJSONEvent[T], bool, error) {
	if s.isClosed() {
		if terr := s.monitor.GetTimeoutErr(); terr != nil {
			return StructuredJSONEvent[T]{}, false, terr
		}
		return StructuredJSONEvent[T]{}, false, nil
	}

	for {
		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			if terr := s.monitor.GetTimeoutErr(); terr != nil && s.ctx.Err() != nil {
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
		s.monitor.SignalActivity()

		record, err := parseStructuredRecord(line)
		if err != nil {
			return StructuredJSONEvent[T]{}, false, s.transportError("invalid structured stream record", err)
		}

		switch record.recordType {
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
			payloadBytes, matErr := s.materializePayload(record)
			if matErr != nil {
				return StructuredJSONEvent[T]{}, false, matErr
			}
			var payload T
			if err := json.Unmarshal(payloadBytes, &payload); err != nil {
				return StructuredJSONEvent[T]{}, false, s.transportError("failed to decode structured payload", err)
			}
			s.monitor.SignalFirstContent()
			if record.recordType == StructuredRecordTypeCompletion {
				s.markTerminal()
				//nolint:errcheck // best-effort cleanup after completion
				_ = s.Close()
			}
			return StructuredJSONEvent[T]{
				Type:           record.recordType,
				Payload:        &payload,
				RequestID:      s.requestID,
				Usage:          record.usage,
				CompleteFields: buildCompleteFieldsMap(record.completeFields),
			}, true, nil
		case StructuredRecordTypeError:
			s.monitor.SignalFirstContent()
			s.markTerminal()
			//nolint:errcheck // best-effort cleanup after error
			_ = s.Close()
			status := record.status
			if status == 0 {
				status = http.StatusInternalServerError
			}
			msg := strings.TrimSpace(record.message)
			if msg == "" {
				msg = "structured stream error"
			}
			return StructuredJSONEvent[T]{}, false, APIError{
				Status:    status,
				Code:      APIErrorCode(record.code),
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
	kind := classifyTransportErrorKind(cause)
	if kind == TransportErrorOther {
		kind = TransportErrorRequest
	}
	return TransportError{
		Kind:    kind,
		Message: message,
		Cause:   cause,
		Retry:   s.retry,
	}
}

func (s *StructuredJSONStream[T]) protocolError(message string) error {
	return TransportError{
		Kind:    TransportErrorRequest,
		Message: message,
		Cause:   nil,
		Retry:   s.retry,
	}
}
