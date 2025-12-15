package sdk

import (
	"context"
	"sync"
	"time"
)

// streamTimeoutMonitor manages timeout tracking for streaming responses.
// It handles three timeout types: TTFT (time to first token), Idle (between events),
// and Total (overall stream duration).
type streamTimeoutMonitor struct {
	timeouts StreamTimeouts
	activity chan struct{}
	first    chan struct{} // closed on first content event
	done     chan struct{}
	cancel   context.CancelFunc
	ctx      context.Context

	timeoutErrMu sync.Mutex
	timeoutErr   error

	firstOnce sync.Once
}

// newStreamTimeoutMonitor creates a timeout monitor for stream operations.
// Pass the stream's done channel and cancel function for lifecycle coordination.
func newStreamTimeoutMonitor(
	ctx context.Context,
	timeouts StreamTimeouts,
	done chan struct{},
	cancel context.CancelFunc,
) *streamTimeoutMonitor {
	return &streamTimeoutMonitor{
		ctx:      ctx,
		timeouts: timeouts,
		activity: make(chan struct{}, 1),
		first:    make(chan struct{}),
		done:     done,
		cancel:   cancel,
	}
}

// HasAnyTimeout returns true if any timeout is configured.
func (t StreamTimeouts) HasAnyTimeout() bool {
	return t.TTFT > 0 || t.Idle > 0 || t.Total > 0
}

// Start begins the timeout monitoring goroutine.
// Returns immediately if no timeouts are configured.
func (m *streamTimeoutMonitor) Start() {
	if !m.timeouts.HasAnyTimeout() {
		return
	}

	go m.run()
}

func (m *streamTimeoutMonitor) run() {
	var ttftTimer *time.Timer
	var ttftC <-chan time.Time
	if m.timeouts.TTFT > 0 {
		ttftTimer = time.NewTimer(m.timeouts.TTFT)
		ttftC = ttftTimer.C
	}

	var idleTimer *time.Timer
	var idleC <-chan time.Time
	if m.timeouts.Idle > 0 {
		idleTimer = time.NewTimer(m.timeouts.Idle)
		idleC = idleTimer.C
	}

	var totalTimer *time.Timer
	var totalC <-chan time.Time
	if m.timeouts.Total > 0 {
		totalTimer = time.NewTimer(m.timeouts.Total)
		totalC = totalTimer.C
	}

	firstCh := m.first

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
		case <-m.done:
			return
		case <-m.ctx.Done():
			return
		case <-firstCh:
			firstCh = nil
			if ttftTimer != nil {
				ttftTimer.Stop()
				ttftC = nil
			}
		case <-m.activity:
			if idleTimer != nil {
				if !idleTimer.Stop() {
					select {
					case <-idleTimer.C:
					default:
					}
				}
				idleTimer.Reset(m.timeouts.Idle)
				idleC = idleTimer.C
			}
		case <-ttftC:
			m.setTimeoutErr(StreamTimeoutError{Kind: StreamTimeoutTTFT, Timeout: m.timeouts.TTFT})
			m.cancel()
			return
		case <-idleC:
			m.setTimeoutErr(StreamTimeoutError{Kind: StreamTimeoutIdle, Timeout: m.timeouts.Idle})
			m.cancel()
			return
		case <-totalC:
			m.setTimeoutErr(StreamTimeoutError{Kind: StreamTimeoutTotal, Timeout: m.timeouts.Total})
			m.cancel()
			return
		}
	}
}

// SignalActivity resets the idle timer. Call this when any stream data is received.
func (m *streamTimeoutMonitor) SignalActivity() {
	select {
	case m.activity <- struct{}{}:
	default:
	}
}

// SignalFirstContent marks that the first content has been received,
// stopping the TTFT timer. Safe to call multiple times.
func (m *streamTimeoutMonitor) SignalFirstContent() {
	m.firstOnce.Do(func() {
		close(m.first)
	})
}

func (m *streamTimeoutMonitor) setTimeoutErr(err error) {
	m.timeoutErrMu.Lock()
	defer m.timeoutErrMu.Unlock()
	if m.timeoutErr == nil {
		m.timeoutErr = err
	}
}

// GetTimeoutErr returns the timeout error if one occurred.
func (m *streamTimeoutMonitor) GetTimeoutErr() error {
	m.timeoutErrMu.Lock()
	defer m.timeoutErrMu.Unlock()
	return m.timeoutErr
}
