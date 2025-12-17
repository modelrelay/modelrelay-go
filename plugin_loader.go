package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

type PluginLoaderOption func(*PluginLoader)

// WithPluginLoaderHTTPClient overrides the HTTP client used for GitHub requests.
func WithPluginLoaderHTTPClient(c *http.Client) PluginLoaderOption {
	return func(l *PluginLoader) {
		if c != nil {
			l.httpClient = c
		}
	}
}

// WithPluginLoaderAPIBaseURL overrides the GitHub API base URL (default https://api.github.com).
func WithPluginLoaderAPIBaseURL(base string) PluginLoaderOption {
	return func(l *PluginLoader) {
		base = strings.TrimSpace(base)
		if base != "" {
			l.apiBaseURL = strings.TrimRight(base, "/")
		}
	}
}

// WithPluginLoaderRawBaseURL overrides the GitHub raw base URL (default https://raw.githubusercontent.com).
func WithPluginLoaderRawBaseURL(base string) PluginLoaderOption {
	return func(l *PluginLoader) {
		base = strings.TrimSpace(base)
		if base != "" {
			l.rawBaseURL = strings.TrimRight(base, "/")
		}
	}
}

// WithPluginLoaderCacheTTL overrides the in-memory cache TTL (default 5 minutes).
func WithPluginLoaderCacheTTL(ttl time.Duration) PluginLoaderOption {
	return func(l *PluginLoader) {
		if ttl > 0 {
			l.cacheTTL = ttl
		}
	}
}

// WithPluginLoaderNow overrides the time source (primarily for tests).
func WithPluginLoaderNow(now func() time.Time) PluginLoaderOption {
	return func(l *PluginLoader) {
		if now != nil {
			l.now = now
		}
	}
}

// PluginLoader loads ModelRelay plugins from GitHub.
//
// It normalizes multiple GitHub URL formats to a canonical form and fetches:
// - PLUGIN.md
// - commands/*.md
// - agents/*.md
//
// Files are fetched from GitHub raw URLs; directory listings use the GitHub contents API.
type PluginLoader struct {
	httpClient *http.Client
	apiBaseURL string
	rawBaseURL string

	cacheTTL time.Duration
	now      func() time.Time

	mu    sync.Mutex
	cache map[string]pluginLoaderCacheEntry
}

type pluginLoaderCacheEntry struct {
	expiresAt time.Time
	plugin    Plugin
}

type pluginHTTPError struct {
	StatusCode int
	Message    string
}

func (e *pluginHTTPError) Error() string {
	if e == nil {
		return "http error"
	}
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		return fmt.Sprintf("http error (%d)", e.StatusCode)
	}
	return msg
}

func NewPluginLoader(opts ...PluginLoaderOption) *PluginLoader {
	l := &PluginLoader{
		httpClient: http.DefaultClient,
		apiBaseURL: "https://api.github.com",
		rawBaseURL: "https://raw.githubusercontent.com",
		cacheTTL:   5 * time.Minute,
		now:        time.Now,
		cache:      make(map[string]pluginLoaderCacheEntry),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(l)
		}
	}
	return l
}

func (l *PluginLoader) Load(ctx context.Context, sourceURL string) (*Plugin, error) {
	if l == nil {
		return nil, errors.New("plugin loader: not initialized")
	}
	ref, err := parseGitHubPluginRef(sourceURL)
	if err != nil {
		return nil, err
	}
	key := ref.canonical()
	if cached, ok := l.cached(key); ok {
		return cached, nil
	}

	pluginRoot := strings.Trim(path.Clean(strings.TrimSpace(ref.repoPath)), "/")
	if pluginRoot == "." {
		pluginRoot = ""
	}

	manifestCandidates := []string{"PLUGIN.md", "SKILL.md"}
	var manifestPath string
	var manifestMD string
	for i, name := range manifestCandidates {
		p := joinRepoPath(pluginRoot, name)
		body, fetchErr := l.getText(ctx, l.rawURL(ref, p))
		if fetchErr != nil {
			var herr *pluginHTTPError
			if errors.As(fetchErr, &herr) && herr.StatusCode == http.StatusNotFound && i < len(manifestCandidates)-1 {
				continue
			}
			return nil, fmt.Errorf("fetch %s: %w", p, fetchErr)
		}
		manifestPath = p
		manifestMD = body
		break
	}
	if strings.TrimSpace(manifestPath) == "" {
		return nil, errors.New("plugin manifest not found")
	}

	commandsDir := joinRepoPath(pluginRoot, "commands")
	agentsDir := joinRepoPath(pluginRoot, "agents")

	commandFiles, err := l.listMarkdownFiles(ctx, ref, commandsDir)
	if err != nil {
		return nil, err
	}
	agentFiles, err := l.listMarkdownFiles(ctx, ref, agentsDir)
	if err != nil {
		return nil, err
	}

	out := Plugin{
		ID:       PluginID(derivePluginID(ref.owner, ref.repo, pluginRoot)),
		URL:      PluginURL(ref.canonical()),
		Manifest: parsePluginManifest(manifestMD),
		Commands: make(map[PluginCommandName]PluginCommand),
		Agents:   make(map[PluginAgentName]PluginAgent),
		RawFiles: make(map[PluginRepoPath]string),
		Ref: PluginGitHubRef{
			Owner: GitHubOwner(ref.owner),
			Repo:  GitHubRepo(ref.repo),
			Ref:   GitHubRef(ref.ref),
			Path:  GitHubPath(pluginRoot),
		},
		LoadedAt: l.now().UTC(),
	}
	out.RawFiles[PluginRepoPath(manifestPath)] = manifestMD

	for _, filePath := range commandFiles {
		body, err := l.getText(ctx, l.rawURL(ref, filePath))
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", filePath, err)
		}
		out.RawFiles[PluginRepoPath(filePath)] = body
		name := PluginCommandName(strings.TrimSuffix(path.Base(filePath), ".md"))
		out.Commands[name] = PluginCommand{Name: name, Prompt: body, AgentRefs: extractAgentRefs(body)}
	}
	for _, filePath := range agentFiles {
		body, err := l.getText(ctx, l.rawURL(ref, filePath))
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", filePath, err)
		}
		out.RawFiles[PluginRepoPath(filePath)] = body
		name := PluginAgentName(strings.TrimSuffix(path.Base(filePath), ".md"))
		out.Agents[name] = PluginAgent{Name: name, SystemPrompt: body}
	}

	out.Manifest.Commands = sortedKeys(out.Commands)
	out.Manifest.Agents = sortedKeys(out.Agents)

	l.store(key, out)
	clone := clonePlugin(out)
	return &clone, nil
}

func (l *PluginLoader) cached(key string) (*Plugin, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ent, ok := l.cache[key]
	if !ok {
		return nil, false
	}
	if l.now().After(ent.expiresAt) {
		delete(l.cache, key)
		return nil, false
	}
	clone := clonePlugin(ent.plugin)
	return &clone, true
}

func (l *PluginLoader) store(key string, plugin Plugin) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cache[key] = pluginLoaderCacheEntry{
		expiresAt: l.now().Add(l.cacheTTL),
		plugin:    clonePlugin(plugin),
	}
}

func (l *PluginLoader) rawURL(ref gitHubPluginRef, repoPath string) string {
	repoPath = strings.TrimLeft(path.Clean("/"+repoPath), "/")
	return strings.TrimSuffix(l.rawBaseURL, "/") + "/" + ref.owner + "/" + ref.repo + "/" + ref.ref + "/" + repoPath
}

type gitHubContentEntry struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Path string `json:"path"`
}

func (l *PluginLoader) listMarkdownFiles(ctx context.Context, ref gitHubPluginRef, repoDir string) ([]string, error) {
	repoDir = strings.TrimLeft(path.Clean("/"+repoDir), "/")
	apiPath := "/repos/" + ref.owner + "/" + ref.repo + "/contents/" + repoDir

	u, err := url.Parse(strings.TrimSuffix(l.apiBaseURL, "/") + apiPath)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("ref", ref.ref)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github contents: %s (%d)", strings.TrimSpace(string(b)), resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var entries []gitHubContentEntry
	if len(body) > 0 && body[0] == '{' {
		var single gitHubContentEntry
		if err := json.Unmarshal(body, &single); err != nil {
			return nil, err
		}
		entries = []gitHubContentEntry{single}
	} else {
		if err := json.Unmarshal(body, &entries); err != nil {
			return nil, err
		}
	}

	var out []string
	for _, e := range entries {
		if e.Type != "file" {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name), ".md") {
			continue
		}
		if strings.TrimSpace(e.Path) == "" {
			continue
		}
		out = append(out, path.Clean(e.Path))
	}
	sort.Strings(out)
	return out, nil
}

func (l *PluginLoader) getText(ctx context.Context, targetURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, http.NoBody)
	if err != nil {
		return "", err
	}
	resp, err := l.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	//nolint:errcheck // best-effort cleanup on return
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(b))
		if msg == "" {
			msg = resp.Status
		}
		return "", &pluginHTTPError{StatusCode: resp.StatusCode, Message: msg}
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func parsePluginManifest(md string) PluginManifest {
	md = strings.TrimSpace(md)
	if md == "" {
		return PluginManifest{}
	}
	if strings.HasPrefix(md, "---\n") || strings.HasPrefix(md, "---\r\n") {
		if mf, ok := parseFrontMatter(md); ok {
			return mf
		}
	}

	lines := splitLines(md)
	var name string
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "# ") {
			name = strings.TrimSpace(strings.TrimPrefix(ln, "# "))
			break
		}
	}
	desc := ""
	if name != "" {
		after := false
		for _, ln := range lines {
			trim := strings.TrimSpace(ln)
			if strings.HasPrefix(trim, "# ") {
				after = true
				continue
			}
			if !after {
				continue
			}
			if trim == "" {
				continue
			}
			if strings.HasPrefix(trim, "## ") {
				break
			}
			desc = trim
			break
		}
	}
	return PluginManifest{Name: name, Description: desc}
}

func parseFrontMatter(md string) (PluginManifest, bool) {
	lines := splitLines(md)
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return PluginManifest{}, false
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return PluginManifest{}, false
	}
	fm := lines[1:end]
	var out PluginManifest
	var currentListKind string
	for _, ln := range fm {
		raw := strings.TrimSpace(ln)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		if strings.HasPrefix(raw, "- ") && currentListKind != "" {
			item := strings.TrimSpace(strings.TrimPrefix(raw, "- "))
			if item != "" {
				switch currentListKind {
				case "commands":
					out.Commands = append(out.Commands, PluginCommandName(item))
				case "agents":
					out.Agents = append(out.Agents, PluginAgentName(item))
				}
			}
			continue
		}
		currentListKind = ""
		parts := strings.SplitN(raw, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)
		switch key {
		case "name":
			out.Name = val
		case "description":
			out.Description = val
		case "version":
			out.Version = val
		case "commands":
			currentListKind = "commands"
		case "agents":
			currentListKind = "agents"
		}
	}
	sort.Slice(out.Commands, func(i, j int) bool { return out.Commands[i] < out.Commands[j] })
	sort.Slice(out.Agents, func(i, j int) bool { return out.Agents[i] < out.Agents[j] })
	return out, true
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}

func joinRepoPath(base, elem string) string {
	base = strings.Trim(path.Clean(strings.TrimSpace(base)), "/")
	elem = strings.Trim(path.Clean(strings.TrimSpace(elem)), "/")
	if base == "" || base == "." {
		return elem
	}
	if elem == "" || elem == "." {
		return base
	}
	return base + "/" + elem
}

func derivePluginID(owner, repo, pluginRoot string) string {
	base := owner + "/" + repo
	pluginRoot = strings.Trim(path.Clean(strings.TrimSpace(pluginRoot)), "/")
	if pluginRoot == "" || pluginRoot == "." {
		return base
	}
	return base + "/" + pluginRoot
}

func sortedKeys[K ~string, V any](m map[K]V) []K {
	if len(m) == 0 {
		return nil
	}
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// extractAgentRefs attempts to extract referenced agent names from a command markdown file.
func extractAgentRefs(markdown string) []PluginAgentName {
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return nil
	}
	seen := map[PluginAgentName]struct{}{}
	var out []PluginAgentName
	add := func(name PluginAgentName) {
		if !name.Valid() {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	for _, line := range splitLines(markdown) {
		l := strings.ToLower(line)
		if idx := strings.Index(l, "agents/"); idx >= 0 && strings.Contains(l[idx:], ".md") {
			seg := line[idx:]
			seg = strings.TrimSpace(seg)
			seg = strings.TrimPrefix(seg, "agents/")
			seg = strings.SplitN(seg, ".md", 2)[0]
			seg = strings.Trim(seg, "`* _")
			add(PluginAgentName(seg))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

type gitHubPluginRef struct {
	owner    string
	repo     string
	ref      string
	repoPath string
}

func (r gitHubPluginRef) canonical() string {
	root := strings.Trim(path.Clean(strings.TrimSpace(r.repoPath)), "/")
	if root == "." {
		root = ""
	}
	if root == "" {
		return "github.com/" + r.owner + "/" + r.repo + "@" + r.ref
	}
	return "github.com/" + r.owner + "/" + r.repo + "@" + r.ref + "/" + root
}

func parseGitHubPluginRef(raw string) (gitHubPluginRef, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return gitHubPluginRef{}, errors.New("source_url required")
	}
	// Allow SSH-style GitHub URLs (git@github.com:owner/repo.git).
	if strings.HasPrefix(raw, "git@github.com:") {
		raw = "https://github.com/" + strings.TrimPrefix(raw, "git@github.com:")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return gitHubPluginRef{}, fmt.Errorf("invalid github url: %w", err)
	}
	host := strings.ToLower(strings.TrimSpace(u.Host))
	host = strings.TrimPrefix(host, "www.")
	switch host {
	case "github.com", "raw.githubusercontent.com":
	default:
		return gitHubPluginRef{}, errors.New("unsupported host: " + u.Host)
	}

	ref := strings.TrimSpace(u.Query().Get("ref"))
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return gitHubPluginRef{}, errors.New("invalid github url: expected /owner/repo")
	}
	owner := parts[0]
	repoPart := strings.TrimSuffix(parts[1], ".git")
	if idx := strings.Index(repoPart, "@"); idx > 0 && idx < len(repoPart)-1 {
		if ref == "" {
			ref = repoPart[idx+1:]
		}
		repoPart = repoPart[:idx]
	}
	repo := repoPart
	rest := parts[2:]

	if host == "github.com" && len(rest) >= 2 && (rest[0] == "tree" || rest[0] == "blob") {
		if ref == "" {
			ref = rest[1]
		}
		rest = rest[2:]
	}
	if host == "raw.githubusercontent.com" {
		if len(rest) < 1 {
			return gitHubPluginRef{}, errors.New("invalid raw github url")
		}
		if ref == "" {
			ref = rest[0]
		}
		rest = rest[1:]
	}
	repoPath := strings.Join(rest, "/")
	repoPath = strings.Trim(path.Clean(strings.TrimSpace(repoPath)), "/")
	if repoPath == "." {
		repoPath = ""
	}
	if strings.HasSuffix(strings.ToLower(repoPath), "plugin.md") || strings.HasSuffix(strings.ToLower(repoPath), "skill.md") {
		repoPath = strings.Trim(path.Dir(repoPath), "/")
	}
	if strings.HasSuffix(strings.ToLower(repoPath), ".md") {
		if idx := strings.Index(repoPath, "/commands/"); idx >= 0 {
			repoPath = strings.Trim(repoPath[:idx], "/")
		}
		if idx := strings.Index(repoPath, "/agents/"); idx >= 0 {
			repoPath = strings.Trim(repoPath[:idx], "/")
		}
	}
	if strings.TrimSpace(ref) == "" {
		ref = "HEAD"
	}
	return gitHubPluginRef{owner: owner, repo: repo, ref: ref, repoPath: repoPath}, nil
}

func clonePlugin(p Plugin) Plugin {
	out := p
	out.Commands = make(map[PluginCommandName]PluginCommand, len(p.Commands))
	for k, v := range p.Commands {
		out.Commands[k] = clonePluginCommand(v)
	}
	out.Agents = make(map[PluginAgentName]PluginAgent, len(p.Agents))
	for k, v := range p.Agents {
		out.Agents[k] = v
	}
	out.RawFiles = make(map[PluginRepoPath]string, len(p.RawFiles))
	for k, v := range p.RawFiles {
		out.RawFiles[k] = v
	}
	out.Manifest = clonePluginManifest(p.Manifest)
	return out
}

func clonePluginCommand(c PluginCommand) PluginCommand {
	out := c
	if c.AgentRefs != nil {
		out.AgentRefs = append([]PluginAgentName(nil), c.AgentRefs...)
	}
	return out
}

func clonePluginManifest(m PluginManifest) PluginManifest {
	out := m
	if m.Commands != nil {
		out.Commands = append([]PluginCommandName(nil), m.Commands...)
	}
	if m.Agents != nil {
		out.Agents = append([]PluginAgentName(nil), m.Agents...)
	}
	return out
}
