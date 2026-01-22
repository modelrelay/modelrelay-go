package rlm

import "bytes"

// limitedWriter captures up to limit bytes from writes.
type limitedWriter struct {
	buf       *bytes.Buffer
	limit     int
	written   int
	truncated bool
}

func newLimitedWriter(limit int) *limitedWriter {
	return &limitedWriter{
		buf:   &bytes.Buffer{},
		limit: limit,
	}
}

func (lw *limitedWriter) Write(p []byte) (n int, err error) {
	if lw.written >= lw.limit {
		lw.truncated = true
		return len(p), nil
	}
	remaining := lw.limit - lw.written
	if len(p) > remaining {
		lw.buf.Write(p[:remaining])
		lw.written += remaining
		lw.truncated = true
		return len(p), nil
	}
	lw.buf.Write(p)
	lw.written += len(p)
	return len(p), nil
}

func (lw *limitedWriter) String() string {
	if lw.truncated {
		return lw.buf.String() + "\n[output truncated]"
	}
	return lw.buf.String()
}
