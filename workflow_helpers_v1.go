package sdk

import (
	"encoding/json"

	"github.com/modelrelay/modelrelay/sdk/go/workflow"
)

// =============================================================================
// Condition Factories
// =============================================================================

// WhenOutputEquals creates a condition that matches when a node's output equals a specific value.
// The path must be a valid JSONPath expression starting with $.
//
// Example:
//
//	builder.EdgeWhen("router", "billing", WhenOutputEquals("$.route", "billing"))
func WhenOutputEquals(path string, value any) ConditionV1 {
	raw, _ := json.Marshal(value)
	return ConditionV1{
		Source: ConditionSourceNodeOutput,
		Op:     ConditionOpEquals,
		Path:   workflow.JSONPath(path),
		Value:  raw,
	}
}

// WhenOutputMatches creates a condition that matches when a node's output matches a regex pattern.
// The path must be a valid JSONPath expression starting with $.
//
// Example:
//
//	builder.EdgeWhen("router", "handler", WhenOutputMatches("$.category", "billing|support"))
func WhenOutputMatches(path string, pattern string) ConditionV1 {
	raw, _ := json.Marshal(pattern)
	return ConditionV1{
		Source: ConditionSourceNodeOutput,
		Op:     ConditionOpMatches,
		Path:   workflow.JSONPath(path),
		Value:  raw,
	}
}

// WhenOutputExists creates a condition that matches when a path exists in the node's output.
// The path must be a valid JSONPath expression starting with $.
//
// Example:
//
//	builder.EdgeWhen("router", "handler", WhenOutputExists("$.special_case"))
func WhenOutputExists(path string) ConditionV1 {
	return ConditionV1{
		Source: ConditionSourceNodeOutput,
		Op:     ConditionOpExists,
		Path:   workflow.JSONPath(path),
	}
}

// WhenStatusEquals creates a condition that matches when a node's status equals a specific value.
// The path must be a valid JSONPath expression starting with $.
//
// Example:
//
//	builder.EdgeWhen("node", "handler", WhenStatusEquals("$.status", "succeeded"))
func WhenStatusEquals(path string, value any) ConditionV1 {
	raw, _ := json.Marshal(value)
	return ConditionV1{
		Source: ConditionSourceNodeStatus,
		Op:     ConditionOpEquals,
		Path:   workflow.JSONPath(path),
		Value:  raw,
	}
}

// WhenStatusMatches creates a condition that matches when a node's status matches a regex pattern.
func WhenStatusMatches(path string, pattern string) ConditionV1 {
	raw, _ := json.Marshal(pattern)
	return ConditionV1{
		Source: ConditionSourceNodeStatus,
		Op:     ConditionOpMatches,
		Path:   workflow.JSONPath(path),
		Value:  raw,
	}
}

// WhenStatusExists creates a condition that matches when a path exists in the node's status.
func WhenStatusExists(path string) ConditionV1 {
	return ConditionV1{
		Source: ConditionSourceNodeStatus,
		Op:     ConditionOpExists,
		Path:   workflow.JSONPath(path),
	}
}

// =============================================================================
// Binding Factories
// =============================================================================

// BindToPlaceholderV1 creates a binding that injects a value into a {{placeholder}} in the prompt.
// Uses json_string encoding by default.
//
// Example:
//
//	builder.LLMResponsesNodeWithBindings("aggregate", request, nil, []LLMResponsesBindingV1{
//	    BindToPlaceholderV1("join", "route_output"),
//	})
func BindToPlaceholderV1(from NodeID, placeholder PlaceholderName) LLMResponsesBindingV1 {
	return LLMResponsesBindingV1{
		From:          from,
		ToPlaceholder: placeholder,
		Encoding:      LLMResponsesBindingEncodingJSONStringV1,
	}
}

// BindToPlaceholderWithPointerV1 creates a binding with a source pointer that injects into a placeholder.
//
// Example:
//
//	builder.LLMResponsesNodeWithBindings("aggregate", request, nil, []LLMResponsesBindingV1{
//	    BindToPlaceholderWithPointerV1("fanout", "/results", "data"),
//	})
func BindToPlaceholderWithPointerV1(from NodeID, pointer JSONPointer, placeholder PlaceholderName) LLMResponsesBindingV1 {
	return LLMResponsesBindingV1{
		From:          from,
		Pointer:       pointer,
		ToPlaceholder: placeholder,
		Encoding:      LLMResponsesBindingEncodingJSONStringV1,
	}
}

// BindToPointerV1 creates a binding that injects a value at a specific JSON pointer in the request.
// Uses json_string encoding by default.
//
// Example:
//
//	builder.LLMResponsesNodeWithBindings("processor", request, nil, []LLMResponsesBindingV1{
//	    BindToPointerV1("source", "/input/0/content/0/text"),
//	})
func BindToPointerV1(from NodeID, to JSONPointer) LLMResponsesBindingV1 {
	return LLMResponsesBindingV1{
		From:     from,
		To:       to,
		Encoding: LLMResponsesBindingEncodingJSONStringV1,
	}
}

// BindToPointerWithSourceV1 creates a binding with both source and destination pointers.
//
// Example:
//
//	builder.LLMResponsesNodeWithBindings("processor", request, nil, []LLMResponsesBindingV1{
//	    BindToPointerWithSourceV1("source", "/output/text", "/input/0/content/0/text"),
//	})
func BindToPointerWithSourceV1(from NodeID, sourcePointer JSONPointer, to JSONPointer) LLMResponsesBindingV1 {
	return LLMResponsesBindingV1{
		From:     from,
		Pointer:  sourcePointer,
		To:       to,
		Encoding: LLMResponsesBindingEncodingJSONStringV1,
	}
}

// BindingBuilderV1 provides a fluent API for constructing v1 bindings.
type BindingBuilderV1 struct {
	binding LLMResponsesBindingV1
}

// BindFromV1 creates a new BindingBuilderV1 starting with the source node.
//
// Example:
//
//	binding := BindFromV1("source").
//	    Pointer("/output/text").
//	    ToPlaceholder("data").
//	    Build()
func BindFromV1(from NodeID) *BindingBuilderV1 {
	return &BindingBuilderV1{
		binding: LLMResponsesBindingV1{
			From:     from,
			Encoding: LLMResponsesBindingEncodingJSONStringV1,
		},
	}
}

// Pointer sets the source pointer to extract from the node's output.
func (b *BindingBuilderV1) Pointer(ptr JSONPointer) *BindingBuilderV1 {
	b.binding.Pointer = ptr
	return b
}

// To sets the destination JSON pointer in the request.
func (b *BindingBuilderV1) To(ptr JSONPointer) *BindingBuilderV1 {
	b.binding.To = ptr
	b.binding.ToPlaceholder = ""
	return b
}

// ToPlaceholder sets the destination placeholder name.
func (b *BindingBuilderV1) ToPlaceholder(name PlaceholderName) *BindingBuilderV1 {
	b.binding.ToPlaceholder = name
	b.binding.To = ""
	return b
}

// Encoding sets the encoding for the binding value.
func (b *BindingBuilderV1) Encoding(enc LLMResponsesBindingEncodingV1) *BindingBuilderV1 {
	b.binding.Encoding = enc
	return b
}

// Build returns the constructed binding.
func (b *BindingBuilderV1) Build() LLMResponsesBindingV1 {
	return b.binding
}
