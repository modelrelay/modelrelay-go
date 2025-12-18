package sdk

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

const (
	localWriteFileDefaultMaxBytes uint64 = 64_000
	localWriteFileHardMaxBytes    uint64 = 1_000_000
	localWriteFileDefaultFileMode        = 0o600
	localWriteFileDefaultDirMode         = 0o750
)

type LocalWriteFileOption func(*localWriteFileConfig)

type localWriteFileConfig struct {
	rootAbs string
	initErr error

	allowWrite bool

	createDirs bool
	atomic     bool

	maxBytes     uint64
	hardMaxBytes uint64

	fileMode os.FileMode
	dirMode  os.FileMode
}

// WithLocalWriteFileAllow enables the `write_file` tool (otherwise deny-all).
func WithLocalWriteFileAllow() LocalWriteFileOption {
	return func(c *localWriteFileConfig) { c.allowWrite = true }
}

// WithLocalWriteFileCreateDirs controls whether missing parent directories are created.
func WithLocalWriteFileCreateDirs(enabled bool) LocalWriteFileOption {
	return func(c *localWriteFileConfig) { c.createDirs = enabled }
}

// WithLocalWriteFileAtomic controls whether writes are atomic (write temp + rename).
func WithLocalWriteFileAtomic(enabled bool) LocalWriteFileOption {
	return func(c *localWriteFileConfig) { c.atomic = enabled }
}

// WithLocalWriteFileMaxBytes sets the default max bytes allowed for contents.
func WithLocalWriteFileMaxBytes(n uint64) LocalWriteFileOption {
	return func(c *localWriteFileConfig) { c.maxBytes = n }
}

// WithLocalWriteFileHardMaxBytes sets the hard cap for contents size.
func WithLocalWriteFileHardMaxBytes(n uint64) LocalWriteFileOption {
	return func(c *localWriteFileConfig) { c.hardMaxBytes = n }
}

// WithLocalWriteFileFileMode sets the permissions for newly created files.
func WithLocalWriteFileFileMode(mode os.FileMode) LocalWriteFileOption {
	return func(c *localWriteFileConfig) { c.fileMode = mode }
}

// WithLocalWriteFileDirMode sets the permissions for newly created directories.
func WithLocalWriteFileDirMode(mode os.FileMode) LocalWriteFileOption {
	return func(c *localWriteFileConfig) { c.dirMode = mode }
}

// LocalWriteFileToolPack provides an opt-in implementation of the tools.v0 `write_file` tool.
//
// Safety properties:
// - Deny-all by default (must enable explicitly via WithLocalWriteFileAllow).
// - Enforces a root sandbox and path traversal prevention.
// - Rejects symlinks in any path component to avoid escapes.
// - Enforces max contents size with a hard cap.
// - Supports optional atomic writes and directory creation policy.
type LocalWriteFileToolPack struct {
	cfg localWriteFileConfig
}

// NewLocalWriteFileToolPack creates a LocalWriteFileToolPack sandboxed to the given root directory.
//
// If root is invalid, tools will return an error at execution time (fail fast).
func NewLocalWriteFileToolPack(root string, opts ...LocalWriteFileOption) *LocalWriteFileToolPack {
	cfg := localWriteFileConfig{
		createDirs: false,
		atomic:     true,

		maxBytes:     localWriteFileDefaultMaxBytes,
		hardMaxBytes: localWriteFileHardMaxBytes,

		fileMode: localWriteFileDefaultFileMode,
		dirMode:  localWriteFileDefaultDirMode,
	}

	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	root = strings.TrimSpace(root)
	if root == "" {
		cfg.initErr = errors.New("local write_file tool: root directory required")
		return &LocalWriteFileToolPack{cfg: cfg}
	}
	if cfg.hardMaxBytes == 0 || cfg.maxBytes == 0 {
		cfg.initErr = errors.New("local write_file tool: max bytes must be > 0")
		return &LocalWriteFileToolPack{cfg: cfg}
	}
	if cfg.maxBytes > cfg.hardMaxBytes {
		cfg.initErr = errors.New("local write_file tool: max bytes exceeds hard cap")
		return &LocalWriteFileToolPack{cfg: cfg}
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		cfg.initErr = fmt.Errorf("local write_file tool: resolve root: %w", err)
		return &LocalWriteFileToolPack{cfg: cfg}
	}
	info, err := os.Stat(abs)
	if err != nil {
		cfg.initErr = fmt.Errorf("local write_file tool: stat root: %w", err)
		return &LocalWriteFileToolPack{cfg: cfg}
	}
	if !info.IsDir() {
		cfg.initErr = fmt.Errorf("local write_file tool: root is not a directory: %s", abs)
		return &LocalWriteFileToolPack{cfg: cfg}
	}
	evalRoot, err := filepath.EvalSymlinks(abs)
	if err != nil {
		cfg.initErr = fmt.Errorf("local write_file tool: resolve root symlinks: %w", err)
		return &LocalWriteFileToolPack{cfg: cfg}
	}
	info, err = os.Stat(evalRoot)
	if err != nil {
		cfg.initErr = fmt.Errorf("local write_file tool: stat resolved root: %w", err)
		return &LocalWriteFileToolPack{cfg: cfg}
	}
	if !info.IsDir() {
		cfg.initErr = fmt.Errorf("local write_file tool: resolved root is not a directory: %s", evalRoot)
		return &LocalWriteFileToolPack{cfg: cfg}
	}
	cfg.rootAbs = evalRoot

	return &LocalWriteFileToolPack{cfg: cfg}
}

// NewLocalWriteFileTools returns a ToolRegistry with the LocalWriteFileToolPack registered.
func NewLocalWriteFileTools(root string, opts ...LocalWriteFileOption) *ToolRegistry {
	reg := NewToolRegistry()
	NewLocalWriteFileToolPack(root, opts...).RegisterInto(reg)
	return reg
}

// RegisterInto registers `write_file` into the provided registry.
func (p *LocalWriteFileToolPack) RegisterInto(registry *ToolRegistry) *ToolRegistry {
	if registry == nil {
		return nil
	}
	registry.Register(ToolNameWriteFile, p.writeFileTool)
	return registry
}

type writeFileArgs struct {
	Path     string  `json:"path"`
	Contents *string `json:"contents"`
}

func (a *writeFileArgs) Validate() error {
	path := strings.TrimSpace(a.Path)
	if path == "" {
		return errors.New("path is required")
	}
	if strings.Contains(path, "\x00") {
		return errors.New("path contains NUL byte")
	}
	if filepath.IsAbs(path) {
		return errors.New("path must be relative")
	}
	if a.Contents == nil {
		return errors.New("contents is required")
	}
	return nil
}

func (p *LocalWriteFileToolPack) ensureReady() error {
	if p == nil {
		return errors.New("local write_file tool: pack is nil")
	}
	if p.cfg.initErr != nil {
		return p.cfg.initErr
	}
	if strings.TrimSpace(p.cfg.rootAbs) == "" {
		return errors.New("local write_file tool: missing root")
	}
	if !p.cfg.allowWrite {
		return errors.New("write_file tool disabled by default: configure WithLocalWriteFileAllow")
	}
	return nil
}

func (p *LocalWriteFileToolPack) writeFileTool(_ map[string]any, call llm.ToolCall) (any, error) {
	if err := p.ensureReady(); err != nil {
		return nil, err
	}

	var args writeFileArgs
	if err := ParseAndValidateToolArgs(call, &args); err != nil {
		return nil, err
	}

	contents := *args.Contents
	contentBytes := uint64(len(contents))
	if contentBytes > p.cfg.hardMaxBytes {
		return nil, &ToolArgsError{
			Message:      fmt.Sprintf("contents exceeds hard cap (%d bytes)", p.cfg.hardMaxBytes),
			ToolCallID:   call.ID,
			ToolName:     toolNameFromToolCall(call),
			RawArguments: rawArgsFromToolCall(call),
		}
	}
	if contentBytes > p.cfg.maxBytes {
		return nil, &ToolArgsError{
			Message:      fmt.Sprintf("contents exceeds max size (%d bytes)", p.cfg.maxBytes),
			ToolCallID:   call.ID,
			ToolName:     toolNameFromToolCall(call),
			RawArguments: rawArgsFromToolCall(call),
		}
	}

	relPath := filepath.Clean(strings.TrimSpace(args.Path))
	if relPath == "." || relPath == string(filepath.Separator) {
		return nil, &ToolArgsError{
			Message:      "path must be a file path, not a directory",
			ToolCallID:   call.ID,
			ToolName:     toolNameFromToolCall(call),
			RawArguments: rawArgsFromToolCall(call),
		}
	}
	if strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || relPath == ".." {
		return nil, &ToolArgsError{
			Message:      "path traversal is not allowed",
			ToolCallID:   call.ID,
			ToolName:     toolNameFromToolCall(call),
			RawArguments: rawArgsFromToolCall(call),
		}
	}

	full := filepath.Join(p.cfg.rootAbs, relPath)
	if err := p.ensureNoSymlinksInParents(relPath, call); err != nil {
		return nil, err
	}

	if err := p.ensureParentDir(full, call); err != nil {
		return nil, err
	}
	if err := p.ensureTargetNotSymlink(full, call); err != nil {
		return nil, err
	}

	if p.cfg.atomic {
		if err := p.atomicWriteFile(full, []byte(contents)); err != nil {
			return nil, err
		}
	} else {
		if err := os.WriteFile(full, []byte(contents), p.cfg.fileMode); err != nil {
			return nil, err
		}
	}

	return map[string]any{"written": relPath, "bytes": contentBytes}, nil
}

func toolNameFromToolCall(call llm.ToolCall) ToolName {
	if call.Function == nil {
		return ""
	}
	return call.Function.Name
}

func rawArgsFromToolCall(call llm.ToolCall) string {
	if call.Function == nil {
		return ""
	}
	return call.Function.Arguments
}

func (p *LocalWriteFileToolPack) ensureParentDir(fullPath string, call llm.ToolCall) error {
	parent := filepath.Dir(fullPath)
	parentInfo, err := os.Stat(parent)
	if err == nil {
		if !parentInfo.IsDir() {
			return fmt.Errorf("write_file: parent path is not a directory: %s", parent)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !p.cfg.createDirs {
		return &ToolArgsError{
			Message:      "parent directory does not exist (directory creation disabled)",
			ToolCallID:   call.ID,
			ToolName:     toolNameFromToolCall(call),
			RawArguments: rawArgsFromToolCall(call),
		}
	}
	if err := os.MkdirAll(parent, p.cfg.dirMode); err != nil {
		return err
	}
	return nil
}

func (p *LocalWriteFileToolPack) ensureTargetNotSymlink(fullPath string, call llm.ToolCall) error {
	info, err := os.Lstat(fullPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return &ToolArgsError{
			Message:      "refusing to write to symlink",
			ToolCallID:   call.ID,
			ToolName:     toolNameFromToolCall(call),
			RawArguments: rawArgsFromToolCall(call),
		}
	}
	if info.IsDir() {
		return &ToolArgsError{
			Message:      "path is a directory",
			ToolCallID:   call.ID,
			ToolName:     toolNameFromToolCall(call),
			RawArguments: rawArgsFromToolCall(call),
		}
	}
	return nil
}

func (p *LocalWriteFileToolPack) ensureNoSymlinksInParents(relPath string, call llm.ToolCall) error {
	dir := filepath.Dir(relPath)
	if dir == "." {
		return nil
	}
	parts := splitPath(dir)
	cur := p.cfg.rootAbs
	for _, part := range parts {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// Directory doesn't exist yet; MkdirAll will create a real directory here.
				continue
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return &ToolArgsError{
				Message:      "symlinks in path are not allowed",
				ToolCallID:   call.ID,
				ToolName:     toolNameFromToolCall(call),
				RawArguments: rawArgsFromToolCall(call),
			}
		}
	}
	return nil
}

func (p *LocalWriteFileToolPack) atomicWriteFile(fullPath string, data []byte) error {
	dir := filepath.Dir(fullPath)
	tmp, err := os.CreateTemp(dir, ".modelrelay-write-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	if err := tmp.Chmod(p.cfg.fileMode); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// Rename is atomic on POSIX when within the same filesystem.
	return os.Rename(tmpName, fullPath)
}

func splitPath(p string) []string {
	p = filepath.Clean(p)
	if p == "." || p == "" {
		return nil
	}
	parts := strings.Split(p, string(filepath.Separator))
	var out []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		out = append(out, part)
	}
	return out
}
