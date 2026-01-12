package sdk

import (
	"strings"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

// ToolName is the identifier for function tools (client tools in tools.v0).
type ToolName = llm.ToolName

// ToolCallID correlates a tool call with its tool result.
type ToolCallID = llm.ToolCallID

func ParseToolName(raw string) (ToolName, error) { return llm.ParseToolName(raw) }

func ParseToolCallID(raw string) (ToolCallID, error) { return llm.ParseToolCallID(raw) }

// tools.v0 reserved tool names.
const (
	ToolNameFSReadFile  ToolName = "fs.read_file"
	ToolNameFSListFiles ToolName = "fs.list_files"
	ToolNameFSSearch    ToolName = "fs.search"
	ToolNameFSEdit      ToolName = "fs.edit"
	ToolNameBash        ToolName = "bash"
	ToolNameWriteFile   ToolName = "write_file"
	ToolNameUserAsk     ToolName = "user.ask"
)

// AllowedToolNames is the canonical list of allowed tools.v0 client tool names.
var AllowedToolNames = []ToolName{
	ToolNameFSReadFile,
	ToolNameFSListFiles,
	ToolNameFSSearch,
	ToolNameFSEdit,
	ToolNameBash,
	ToolNameWriteFile,
	ToolNameUserAsk,
}

// AllowedToolNamesString returns a comma-separated string of allowed tool names.
func AllowedToolNamesString() string {
	names := make([]string, len(AllowedToolNames))
	for i, n := range AllowedToolNames {
		names[i] = string(n)
	}
	return strings.Join(names, ", ")
}

// AllowedToolNamesSet returns a set of allowed tool names for fast lookup.
func AllowedToolNamesSet() map[ToolName]struct{} {
	set := make(map[ToolName]struct{}, len(AllowedToolNames))
	for _, n := range AllowedToolNames {
		set[n] = struct{}{}
	}
	return set
}
