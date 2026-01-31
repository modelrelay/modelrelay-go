// Package rlm provides local RLM (Recursive Language Model) execution for the mrl CLI.
// It implements the CodeInterpreter interface using local Python subprocesses.
package rlm

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const defaultMaxInlineBytes = int64(128 * 1024) // 128KB

// LocalInterpreterConfig configures the local Python interpreter.
type LocalInterpreterConfig struct {
	PythonPath string
	Limits     InterpreterLimits
	Caps       InterpreterCapabilities
	Env        []string
	WorkDir    string
}

// LocalInterpreter runs Python locally using a subprocess.
type LocalInterpreter struct {
	cfg LocalInterpreterConfig
}

// NewLocalInterpreter constructs a LocalInterpreter with defaults.
func NewLocalInterpreter(cfg LocalInterpreterConfig) *LocalInterpreter {
	if strings.TrimSpace(cfg.PythonPath) == "" {
		cfg.PythonPath = "python3"
	}
	if cfg.Limits.MaxTimeoutMS == 0 {
		cfg.Limits.MaxTimeoutMS = 30000
	}
	if cfg.Limits.MaxOutputBytes == 0 {
		cfg.Limits.MaxOutputBytes = 1_048_576
	}
	if cfg.Caps.MaxInlineBytes == 0 {
		cfg.Caps.MaxInlineBytes = defaultMaxInlineBytes
	}
	return &LocalInterpreter{cfg: cfg}
}

// Limits returns interpreter limits.
func (l *LocalInterpreter) Limits() InterpreterLimits {
	if l == nil {
		return InterpreterLimits{}
	}
	return l.cfg.Limits
}

// Capabilities returns interpreter capabilities.
func (l *LocalInterpreter) Capabilities() InterpreterCapabilities {
	if l == nil {
		return InterpreterCapabilities{}
	}
	return l.cfg.Caps
}

// PlanContext applies the local default context policy (PreferInline=true).
func (l *LocalInterpreter) PlanContext(payload []byte, contextPath string) (ContextPlan, error) {
	policy := DefaultContextPolicy(l.Capabilities())
	policy.PreferInline = true
	return PlanContext(payload, policy, contextPath)
}

// Start creates a new local session. NetworkPolicy is currently ignored in trusted dev mode.
func (l *LocalInterpreter) Start(ctx context.Context, name string, _ *NetworkPolicy) (CodeSession, error) {
	if l == nil {
		return nil, errors.New("local interpreter not configured")
	}
	dir := l.cfg.WorkDir
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "modelrelay-rlm-")
		if err != nil {
			return nil, fmt.Errorf("create temp dir: %w", err)
		}
	}
	return &localSession{
		pythonPath: l.cfg.PythonPath,
		limits:     l.cfg.Limits,
		env:        append([]string(nil), l.cfg.Env...),
		workDir:    dir,
		ownsDir:    l.cfg.WorkDir == "",
	}, nil
}

type localSession struct {
	pythonPath string
	limits     InterpreterLimits
	env        []string
	workDir    string
	ownsDir    bool
}

func (s *localSession) WriteFile(_ context.Context, path string, data []byte, perm fs.FileMode) error {
	if s == nil {
		return errors.New("session not configured")
	}
	writePath := path
	if !filepath.IsAbs(path) {
		writePath = filepath.Join(s.workDir, path)
	}
	if err := os.MkdirAll(filepath.Dir(writePath), 0o750); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return os.WriteFile(writePath, data, perm)
}

func (s *localSession) RunPython(ctx context.Context, script string, env []string, timeoutMS int) (*ExecutionResult, error) {
	if s == nil {
		return nil, errors.New("session not configured")
	}
	if strings.TrimSpace(script) == "" {
		return nil, &ExecutionError{Kind: ExecutionErrorUnknown, Cause: errors.New("empty script")}
	}
	timeout := timeoutMS
	if timeout <= 0 {
		timeout = s.limits.MaxTimeoutMS
	}
	if s.limits.MaxTimeoutMS > 0 && timeout > s.limits.MaxTimeoutMS {
		timeout = s.limits.MaxTimeoutMS
	}
	execCtx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
		defer cancel()
	}

	cmd := exec.CommandContext(execCtx, s.pythonPath, "-c", script) //nolint:gosec // pythonPath is user-configured, script is from RLM orchestration
	cmd.Dir = s.workDir
	if len(env) > 0 {
		cmd.Env = append([]string(nil), env...)
	} else if len(s.env) > 0 {
		cmd.Env = append([]string(nil), s.env...)
	} else {
		cmd.Env = os.Environ()
	}

	stdout := newLimitedWriter(s.limits.MaxOutputBytes)
	stderr := newLimitedWriter(s.limits.MaxOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	start := time.Now()
	runErr := cmd.Run()
	durationMS := time.Since(start).Milliseconds()

	result := &ExecutionResult{
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		DurationMS: durationMS,
	}

	if execCtx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		return result, ErrExecutionTimeout
	}

	if runErr != nil {
		exitCode := 1
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		result.ExitCode = exitCode
		return result, &ExecutionError{
			Kind:     ExecutionErrorExit,
			ExitCode: exitCode,
			Stderr:   result.Stderr,
			Cause:    runErr,
		}
	}

	return result, nil
}

func (s *localSession) Close() {
	if s == nil {
		return
	}
	if s.ownsDir && s.workDir != "" {
		if err := os.RemoveAll(s.workDir); err != nil {
			log.Printf("warning: failed to remove RLM temp dir %s: %v", s.workDir, err)
		}
	}
}
