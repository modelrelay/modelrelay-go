package sdk

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

// failingReadCloser simulates a transport-level read failure with no bytes
// returned. This reproduces the condition where StructuredJSONStream.Next
// should surface a TransportError with message "structured stream read failed".
type failingReadCloser struct {
	err error
}

func (f *failingReadCloser) Read(p []byte) (int, error) {
	return 0, f.err
}

func (f *failingReadCloser) Close() error {
	return nil
}

func TestStructuredJSONStream_ReadFailureReturnsTransportError(t *testing.T) {
	ctx := context.Background()
	sentinelErr := errors.New("synthetic read failure")

	stream := newStructuredJSONStream[struct{}](ctx, &failingReadCloser{err: sentinelErr}, "req-structured-read-failure", nil, StreamTimeouts{})

	_, ok, err := stream.Next()
	if err == nil {
		t.Fatal("expected error from Next, got nil")
	}
	if ok {
		t.Fatalf("expected ok=false from Next, got %v", ok)
	}

	var terr TransportError
	if !errors.As(err, &terr) {
		t.Fatalf("expected TransportError from Next, got %T", err)
	}
	if terr.Message != "structured stream read failed" {
		t.Fatalf("expected TransportError.Message %q, got %q", "structured stream read failed", terr.Message)
	}
	if !errors.Is(terr, sentinelErr) {
		t.Fatalf("expected TransportError to wrap sentinel error, got Cause=%v", terr.Cause)
	}

	// Collect should surface the same transport error shape.
	stream2 := newStructuredJSONStream[struct{}](ctx, &failingReadCloser{err: sentinelErr}, "req-structured-read-failure-collect", nil, StreamTimeouts{})
	_, err = stream2.Collect(ctx)
	if err == nil {
		t.Fatal("expected error from Collect, got nil")
	}
	var terr2 TransportError
	if !errors.As(err, &terr2) {
		t.Fatalf("expected TransportError from Collect, got %T", err)
	}
	if terr2.Message != "structured stream read failed" {
		t.Fatalf("expected TransportError.Message %q from Collect, got %q", "structured stream read failed", terr2.Message)
	}
	if !errors.Is(terr2, sentinelErr) {
		t.Fatalf("expected TransportError from Collect to wrap sentinel error, got Cause=%v", terr2.Cause)
	}
}

func TestStructuredJSONStream_MissingTypeReturnsProtocolError(t *testing.T) {
	ctx := context.Background()

	type Simple struct {
		Name string `json:"name"`
	}

	ndjson := `{"payload":{"name":"Test"}}
`
	stream := newStructuredJSONStream[Simple](ctx, newNDJSONReadCloser(ndjson), "req-missing-type", nil, StreamTimeouts{})
	defer stream.Close()

	_, ok, err := stream.Next()
	if err == nil {
		t.Fatal("expected error from Next, got nil")
	}
	if ok {
		t.Fatalf("expected ok=false from Next, got %v", ok)
	}

	var terr TransportError
	if !errors.As(err, &terr) {
		t.Fatalf("expected TransportError from Next, got %T", err)
	}
	if terr.Message != "structured stream record missing type" {
		t.Fatalf("expected TransportError.Message %q, got %q", "structured stream record missing type", terr.Message)
	}
}

// ndjsonReadCloser wraps a string buffer for testing NDJSON parsing.
type ndjsonReadCloser struct {
	*bytes.Reader
}

func (n *ndjsonReadCloser) Close() error { return nil }

func newNDJSONReadCloser(data string) io.ReadCloser {
	return &ndjsonReadCloser{Reader: bytes.NewReader([]byte(data))}
}

func TestStructuredJSONStream_CompleteFieldsParsing(t *testing.T) {
	ctx := context.Background()

	type Article struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}

	// Simulate a stream with update (partial fields) and completion (all fields)
	ndjson := `{"type":"start","request_id":"req-1","provider":"test","model":"test-model"}
{"type":"update","patch":[{"op":"add","path":"/title","value":"Hello"}],"complete_fields":["title"]}
{"type":"update","patch":[{"op":"add","path":"/body","value":"World"}],"complete_fields":["title","body"]}
{"type":"completion","payload":{"title":"Hello","body":"World"},"complete_fields":["title","body"]}
`
	stream := newStructuredJSONStream[Article](ctx, newNDJSONReadCloser(ndjson), "req-1", nil, StreamTimeouts{})
	defer stream.Close()

	// First event: update with only title complete
	event1, ok, err := stream.Next()
	if err != nil {
		t.Fatalf("unexpected error on first Next: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true on first Next")
	}
	if event1.Type != StructuredRecordTypeUpdate {
		t.Errorf("expected update, got %s", event1.Type)
	}
	if !event1.CompleteFields["title"] {
		t.Error("expected 'title' in CompleteFields for first update")
	}
	if event1.CompleteFields["body"] {
		t.Error("expected 'body' NOT in CompleteFields for first update")
	}

	// Second event: update with both fields complete
	event2, ok, err := stream.Next()
	if err != nil {
		t.Fatalf("unexpected error on second Next: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true on second Next")
	}
	if !event2.CompleteFields["title"] {
		t.Error("expected 'title' in CompleteFields for second update")
	}
	if !event2.CompleteFields["body"] {
		t.Error("expected 'body' in CompleteFields for second update")
	}

	// Third event: completion
	event3, ok, err := stream.Next()
	if err != nil {
		t.Fatalf("unexpected error on third Next: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true on third Next")
	}
	if event3.Type != StructuredRecordTypeCompletion {
		t.Errorf("expected completion, got %s", event3.Type)
	}
	if !event3.CompleteFields["title"] || !event3.CompleteFields["body"] {
		t.Error("expected both 'title' and 'body' in CompleteFields for completion")
	}

	// Stream should be done
	_, ok, err = stream.Next()
	if err != nil {
		t.Fatalf("unexpected error after completion: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false after completion")
	}
}

func TestStructuredJSONStream_CompleteFieldsFiltersEmptyStrings(t *testing.T) {
	ctx := context.Background()

	type Simple struct {
		Name string `json:"name"`
	}

	// Include empty strings and whitespace-only strings in complete_fields
	ndjson := `{"type":"update","patch":[{"op":"add","path":"/name","value":"Test"}],"complete_fields":["name", "", "  ", "other"]}
{"type":"completion","payload":{"name":"Test"},"complete_fields":["name"]}
`
	stream := newStructuredJSONStream[Simple](ctx, newNDJSONReadCloser(ndjson), "req-2", nil, StreamTimeouts{})
	defer stream.Close()

	event, ok, err := stream.Next()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}

	// Should have name and other, but not empty strings
	if !event.CompleteFields["name"] {
		t.Error("expected 'name' in CompleteFields")
	}
	if !event.CompleteFields["other"] {
		t.Error("expected 'other' in CompleteFields")
	}
	// Empty strings should have been filtered
	if event.CompleteFields[""] {
		t.Error("expected empty string NOT in CompleteFields")
	}
	// The length should be 2 (name, other) - not including empty or whitespace
	if len(event.CompleteFields) != 2 {
		t.Errorf("expected 2 fields in CompleteFields, got %d: %v", len(event.CompleteFields), event.CompleteFields)
	}
}

func TestStructuredJSONStream_UsageParsing(t *testing.T) {
	ctx := context.Background()

	type Simple struct {
		Name string `json:"name"`
	}

	ndjson := `{"type":"start","request_id":"req-usage","provider":"test","model":"test-model"}
{"type":"update","patch":[{"op":"add","path":"/name","value":"Test"}],"complete_fields":["name"]}
{"type":"completion","payload":{"name":"Test"},"complete_fields":["name"],"usage":{"input_tokens":3,"output_tokens":5,"total_tokens":8}}
`
	stream := newStructuredJSONStream[Simple](ctx, newNDJSONReadCloser(ndjson), "req-usage", nil, StreamTimeouts{})
	defer stream.Close()

	_, ok, err := stream.Next()
	if err != nil {
		t.Fatalf("unexpected error on update Next: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true on update Next")
	}

	event, ok, err := stream.Next()
	if err != nil {
		t.Fatalf("unexpected error on completion Next: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true on completion Next")
	}
	if event.Type != StructuredRecordTypeCompletion {
		t.Fatalf("expected completion, got %s", event.Type)
	}
	if event.Usage == nil {
		t.Fatal("expected usage on completion event")
	}
	if event.Usage.InputTokens != 3 {
		t.Errorf("expected input_tokens=3, got %d", event.Usage.InputTokens)
	}
	if event.Usage.OutputTokens != 5 {
		t.Errorf("expected output_tokens=5, got %d", event.Usage.OutputTokens)
	}
	if event.Usage.TotalTokens != 8 {
		t.Errorf("expected total_tokens=8, got %d", event.Usage.TotalTokens)
	}
}
