package sdk

import "time"

// DurationPtr is a convenience helper for optional timeout fields.
func DurationPtr(d time.Duration) *time.Duration { return &d }

// BoolPtr is a convenience helper for optional boolean fields.
func BoolPtr(b bool) *bool { return &b }

// Int64Ptr is a convenience helper for optional int64 fields.
func Int64Ptr(v int64) *int64 { return &v }
