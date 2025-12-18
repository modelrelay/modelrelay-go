package sdk

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

const (
	localFSDefaultMaxReadBytes     uint64        = 64_000
	localFSHardMaxReadBytes        uint64        = 1_000_000
	localFSDefaultMaxListEntries   uint64        = 2_000
	localFSHardMaxListEntries      uint64        = 20_000
	localFSDefaultMaxSearchMatches uint64        = 100
	localFSHardMaxSearchMatches    uint64        = 2_000
	localFSDefaultSearchTimeout    time.Duration = 5 * time.Second
	localFSDefaultMaxSearchBytes   uint64        = 1_000_000
)

type localFSStopWalk struct{}

func (localFSStopWalk) Error() string { return "local fs stop walk" }

type LocalFSOption func(*localFSConfig)

type localFSConfig struct {
	rootAbs string
	initErr error

	ignoreDirNames map[string]struct{}

	maxReadBytes     uint64
	hardMaxReadBytes uint64

	maxListEntries     uint64
	hardMaxListEntries uint64

	maxSearchMatches     uint64
	hardMaxSearchMatches uint64

	searchTimeout  time.Duration
	maxSearchBytes uint64
}

// WithLocalFSIgnoreDirs configures directory names to skip during fs.list_files and fs.search.
// Names are matched by path segment (e.g. ".git" skips any ".git" directory anywhere under root).
func WithLocalFSIgnoreDirs(names ...string) LocalFSOption {
	return func(c *localFSConfig) {
		if c.ignoreDirNames == nil {
			c.ignoreDirNames = make(map[string]struct{})
		}
		for _, n := range names {
			n = strings.TrimSpace(n)
			if n == "" {
				continue
			}
			c.ignoreDirNames[n] = struct{}{}
		}
	}
}

// WithLocalFSMaxReadBytes changes the default max_bytes when the tool call does not specify max_bytes.
func WithLocalFSMaxReadBytes(n uint64) LocalFSOption {
	return func(c *localFSConfig) { c.maxReadBytes = n }
}

// WithLocalFSHardMaxReadBytes changes the hard cap for fs.read_file max_bytes.
func WithLocalFSHardMaxReadBytes(n uint64) LocalFSOption {
	return func(c *localFSConfig) { c.hardMaxReadBytes = n }
}

// WithLocalFSMaxListEntries changes the default max_entries when the tool call does not specify max_entries.
func WithLocalFSMaxListEntries(n uint64) LocalFSOption {
	return func(c *localFSConfig) { c.maxListEntries = n }
}

// WithLocalFSHardMaxListEntries changes the hard cap for fs.list_files max_entries.
func WithLocalFSHardMaxListEntries(n uint64) LocalFSOption {
	return func(c *localFSConfig) { c.hardMaxListEntries = n }
}

// WithLocalFSMaxSearchMatches changes the default max_matches when the tool call does not specify max_matches.
func WithLocalFSMaxSearchMatches(n uint64) LocalFSOption {
	return func(c *localFSConfig) { c.maxSearchMatches = n }
}

// WithLocalFSHardMaxSearchMatches changes the hard cap for fs.search max_matches.
func WithLocalFSHardMaxSearchMatches(n uint64) LocalFSOption {
	return func(c *localFSConfig) { c.hardMaxSearchMatches = n }
}

// WithLocalFSSearchTimeout configures the timeout for fs.search.
func WithLocalFSSearchTimeout(d time.Duration) LocalFSOption {
	return func(c *localFSConfig) { c.searchTimeout = d }
}

// WithLocalFSMaxSearchBytes sets a per-file byte limit for the Go fallback implementation of fs.search.
func WithLocalFSMaxSearchBytes(n uint64) LocalFSOption {
	return func(c *localFSConfig) { c.maxSearchBytes = n }
}

// LocalFSToolPack provides safe-by-default implementations of tools.v0 filesystem tools:
// - fs.read_file
// - fs.list_files
// - fs.search
//
// The pack enforces a root sandbox, path traversal prevention, ignore lists, and size/time caps.
//
// fs.search uses ripgrep ("rg") if available, otherwise falls back to a Go implementation.
type LocalFSToolPack struct {
	cfg localFSConfig

	rgOnce sync.Once
	rgPath string
}

// NewLocalFSToolPack creates a LocalFSToolPack sandboxed to the given root directory.
//
// If root is invalid, tools will return an error at execution time (fail fast).
func NewLocalFSToolPack(root string, opts ...LocalFSOption) *LocalFSToolPack {
	cfg := localFSConfig{
		ignoreDirNames: defaultIgnoredDirNames(),

		maxReadBytes:     localFSDefaultMaxReadBytes,
		hardMaxReadBytes: localFSHardMaxReadBytes,

		maxListEntries:     localFSDefaultMaxListEntries,
		hardMaxListEntries: localFSHardMaxListEntries,

		maxSearchMatches:     localFSDefaultMaxSearchMatches,
		hardMaxSearchMatches: localFSHardMaxSearchMatches,

		searchTimeout:  localFSDefaultSearchTimeout,
		maxSearchBytes: localFSDefaultMaxSearchBytes,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	root = strings.TrimSpace(root)
	if root == "" {
		cfg.initErr = errors.New("local fs tools: root directory required")
		return &LocalFSToolPack{cfg: cfg}
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		cfg.initErr = fmt.Errorf("local fs tools: resolve root: %w", err)
		return &LocalFSToolPack{cfg: cfg}
	}
	info, err := os.Stat(abs)
	if err != nil {
		cfg.initErr = fmt.Errorf("local fs tools: stat root: %w", err)
		return &LocalFSToolPack{cfg: cfg}
	}
	if !info.IsDir() {
		cfg.initErr = fmt.Errorf("local fs tools: root is not a directory: %s", abs)
		return &LocalFSToolPack{cfg: cfg}
	}
	evalRoot, err := filepath.EvalSymlinks(abs)
	if err != nil {
		cfg.initErr = fmt.Errorf("local fs tools: resolve root symlinks: %w", err)
		return &LocalFSToolPack{cfg: cfg}
	}
	info, err = os.Stat(evalRoot)
	if err != nil {
		cfg.initErr = fmt.Errorf("local fs tools: stat resolved root: %w", err)
		return &LocalFSToolPack{cfg: cfg}
	}
	if !info.IsDir() {
		cfg.initErr = fmt.Errorf("local fs tools: resolved root is not a directory: %s", evalRoot)
		return &LocalFSToolPack{cfg: cfg}
	}
	cfg.rootAbs = evalRoot

	return &LocalFSToolPack{cfg: cfg}
}

// NewLocalFSTools returns a ToolRegistry with the LocalFSToolPack registered.
func NewLocalFSTools(root string, opts ...LocalFSOption) *ToolRegistry {
	reg := NewToolRegistry()
	NewLocalFSToolPack(root, opts...).RegisterInto(reg)
	return reg
}

// RegisterInto registers fs.* tools into the provided registry.
func (p *LocalFSToolPack) RegisterInto(registry *ToolRegistry) *ToolRegistry {
	if registry == nil {
		return nil
	}
	registry.Register("fs.read_file", p.readFileTool)
	registry.Register("fs.list_files", p.listFilesTool)
	registry.Register("fs.search", p.searchTool)
	return registry
}

type fsReadFileArgs struct {
	Path     string  `json:"path"`
	MaxBytes *uint64 `json:"max_bytes,omitempty"`
}

func (a *fsReadFileArgs) Validate() error {
	if strings.TrimSpace(a.Path) == "" {
		return errors.New("path is required")
	}
	if a.MaxBytes != nil && *a.MaxBytes == 0 {
		return errors.New("max_bytes must be > 0")
	}
	return nil
}

type fsListFilesArgs struct {
	Path       string  `json:"path,omitempty"`
	MaxEntries *uint64 `json:"max_entries,omitempty"`
}

func (a *fsListFilesArgs) Validate() error {
	if a.MaxEntries != nil && *a.MaxEntries == 0 {
		return errors.New("max_entries must be > 0")
	}
	return nil
}

type fsSearchArgs struct {
	Query      string  `json:"query"`
	Path       string  `json:"path,omitempty"`
	MaxMatches *uint64 `json:"max_matches,omitempty"`
}

func (a *fsSearchArgs) Validate() error {
	if strings.TrimSpace(a.Query) == "" {
		return errors.New("query is required")
	}
	if a.MaxMatches != nil && *a.MaxMatches == 0 {
		return errors.New("max_matches must be > 0")
	}
	return nil
}

func (p *LocalFSToolPack) ensureReady() error {
	if p == nil {
		return errors.New("local fs tools: pack is nil")
	}
	if p.cfg.initErr != nil {
		return p.cfg.initErr
	}
	if strings.TrimSpace(p.cfg.rootAbs) == "" {
		return errors.New("local fs tools: root not configured")
	}
	return nil
}

func (p *LocalFSToolPack) cleanRelPath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", &ToolArgsError{Message: "path cannot be empty"}
	}
	if filepath.IsAbs(raw) {
		return "", &ToolArgsError{Message: "path must be workspace-relative (not absolute)"}
	}
	// Accept forward slashes in tool calls regardless of platform.
	rel := filepath.Clean(filepath.FromSlash(raw))
	if rel == "." {
		return rel, nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", &ToolArgsError{Message: "path must not escape the workspace root"}
	}
	return rel, nil
}

func (p *LocalFSToolPack) resolveExistingPath(rel string) (string, string, error) {
	if ensureErr := p.ensureReady(); ensureErr != nil {
		return "", "", ensureErr
	}
	cleanRel, err := p.cleanRelPath(rel)
	if err != nil {
		return "", "", err
	}
	target := filepath.Join(p.cfg.rootAbs, cleanRel)

	// Resolve symlinks before enforcing root containment.
	eval, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", "", fmt.Errorf("local fs tools: resolve path: %w", err)
	}

	within, err := filepath.Rel(p.cfg.rootAbs, eval)
	if err != nil {
		return "", "", fmt.Errorf("local fs tools: resolve relative path: %w", err)
	}
	if within == ".." || strings.HasPrefix(within, ".."+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("local fs tools: path escapes root: %s", rel)
	}

	return eval, filepath.ToSlash(within), nil
}

func (p *LocalFSToolPack) readFileTool(_ map[string]any, call llm.ToolCall) (any, error) {
	if err := p.ensureReady(); err != nil {
		return nil, err
	}

	var args fsReadFileArgs
	if err := ParseAndValidateToolArgs(call, &args); err != nil {
		return nil, err
	}

	maxBytes := p.cfg.maxReadBytes
	if args.MaxBytes != nil {
		if *args.MaxBytes > p.cfg.hardMaxReadBytes {
			return nil, &ToolArgsError{Message: fmt.Sprintf("max_bytes exceeds hard cap (%d)", p.cfg.hardMaxReadBytes)}
		}
		maxBytes = *args.MaxBytes
	}
	if maxBytes == 0 {
		maxBytes = 1
	}

	abs, _, err := p.resolveExistingPath(args.Path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("fs.read_file: stat: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("fs.read_file: path is a directory: %s", args.Path)
	}

	//nolint:gosec // G304: path is sandboxed via resolveExistingPath (root containment + symlink resolution)
	f, err := os.Open(abs)
	if err != nil {
		return nil, fmt.Errorf("fs.read_file: open: %w", err)
	}
	defer func() { _ = f.Close() }()

	if maxBytes > uint64(math.MaxInt64-1) {
		return nil, &ToolArgsError{Message: "max_bytes is too large"}
	}
	limit := int64(maxBytes)
	if limit <= 0 {
		limit = int64(localFSHardMaxReadBytes)
	}

	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("fs.read_file: read: %w", err)
	}
	if uint64(len(data)) > maxBytes {
		return nil, fmt.Errorf("fs.read_file: file exceeds max_bytes (%d)", maxBytes)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("fs.read_file: file is not valid UTF-8: %s", args.Path)
	}
	return string(data), nil
}

func (p *LocalFSToolPack) listFilesTool(_ map[string]any, call llm.ToolCall) (any, error) {
	if err := p.ensureReady(); err != nil {
		return nil, err
	}

	var args fsListFilesArgs
	if err := ParseAndValidateToolArgs(call, &args); err != nil {
		return nil, err
	}

	start := strings.TrimSpace(args.Path)
	if start == "" {
		start = "."
	}

	maxEntries := p.cfg.maxListEntries
	if args.MaxEntries != nil {
		if *args.MaxEntries > p.cfg.hardMaxListEntries {
			return nil, &ToolArgsError{Message: fmt.Sprintf("max_entries exceeds hard cap (%d)", p.cfg.hardMaxListEntries)}
		}
		maxEntries = *args.MaxEntries
	}
	if maxEntries == 0 {
		maxEntries = 1
	}

	dirAbs, _, err := p.resolveExistingPath(start)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(dirAbs)
	if err != nil {
		return nil, fmt.Errorf("fs.list_files: stat: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("fs.list_files: path is not a directory: %s", start)
	}

	var out []string
	walkErr := filepath.WalkDir(dirAbs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if _, ignored := p.cfg.ignoreDirNames[d.Name()]; ignored {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(p.cfg.rootAbs, path)
		if err != nil {
			return err
		}
		if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("fs.list_files: internal error: escaped root")
		}
		out = append(out, filepath.ToSlash(rel))
		if uint64(len(out)) >= maxEntries {
			return localFSStopWalk{}
		}
		return nil
	})
	if walkErr != nil {
		var stop localFSStopWalk
		if !errors.As(walkErr, &stop) {
			return nil, fmt.Errorf("fs.list_files: walk: %w", walkErr)
		}
	}

	return strings.Join(out, "\n"), nil
}

func (p *LocalFSToolPack) searchTool(_ map[string]any, call llm.ToolCall) (any, error) {
	if err := p.ensureReady(); err != nil {
		return nil, err
	}

	var args fsSearchArgs
	if err := ParseAndValidateToolArgs(call, &args); err != nil {
		return nil, err
	}

	start := strings.TrimSpace(args.Path)
	if start == "" {
		start = "."
	}

	maxMatches := p.cfg.maxSearchMatches
	if args.MaxMatches != nil {
		if *args.MaxMatches > p.cfg.hardMaxSearchMatches {
			return nil, &ToolArgsError{Message: fmt.Sprintf("max_matches exceeds hard cap (%d)", p.cfg.hardMaxSearchMatches)}
		}
		maxMatches = *args.MaxMatches
	}
	if maxMatches == 0 {
		maxMatches = 1
	}

	dirAbs, _, err := p.resolveExistingPath(start)
	if err != nil {
		return nil, err
	}

	if rg, ok := p.rgBinary(); ok {
		return p.searchWithRipgrep(rg, args.Query, dirAbs, maxMatches)
	}
	return p.searchWithGo(args.Query, dirAbs, maxMatches)
}

func (p *LocalFSToolPack) rgBinary() (string, bool) {
	p.rgOnce.Do(func() {
		rg, err := exec.LookPath("rg")
		if err != nil {
			return
		}
		p.rgPath = rg
	})
	if strings.TrimSpace(p.rgPath) == "" {
		return "", false
	}
	return p.rgPath, true
}

func (p *LocalFSToolPack) searchWithRipgrep(rgPath, query string, dirAbs string, maxMatches uint64) (any, error) {
	timeout := p.cfg.searchTimeout
	if timeout <= 0 {
		timeout = localFSDefaultSearchTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := []string{
		"--line-number",
		"--no-heading",
		"--color=never",
	}
	for name := range p.cfg.ignoreDirNames {
		// Exclude ignored directories at any depth.
		args = append(args, "--glob", "!**/"+name+"/**")
	}
	args = append(args, query, dirAbs)

	cmd := exec.CommandContext(ctx, rgPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("fs.search: rg stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("fs.search: rg stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("fs.search: rg start: %w", err)
	}

	var stderrBuf bytes.Buffer
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		_, _ = io.Copy(&stderrBuf, io.LimitReader(stderr, 32_000))
	}()

	var lines []string
	stopByCap := false
	sc := bufio.NewScanner(stdout)
	// Allow long match lines, but still bounded.
	sc.Buffer(make([]byte, 16*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		lines = append(lines, p.normalizeRipgrepLine(line))
		if uint64(len(lines)) >= maxMatches {
			stopByCap = true
			cancel()
			break
		}
	}
	// Drain scanner error (if any).
	scanErr := sc.Err()

	waitErr := cmd.Wait()
	<-stderrDone

	if stopByCap {
		// We canceled the process intentionally after reaching cap.
		return strings.Join(lines, "\n"), nil
	}
	if scanErr != nil {
		return nil, fmt.Errorf("fs.search: rg scan: %w", scanErr)
	}
	if waitErr == nil {
		return strings.Join(lines, "\n"), nil
	}

	// ripgrep exit codes: 0 matches, 1 no matches, 2 error.
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		if exitErr.ExitCode() == 1 {
			return "", nil
		}
		stderrText := strings.TrimSpace(stderrBuf.String())
		if exitErr.ExitCode() == 2 && stderrText != "" && strings.Contains(strings.ToLower(stderrText), "regex") {
			return nil, &ToolArgsError{Message: "invalid query regex: " + stderrText}
		}
		if stderrText != "" {
			return nil, fmt.Errorf("fs.search: rg failed: %s", stderrText)
		}
	}
	if errors.Is(waitErr, context.DeadlineExceeded) || errors.Is(waitErr, context.Canceled) {
		return nil, fmt.Errorf("fs.search: timed out after %s", timeout)
	}
	return nil, fmt.Errorf("fs.search: rg failed: %w", waitErr)
}

func (p *LocalFSToolPack) normalizeRipgrepLine(line string) string {
	// ripgrep prints absolute or relative-to-dirAbs paths depending on invocation.
	// Normalize to workspace-relative paths.
	line = strings.TrimSpace(line)
	if line == "" {
		return line
	}
	// Try to rewrite prefix "<absRoot>/<rel>:<line>:" to "<rel>:<line>:"
	if strings.Contains(line, ":") {
		prefix, rest, ok := strings.Cut(line, ":")
		if ok {
			// prefix is file path; rest starts with line number.
			prefixOS := filepath.FromSlash(prefix)
			if !filepath.IsAbs(prefixOS) {
				// prefix could be relative to current dir; just normalize slashes.
				return filepath.ToSlash(prefixOS) + ":" + rest
			}
			rel, err := filepath.Rel(p.cfg.rootAbs, prefixOS)
			if err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".." {
				return filepath.ToSlash(rel) + ":" + rest
			}
		}
	}
	return line
}

func (p *LocalFSToolPack) searchWithGo(query string, dirAbs string, maxMatches uint64) (any, error) {
	re, err := regexp.Compile(query)
	if err != nil {
		return nil, &ToolArgsError{Message: "invalid query regex: " + err.Error()}
	}

	timeout := p.cfg.searchTimeout
	if timeout <= 0 {
		timeout = localFSDefaultSearchTimeout
	}
	deadline := time.Now().Add(timeout)

	var out []string

	walkErr := filepath.WalkDir(dirAbs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return context.DeadlineExceeded
		}
		if d.IsDir() {
			if _, ignored := p.cfg.ignoreDirNames[d.Name()]; ignored {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			// Skip symlinks to avoid ambiguous containment.
			return nil
		}

		rel, err := filepath.Rel(p.cfg.rootAbs, path)
		if err != nil {
			return err
		}
		if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("fs.search: internal error: escaped root")
		}

		// Best-effort binary skip: only scan UTF-8 text up to cap.
		//nolint:gosec // G304: walk is sandboxed to the resolved root; we never follow symlinks.
		f, openErr := os.Open(path)
		if openErr != nil {
			return nil //nolint:nilerr // best-effort search: unreadable files are skipped
		}
		if p.cfg.maxSearchBytes > uint64(math.MaxInt64) {
			_ = f.Close()
			return fmt.Errorf("fs.search: maxSearchBytes too large")
		}
		limit := int64(p.cfg.maxSearchBytes) //nolint:gosec // G115: bounded by check above
		if limit <= 0 {
			limit = int64(localFSDefaultMaxSearchBytes)
		}
		r := io.LimitReader(f, limit)

		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 16*1024), 1024*1024)
		lineNo := 0
		for sc.Scan() {
			lineNo++
			b := sc.Bytes()
			if !utf8.Valid(b) {
				// Treat as binary; stop scanning this file.
				_ = f.Close()
				return nil
			}
			if re.Match(b) {
				out = append(out, fmt.Sprintf("%s:%d:%s", filepath.ToSlash(rel), lineNo, string(bytes.TrimRight(b, "\r\n"))))
				if uint64(len(out)) >= maxMatches {
					_ = f.Close()
					return localFSStopWalk{}
				}
			}
			if time.Now().After(deadline) {
				_ = f.Close()
				return context.DeadlineExceeded
			}
		}
		_ = f.Close()
		return nil
	})

	if walkErr != nil {
		var stop localFSStopWalk
		if errors.As(walkErr, &stop) {
			return strings.Join(out, "\n"), nil
		}
		if errors.Is(walkErr, context.DeadlineExceeded) {
			return nil, fmt.Errorf("fs.search: timed out after %s", timeout)
		}
		return nil, fmt.Errorf("fs.search: walk: %w", walkErr)
	}

	return strings.Join(out, "\n"), nil
}

func defaultIgnoredDirNames() map[string]struct{} {
	ignored := []string{
		".git",
		"node_modules",
		"vendor",
		"dist",
		"build",
		".next",
		"target",
		".idea",
		".vscode",
	}
	out := make(map[string]struct{}, len(ignored))
	for _, n := range ignored {
		out[n] = struct{}{}
	}
	return out
}
