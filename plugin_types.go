package sdk

import (
	"errors"
	"strings"
	"time"
)

// PluginID is a stable plugin identifier (owner/repo/path).
type PluginID string

func (id PluginID) String() string { return string(id) }

func (id PluginID) Valid() bool { return strings.TrimSpace(string(id)) != "" }

func (id PluginID) Validate() error {
	if !id.Valid() {
		return errors.New("plugin id required")
	}
	return nil
}

// PluginURL is a canonical plugin URL (github.com/owner/repo@ref/path).
type PluginURL string

func (u PluginURL) String() string { return string(u) }

func (u PluginURL) Valid() bool { return strings.TrimSpace(string(u)) != "" }

func (u PluginURL) Validate() error {
	if !u.Valid() {
		return errors.New("plugin url required")
	}
	return nil
}

type GitHubOwner string

func (o GitHubOwner) String() string { return string(o) }

func (o GitHubOwner) Valid() bool { return strings.TrimSpace(string(o)) != "" }

func (o GitHubOwner) Validate() error {
	if !o.Valid() {
		return errors.New("github owner required")
	}
	return nil
}

type GitHubRepo string

func (r GitHubRepo) String() string { return string(r) }

func (r GitHubRepo) Valid() bool { return strings.TrimSpace(string(r)) != "" }

func (r GitHubRepo) Validate() error {
	if !r.Valid() {
		return errors.New("github repo required")
	}
	return nil
}

// GitHubRef is a git reference name (branch, tag) or commit sha.
type GitHubRef string

func (r GitHubRef) String() string { return string(r) }

func (r GitHubRef) Valid() bool { return strings.TrimSpace(string(r)) != "" }

func (r GitHubRef) Validate() error {
	if !r.Valid() {
		return errors.New("github ref required")
	}
	return nil
}

// GitHubPath is the plugin root path within the repository (may be empty for repo root).
type GitHubPath string

func (p GitHubPath) String() string { return string(p) }

// PluginRepoPath is a repo-relative file path stored in Plugin.RawFiles.
type PluginRepoPath string

func (p PluginRepoPath) String() string { return string(p) }

type PluginCommandName string

func (n PluginCommandName) String() string { return string(n) }

func (n PluginCommandName) Valid() bool { return strings.TrimSpace(string(n)) != "" }

func (n PluginCommandName) Validate() error {
	if !n.Valid() {
		return errors.New("plugin command name required")
	}
	return nil
}

type PluginAgentName string

func (n PluginAgentName) String() string { return string(n) }

func (n PluginAgentName) Valid() bool { return strings.TrimSpace(string(n)) != "" }

func (n PluginAgentName) Validate() error {
	if !n.Valid() {
		return errors.New("plugin agent name required")
	}
	return nil
}

// Plugin is a loaded plugin with manifest, commands, agents, and raw source files.
type Plugin struct {
	ID       PluginID                            `json:"id"`
	URL      PluginURL                           `json:"url"`
	Manifest PluginManifest                      `json:"manifest"`
	Commands map[PluginCommandName]PluginCommand `json:"commands"`
	Agents   map[PluginAgentName]PluginAgent     `json:"agents"`
	RawFiles map[PluginRepoPath]string           `json:"raw_files"`

	// Ref describes the resolved GitHub reference for this plugin load.
	Ref PluginGitHubRef `json:"ref"`

	// LoadedAt is when the plugin was loaded (useful for cache introspection).
	LoadedAt time.Time `json:"loaded_at"`
}

// PluginGitHubRef is the resolved GitHub ref and path for a loaded plugin.
type PluginGitHubRef struct {
	Owner GitHubOwner `json:"owner"`
	Repo  GitHubRepo  `json:"repo"`
	Ref   GitHubRef   `json:"ref"`
	Path  GitHubPath  `json:"path,omitempty"`
}

// PluginManifest is metadata parsed from PLUGIN.md.
type PluginManifest struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Version     string              `json:"version"`
	Commands    []PluginCommandName `json:"commands"`
	Agents      []PluginAgentName   `json:"agents"`
}

// PluginCommand is a command definition from commands/*.md.
type PluginCommand struct {
	Name      PluginCommandName `json:"name"`
	Prompt    string            `json:"prompt"`
	AgentRefs []PluginAgentName `json:"agent_refs,omitempty"`
}

// PluginAgent is an agent definition from agents/*.md.
type PluginAgent struct {
	Name         PluginAgentName `json:"name"`
	SystemPrompt string          `json:"system_prompt"`
}
