package sdk

import (
	"time"

	"github.com/google/uuid"
)

// UsagePoint represents a single data point for usage charting.
type UsagePoint struct {
	Day      time.Time  `json:"day"`
	APIKeyID *uuid.UUID `json:"api_key_id"`
	Quantity int64      `json:"quantity"`
}

// UsageQuotaState encodes the decision outcome for a quota evaluation.
type UsageQuotaState string

const (
	UsageQuotaStateAllowed   UsageQuotaState = "allowed"
	UsageQuotaStateExhausted UsageQuotaState = "exhausted"
)

// UsagePlanMeteringType distinguishes quota accounting modes.
type UsagePlanMeteringType string

const (
	UsagePlanMeteringToken           UsagePlanMeteringType = "token_metered"
	UsagePlanMeteringRequestWeighted UsagePlanMeteringType = "request_weighted"
)

// UsageSummary reports the counters and window metadata used for quota enforcement.
type UsageSummary struct {
	PlanType    UsagePlanMeteringType `json:"plan_type,omitempty"`
	WindowStart time.Time             `json:"window_start"`
	WindowEnd   time.Time             `json:"window_end"`
	Limit       int64                 `json:"limit"`
	Used        int64                 `json:"used"`
	Remaining   int64                 `json:"remaining"`
	State       UsageQuotaState       `json:"state"`
}
