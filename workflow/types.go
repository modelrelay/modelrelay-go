// Package workflow provides workflow specification types for workflow.v1.
//
// Types in this package use concise names without the "Workflow" prefix.
// For example, use workflow.SpecV1 instead of sdk.WorkflowSpecV1.
package workflow

import (
	"fmt"
	"strings"
)

// Kind identifies the versioned workflow spec kind.
type Kind string

const (
	KindV1 Kind = "workflow.v1"
)

// NodeID is a unique identifier for a node within a workflow.
type NodeID string

func NewNodeID(val string) NodeID { return NodeID(strings.TrimSpace(val)) }
func (id NodeID) String() string  { return string(id) }
func (id NodeID) Valid() bool     { return strings.TrimSpace(string(id)) != "" }

// OutputName is the name of a workflow output.
type OutputName string

func NewOutputName(val string) OutputName { return OutputName(strings.TrimSpace(val)) }
func (n OutputName) String() string       { return string(n) }
func (n OutputName) Valid() bool          { return strings.TrimSpace(string(n)) != "" }

// JSONPointer is an RFC 6901 JSON Pointer.
// The SDK does not validate semantics beyond basic formatting.
type JSONPointer string

func NewJSONPointer(val string) JSONPointer { return JSONPointer(strings.TrimSpace(val)) }
func (p JSONPointer) String() string        { return string(p) }
func (p JSONPointer) Valid() bool {
	if strings.TrimSpace(string(p)) == "" {
		return true
	}
	return strings.HasPrefix(string(p), "/")
}

// PlaceholderName is a named placeholder marker in a prompt (e.g., "tier_data" for {{tier_data}}).
type PlaceholderName string

func NewPlaceholderName(val string) PlaceholderName { return PlaceholderName(strings.TrimSpace(val)) }
func (n PlaceholderName) String() string            { return string(n) }
func (n PlaceholderName) Valid() bool               { return strings.TrimSpace(string(n)) != "" }

// Issue describes a validation issue with a workflow spec.
type Issue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationError is returned by workflow compilation/validation endpoints.
type ValidationError struct {
	Issues []Issue `json:"issues"`
}

func (e ValidationError) Error() string {
	if len(e.Issues) == 0 {
		return "workflow spec invalid"
	}
	if len(e.Issues) == 1 {
		return "workflow spec invalid: " + e.Issues[0].Message
	}
	return fmt.Sprintf("workflow spec invalid: %d issues", len(e.Issues))
}
