package sdk

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

const (
	localBashDefaultTimeout        time.Duration = 10 * time.Second
	localBashDefaultMaxOutputBytes uint64        = 32_000
	localBashHardMaxOutputBytes    uint64        = 256_000
)

// BashResult is the structured result for the tools.v0 `bash` tool.
//
// Tools.v0 defines a single `output` string result, but permits SDKs to return structured JSON.
// This result preserves command output even when the command fails, times out, or is truncated.
type BashResult struct {
	Output          string `json:"output"`
	ExitCode        int    `json:"exit_code"`
	TimedOut        bool   `json:"timed_out,omitempty"`
	OutputTruncated bool   `json:"output_truncated,omitempty"`
	Error           string `json:"error,omitempty"`
}

type localBashEnvMode uint8

const (
	localBashEnvEmpty localBashEnvMode = iota
	localBashEnvInheritAll
	localBashEnvAllowList
)

// BashCommandRule matches commands for allow/deny policies.
type BashCommandRule interface {
	Match(command string) bool
	String() string
}

// BashCommandExact matches a command exactly.
type BashCommandExact string

func (r BashCommandExact) Match(command string) bool { return command == string(r) }
func (r BashCommandExact) String() string            { return fmt.Sprintf("exact(%q)", string(r)) }

// BashCommandPrefix matches commands by prefix.
type BashCommandPrefix string

func (r BashCommandPrefix) Match(command string) bool { return strings.HasPrefix(command, string(r)) }
func (r BashCommandPrefix) String() string            { return fmt.Sprintf("prefix(%q)", string(r)) }

// BashCommandRegexp matches commands by regexp.
type BashCommandRegexp struct{ Re *regexp.Regexp }

func (r BashCommandRegexp) Match(command string) bool {
	return r.Re != nil && r.Re.MatchString(command)
}
func (r BashCommandRegexp) String() string {
	if r.Re == nil {
		return "regexp(<nil>)"
	}
	return fmt.Sprintf("regexp(%q)", r.Re.String())
}

type LocalBashOption func(*localBashConfig)

type localBashConfig struct {
	rootAbs string
	initErr error

	timeout time.Duration

	maxOutputBytes     uint64
	hardMaxOutputBytes uint64
	maxOutputBytesInt  int

	allowAll  bool
	allowList []BashCommandRule
	denyList  []BashCommandRule

	envMode  localBashEnvMode
	envAllow map[string]struct{}
}

// WithLocalBashTimeout sets a per-command timeout.
func WithLocalBashTimeout(d time.Duration) LocalBashOption {
	return func(c *localBashConfig) { c.timeout = d }
}

// WithLocalBashMaxOutputBytes configures the max output bytes captured from stdout/stderr.
func WithLocalBashMaxOutputBytes(n uint64) LocalBashOption {
	return func(c *localBashConfig) { c.maxOutputBytes = n }
}

// WithLocalBashHardMaxOutputBytes configures a hard cap for max output bytes.
func WithLocalBashHardMaxOutputBytes(n uint64) LocalBashOption {
	return func(c *localBashConfig) { c.hardMaxOutputBytes = n }
}

// WithLocalBashAllowAllCommands enables execution of any command (subject to deny rules).
func WithLocalBashAllowAllCommands() LocalBashOption {
	return func(c *localBashConfig) { c.allowAll = true }
}

// WithLocalBashAllowRules adds allow rules. If allow-all is not enabled, at least one allow rule is required.
func WithLocalBashAllowRules(rules ...BashCommandRule) LocalBashOption {
	return func(c *localBashConfig) { c.allowList = append(c.allowList, rules...) }
}

// WithLocalBashDenyRules adds deny rules. Deny rules always take precedence.
func WithLocalBashDenyRules(rules ...BashCommandRule) LocalBashOption {
	return func(c *localBashConfig) { c.denyList = append(c.denyList, rules...) }
}

// WithLocalBashInheritEnv passes the caller's full environment to the command.
func WithLocalBashInheritEnv() LocalBashOption {
	return func(c *localBashConfig) { c.envMode = localBashEnvInheritAll }
}

// WithLocalBashAllowEnvVars passes only the named env vars from the caller's environment.
func WithLocalBashAllowEnvVars(names ...string) LocalBashOption {
	return func(c *localBashConfig) {
		c.envMode = localBashEnvAllowList
		if c.envAllow == nil {
			c.envAllow = make(map[string]struct{})
		}
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			c.envAllow[name] = struct{}{}
		}
	}
}

// LocalBashToolPack provides an opt-in implementation of the tools.v0 `bash` tool.
//
// Safety properties:
// - Requires explicit opt-in: by default, no commands are allowed (deny-all).
// - Enforces a root working directory (cmd.Dir) and resolves root symlinks.
// - Enforces timeout and output byte caps.
// - Controls environment variable inheritance (default: empty env).
//
// Note: `bash` is intentionally powerful. This pack is not a full OS sandbox; prefer dedicated sandboxing
// mechanisms for untrusted code execution. This pack is designed for local developer workflows.
type LocalBashToolPack struct {
	cfg localBashConfig
}

// NewLocalBashToolPack creates a LocalBashToolPack sandboxed to the given root directory.
//
// If root is invalid, tools will return an error at execution time (fail fast).
func NewLocalBashToolPack(root string, opts ...LocalBashOption) *LocalBashToolPack {
	cfg := localBashConfig{
		timeout:            localBashDefaultTimeout,
		maxOutputBytes:     localBashDefaultMaxOutputBytes,
		hardMaxOutputBytes: localBashHardMaxOutputBytes,
		envMode:            localBashEnvEmpty,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	root = strings.TrimSpace(root)
	if root == "" {
		cfg.initErr = errors.New("local bash tool: root directory required")
		return &LocalBashToolPack{cfg: cfg}
	}
	if cfg.timeout <= 0 {
		cfg.initErr = errors.New("local bash tool: timeout must be > 0")
		return &LocalBashToolPack{cfg: cfg}
	}
	if cfg.hardMaxOutputBytes == 0 {
		cfg.initErr = errors.New("local bash tool: hard max output bytes must be > 0")
		return &LocalBashToolPack{cfg: cfg}
	}
	if cfg.maxOutputBytes == 0 {
		cfg.initErr = errors.New("local bash tool: max output bytes must be > 0")
		return &LocalBashToolPack{cfg: cfg}
	}
	if cfg.maxOutputBytes > cfg.hardMaxOutputBytes {
		cfg.initErr = errors.New("local bash tool: max output bytes exceeds hard cap")
		return &LocalBashToolPack{cfg: cfg}
	}
	maxInt := uint64(^uint(0) >> 1)
	if cfg.hardMaxOutputBytes > maxInt || cfg.maxOutputBytes > maxInt {
		cfg.initErr = errors.New("local bash tool: output byte caps must fit in int")
		return &LocalBashToolPack{cfg: cfg}
	}
	cfg.maxOutputBytesInt = int(cfg.maxOutputBytes)

	abs, err := filepath.Abs(root)
	if err != nil {
		cfg.initErr = fmt.Errorf("local bash tool: resolve root: %w", err)
		return &LocalBashToolPack{cfg: cfg}
	}
	info, err := os.Stat(abs)
	if err != nil {
		cfg.initErr = fmt.Errorf("local bash tool: stat root: %w", err)
		return &LocalBashToolPack{cfg: cfg}
	}
	if !info.IsDir() {
		cfg.initErr = fmt.Errorf("local bash tool: root is not a directory: %s", abs)
		return &LocalBashToolPack{cfg: cfg}
	}
	evalRoot, err := filepath.EvalSymlinks(abs)
	if err != nil {
		cfg.initErr = fmt.Errorf("local bash tool: resolve root symlinks: %w", err)
		return &LocalBashToolPack{cfg: cfg}
	}
	info, err = os.Stat(evalRoot)
	if err != nil {
		cfg.initErr = fmt.Errorf("local bash tool: stat resolved root: %w", err)
		return &LocalBashToolPack{cfg: cfg}
	}
	if !info.IsDir() {
		cfg.initErr = fmt.Errorf("local bash tool: resolved root is not a directory: %s", evalRoot)
		return &LocalBashToolPack{cfg: cfg}
	}
	cfg.rootAbs = evalRoot

	return &LocalBashToolPack{cfg: cfg}
}

// NewLocalBashTools returns a ToolRegistry with the LocalBashToolPack registered.
func NewLocalBashTools(root string, opts ...LocalBashOption) *ToolRegistry {
	reg := NewToolRegistry()
	NewLocalBashToolPack(root, opts...).RegisterInto(reg)
	return reg
}

// RegisterInto registers `bash` into the provided registry.
func (p *LocalBashToolPack) RegisterInto(registry *ToolRegistry) *ToolRegistry {
	if registry == nil {
		return nil
	}
	registry.Register("bash", p.bashTool)
	return registry
}

type bashArgs struct {
	Command string `json:"command"`
}

func (a *bashArgs) Validate() error {
	if strings.TrimSpace(a.Command) == "" {
		return errors.New("command is required")
	}
	return nil
}

func (p *LocalBashToolPack) ensureReady() error {
	if p == nil {
		return errors.New("local bash tool: pack is nil")
	}
	if p.cfg.initErr != nil {
		return p.cfg.initErr
	}
	if strings.TrimSpace(p.cfg.rootAbs) == "" {
		return errors.New("local bash tool: missing root")
	}
	return nil
}

func (p *LocalBashToolPack) bashTool(_ map[string]any, call llm.ToolCall) (any, error) {
	if err := p.ensureReady(); err != nil {
		return nil, err
	}

	var args bashArgs
	if err := ParseAndValidateToolArgs(call, &args); err != nil {
		return nil, err
	}

	cmdStr := strings.TrimSpace(args.Command)
	if err := p.checkCommandPolicy(cmdStr); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.timeout)
	defer cancel()

	var truncated bool
	w := newLimitedBuffer(p.cfg.maxOutputBytesInt, func() {
		truncated = true
		cancel()
	})

	//nolint:gosec // G204: this tool intentionally executes user-provided commands; guarded by explicit opt-in + policy.
	cmd := exec.CommandContext(ctx, "bash", "--noprofile", "--norc", "-c", cmdStr)
	cmd.Dir = p.cfg.rootAbs
	cmd.Env = p.buildEnv()
	cmd.Stdout = w
	cmd.Stderr = w

	err := cmd.Run()

	outStr := w.String()
	res := BashResult{
		Output:          outStr,
		ExitCode:        0,
		TimedOut:        ctx.Err() == context.DeadlineExceeded,
		OutputTruncated: truncated,
	}

	if err != nil {
		res.Error = err.Error()
		if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = -1
		}
	}

	return res, nil
}

func (p *LocalBashToolPack) buildEnv() []string {
	switch p.cfg.envMode {
	case localBashEnvInheritAll:
		return os.Environ()
	case localBashEnvAllowList:
		if len(p.cfg.envAllow) == 0 {
			return []string{}
		}
		out := make([]string, 0, len(p.cfg.envAllow))
		for _, kv := range os.Environ() {
			k, _, ok := strings.Cut(kv, "=")
			if !ok {
				continue
			}
			if _, allowed := p.cfg.envAllow[k]; allowed {
				out = append(out, kv)
			}
		}
		return out
	default:
		// In os/exec, nil Env means "inherit parent env". Use an empty slice to mean "no env".
		return []string{}
	}
}

func (p *LocalBashToolPack) checkCommandPolicy(command string) error {
	for _, r := range p.cfg.denyList {
		if r == nil {
			continue
		}
		if r.Match(command) {
			return fmt.Errorf("bash tool denied by policy: %s", r.String())
		}
	}

	if p.cfg.allowAll {
		return nil
	}
	if len(p.cfg.allowList) == 0 {
		return errors.New("bash tool disabled by default: configure WithLocalBashAllowAllCommands or WithLocalBashAllowRules")
	}
	for _, r := range p.cfg.allowList {
		if r == nil {
			continue
		}
		if r.Match(command) {
			return nil
		}
	}
	return errors.New("bash tool denied by policy: no allow rule matched")
}

type limitedBuffer struct {
	mu sync.Mutex

	limit int
	onHit func()

	buf bytes.Buffer
	hit bool
}

var errBashOutputCapReached = errors.New("bash tool output cap reached")

func newLimitedBuffer(limit int, onHit func()) *limitedBuffer {
	return &limitedBuffer{limit: limit, onHit: onHit}
}

func (w *limitedBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.hit || w.limit == 0 {
		return 0, errBashOutputCapReached
	}

	used := w.buf.Len()
	if used >= w.limit {
		w.hit = true
		if w.onHit != nil {
			w.onHit()
		}
		return 0, errBashOutputCapReached
	}
	remaining := w.limit - used

	if len(p) > remaining {
		_, _ = w.buf.Write(p[:remaining])
		w.hit = true
		if w.onHit != nil {
			w.onHit()
		}
		return remaining, errBashOutputCapReached
	}

	n, _ := w.buf.Write(p)
	return n, nil
}

func (w *limitedBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}
