package llm

import (
	"encoding/json"
	"fmt"
	"regexp"
	"unicode"
)

var toolNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)

// ToolName is the identifier used for function tools.
//
// For tools.v0 client tools, use underscore-separated lowercase segments (e.g. "fs_search").
type ToolName string

func (n ToolName) String() string { return string(n) }

func (n ToolName) Validate() error {
	if n == "" {
		return fmt.Errorf("tool name is required")
	}
	for _, r := range n {
		if unicode.IsSpace(r) {
			return fmt.Errorf("tool name must not contain whitespace")
		}
	}
	if len(n) > 128 {
		return fmt.Errorf("tool name too long: max 128 bytes")
	}
	if !toolNamePattern.MatchString(string(n)) {
		return fmt.Errorf("invalid tool name: %q", string(n))
	}
	return nil
}

// ParseToolName validates and returns a ToolName.
func ParseToolName(raw string) (ToolName, error) {
	n := ToolName(raw)
	if err := n.Validate(); err != nil {
		return "", err
	}
	return n, nil
}

func (n ToolName) MarshalJSON() ([]byte, error) { return json.Marshal(string(n)) }

func (n *ToolName) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	parsed, err := ParseToolName(raw)
	if err != nil {
		return err
	}
	*n = parsed
	return nil
}

// ToolCallID is the opaque identifier used to correlate tool calls with tool results.
type ToolCallID string

func (id ToolCallID) String() string { return string(id) }

func (id ToolCallID) Validate() error {
	if id == "" {
		return fmt.Errorf("tool call id is required")
	}
	for _, r := range id {
		if unicode.IsSpace(r) {
			return fmt.Errorf("tool call id must not contain whitespace")
		}
	}
	if len(id) > 1024 {
		return fmt.Errorf("tool call id too long: max 1024 bytes")
	}
	return nil
}

// ParseToolCallID validates and returns a ToolCallID.
func ParseToolCallID(raw string) (ToolCallID, error) {
	id := ToolCallID(raw)
	if err := id.Validate(); err != nil {
		return "", err
	}
	return id, nil
}

func (id ToolCallID) MarshalJSON() ([]byte, error) { return json.Marshal(string(id)) }

func (id *ToolCallID) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	parsed, err := ParseToolCallID(raw)
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}
