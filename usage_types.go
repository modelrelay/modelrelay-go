package sdk

import "encoding/json"

// Usage provides token accounting metadata.
type Usage struct {
	InputTokens     int64 `json:"input_tokens"`
	OutputTokens    int64 `json:"output_tokens"`
	TotalTokens     int64 `json:"total_tokens"`
	CachedTokens    int64 `json:"cached_tokens,omitempty"`
	ReasoningTokens int64 `json:"reasoning_tokens,omitempty"`
}

// Total returns provider-reported total tokens or a best-effort sum.
func (u Usage) Total() int64 {
	if u.TotalTokens > 0 {
		return u.TotalTokens
	}
	return u.InputTokens + u.OutputTokens
}

func (u Usage) MarshalJSON() ([]byte, error) {
	type alias Usage
	return json.Marshal(alias(u))
}

func (u *Usage) UnmarshalJSON(data []byte) error {
	type alias Usage
	var tmp alias
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	*u = Usage(tmp)
	return nil
}
