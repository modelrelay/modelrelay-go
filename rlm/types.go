// Package rlm provides local RLM (Recursive Language Model) execution.
// These types are copied from platform/rlm to avoid private repo dependencies.
package rlm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
)

// ErrExecutionTimeout indicates the interpreter timed out while executing code.
var ErrExecutionTimeout = errors.New("rlm interpreter timeout")

// InterpreterLimits defines resource limits for an interpreter runtime.
type InterpreterLimits struct {
	MaxTimeoutMS   int
	MaxMemoryMB    int
	MaxCPUCores    int
	MaxOutputBytes int
}

// InterpreterCapabilities describe optional interpreter characteristics.
// A zero value means the capability is unknown or unlimited.
type InterpreterCapabilities struct {
	SupportsLazyRead bool
	MaxInlineBytes   int64
	MaxTotalBytes    int64
}

// NetworkAction represents an allow/deny action for network policy rules.
type NetworkAction string

const (
	// NetworkActionAllow permits outbound requests to the domain.
	NetworkActionAllow NetworkAction = "allow"
	// NetworkActionDeny blocks outbound requests to the domain.
	NetworkActionDeny NetworkAction = "deny"
)

// NetworkPolicyRule defines an allow/deny rule for outbound requests.
type NetworkPolicyRule struct {
	Domain string
	Action NetworkAction
}

// NetworkPolicy restricts outbound network access for code execution.
type NetworkPolicy struct {
	Rules []NetworkPolicyRule
}

// ExecutionResult captures the outcome of code execution.
type ExecutionResult struct {
	Stdout     string
	Stderr     string
	ExitCode   int
	DurationMS int64
	TimedOut   bool
}

// CodeSession is a long-lived interpreter session.
type CodeSession interface {
	WriteFile(ctx context.Context, path string, data []byte, perm fs.FileMode) error
	RunPython(ctx context.Context, script string, env []string, timeoutMS int) (*ExecutionResult, error)
	Close()
}

// CodeInterpreter defines the contract for Python code execution backends.
type CodeInterpreter interface {
	Limits() InterpreterLimits
	Capabilities() InterpreterCapabilities
	Start(ctx context.Context, name string, policy *NetworkPolicy) (CodeSession, error)
}

// ExecutionErrorKind classifies interpreter execution failures.
type ExecutionErrorKind string

const (
	ExecutionErrorUnknown ExecutionErrorKind = "unknown"
	ExecutionErrorExit    ExecutionErrorKind = "exit"
	ExecutionErrorPolicy  ExecutionErrorKind = "policy"
)

// ExecutionError wraps an execution failure with structured metadata.
type ExecutionError struct {
	Kind     ExecutionErrorKind
	ExitCode int
	Stderr   string
	Cause    error
}

func (e *ExecutionError) Error() string {
	if e == nil {
		return ""
	}
	if e.Stderr != "" {
		return e.Stderr
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return "execution failed"
}

func (e *ExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ContextLoadMode determines how context is provided to the wrapper.
type ContextLoadMode string

const (
	ContextLoadFile   ContextLoadMode = "file"
	ContextLoadInline ContextLoadMode = "inline"
)

// ContextPolicy selects a context loading strategy based on size and capabilities.
type ContextPolicy struct {
	MaxInlineBytes int64
	MaxTotalBytes  int64
	PreferInline   bool
}

// DefaultContextPolicy builds a policy from interpreter capabilities.
func DefaultContextPolicy(capabilities InterpreterCapabilities) ContextPolicy {
	return ContextPolicy{
		MaxInlineBytes: capabilities.MaxInlineBytes,
		MaxTotalBytes:  capabilities.MaxTotalBytes,
		PreferInline:   true,
	}
}

// ContextPlan defines how to load context in the Python wrapper.
type ContextPlan struct {
	Mode        ContextLoadMode
	ContextPath string
	InlineJSON  json.RawMessage
}

// PlanContext determines a context plan based on payload size and policy.
func PlanContext(payload json.RawMessage, policy ContextPolicy, contextPath string) (ContextPlan, error) {
	if policy.MaxTotalBytes > 0 && int64(len(payload)) > policy.MaxTotalBytes {
		return ContextPlan{}, fmt.Errorf("context size exceeds max_total_bytes")
	}
	if policy.PreferInline && policy.MaxInlineBytes > 0 && int64(len(payload)) <= policy.MaxInlineBytes {
		return ContextPlan{
			Mode:       ContextLoadInline,
			InlineJSON: payload,
		}, nil
	}
	if policy.PreferInline && policy.MaxInlineBytes == 0 {
		return ContextPlan{
			Mode:       ContextLoadInline,
			InlineJSON: payload,
		}, nil
	}
	return ContextPlan{
		Mode:        ContextLoadFile,
		ContextPath: contextPath,
	}, nil
}
