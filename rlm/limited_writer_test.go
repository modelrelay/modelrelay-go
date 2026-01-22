package rlm

import "testing"

func TestLimitedWriter_Truncates(t *testing.T) {
	lw := newLimitedWriter(3)
	_, _ = lw.Write([]byte("hello"))
	if got := lw.String(); got != "hel\n[output truncated]" {
		t.Fatalf("got %q, want truncated output", got)
	}
}

func TestLimitedWriter_NoLimit(t *testing.T) {
	lw := newLimitedWriter(0)
	_, _ = lw.Write([]byte("hello"))
	if got := lw.String(); got != "\n[output truncated]" {
		t.Fatalf("got %q, want truncated marker", got)
	}
}
