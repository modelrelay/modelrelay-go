package sdk

import "time"

// DurationPtr is a convenience helper for optional timeout fields.
func DurationPtr(d time.Duration) *time.Duration { return &d }
