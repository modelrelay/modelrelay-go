package sdk

import (
	"math"
	"math/rand"
	"time"
)

// RetryConfig controls exponential backoff and attempt counts.
type RetryConfig struct {
	MaxAttempts int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	RetryPost   bool
}

// RetryMetadata describes what happened during retries.
type RetryMetadata struct {
	Attempts    int
	MaxAttempts int
	LastBackoff time.Duration
	LastStatus  int
	LastError   string
}

func defaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		BaseBackoff: 300 * time.Millisecond,
		MaxBackoff:  5 * time.Second,
		RetryPost:   false,
	}
}

func (r RetryConfig) normalized() RetryConfig {
	cfg := r
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = 300 * time.Millisecond
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 5 * time.Second
	}
	return cfg
}

func (r RetryConfig) backoffDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return 0
	}
	exp := attempt - 2
	base := float64(r.BaseBackoff) * math.Pow(2, float64(exp))
	cap := float64(r.MaxBackoff)
	if base > cap {
		base = cap
	}
	// jitter 0.5x..1.5x
	jitter := 0.5 + rand.Float64()
	d := time.Duration(base * jitter)
	if d > r.MaxBackoff {
		d = r.MaxBackoff
	}
	return d
}
