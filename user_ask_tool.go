package sdk

import (
	"encoding/json"
	"errors"
	"strings"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

// UserAskToolOption is a multiple-choice option for user.ask.
type UserAskToolOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// UserAskToolArgs are the arguments for the user.ask tool.
type UserAskToolArgs struct {
	Question      string              `json:"question"`
	Options       []UserAskToolOption `json:"options,omitempty"`
	AllowFreeform *bool               `json:"allow_freeform,omitempty"`
}

// UserAskToolResult is the structured result for user.ask tool calls.
type UserAskToolResult struct {
	Answer     string `json:"answer"`
	IsFreeform bool   `json:"is_freeform"`
}

var userAskToolSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "question": {
      "type": "string",
      "minLength": 1,
      "description": "The question to ask the user."
    },
    "options": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "label": { "type": "string", "minLength": 1 },
          "description": { "type": "string" }
        },
        "required": ["label"]
      },
      "description": "Optional multiple choice options."
    },
    "allow_freeform": {
      "type": "boolean",
      "default": true,
      "description": "Allow user to type a custom response."
    }
  },
  "required": ["question"]
}`)

// UserAskTool returns the tools.v0 definition for user.ask.
func UserAskTool() llm.Tool {
	return llm.Tool{
		Type: llm.ToolTypeFunction,
		Function: &llm.FunctionTool{
			Name:        ToolNameUserAsk,
			Description: "Ask the user a clarifying question.",
			Parameters:  append(json.RawMessage(nil), userAskToolSchema...),
		},
	}
}

// IsUserAskToolCall reports whether the tool call is a user.ask function tool.
func IsUserAskToolCall(call llm.ToolCall) bool {
	return call.Type == llm.ToolTypeFunction && call.Function != nil && call.Function.Name == ToolNameUserAsk
}

// ParseUserAskArgs parses and validates user.ask arguments from a tool call.
func ParseUserAskArgs(call llm.ToolCall) (UserAskToolArgs, error) {
	if call.Function == nil {
		return UserAskToolArgs{}, errors.New("user.ask requires function tool arguments")
	}
	if strings.TrimSpace(call.Function.Arguments) == "" {
		return UserAskToolArgs{}, errors.New("user.ask arguments required")
	}
	var args UserAskToolArgs
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return UserAskToolArgs{}, err
	}
	args.Question = strings.TrimSpace(args.Question)
	if args.Question == "" {
		return UserAskToolArgs{}, errors.New("user.ask question required")
	}
	for i := range args.Options {
		args.Options[i].Label = strings.TrimSpace(args.Options[i].Label)
		args.Options[i].Description = strings.TrimSpace(args.Options[i].Description)
		if args.Options[i].Label == "" {
			return UserAskToolArgs{}, errors.New("user.ask options label required")
		}
	}
	return args, nil
}

// FormatUserAskResult encodes a user.ask response as a JSON string.
func FormatUserAskResult(result UserAskToolResult) (string, error) {
	result.Answer = strings.TrimSpace(result.Answer)
	if result.Answer == "" {
		return "", errors.New("user.ask answer required")
	}
	out, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// UserAskResultFreeform builds a freeform user.ask result string.
func UserAskResultFreeform(answer string) (string, error) {
	return FormatUserAskResult(UserAskToolResult{Answer: answer, IsFreeform: true})
}

// UserAskResultChoice builds a multiple-choice user.ask result string.
func UserAskResultChoice(answer string) (string, error) {
	return FormatUserAskResult(UserAskToolResult{Answer: answer, IsFreeform: false})
}
