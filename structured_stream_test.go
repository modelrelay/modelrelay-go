package sdk

import (
	"context"
	"errors"
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

	stream := newStructuredJSONStream[struct{}](ctx, &failingReadCloser{err: sentinelErr}, "req-structured-read-failure", nil)

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
	stream2 := newStructuredJSONStream[struct{}](ctx, &failingReadCloser{err: sentinelErr}, "req-structured-read-failure-collect", nil)
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

