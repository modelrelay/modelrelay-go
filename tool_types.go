package sdk

import llm "github.com/modelrelay/modelrelay/sdk/go/llm"

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
	ToolNameBash        ToolName = "bash"
	ToolNameWriteFile   ToolName = "write_file"
)
