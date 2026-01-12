package sdk

import (
	"strings"
	"testing"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

func TestParseUserAskArgs(t *testing.T) {
	call := llm.ToolCall{
		ID:   "call_user_ask",
		Type: llm.ToolTypeFunction,
		Function: &llm.FunctionCall{
			Name:      ToolNameUserAsk,
			Arguments: `{"question":"Pick one","allow_freeform":false,"options":[{"label":"A"}]}`,
		},
	}

	args, err := ParseUserAskArgs(call)
	if err != nil {
		t.Fatalf("parse user.ask args: %v", err)
	}
	if args.Question != "Pick one" {
		t.Fatalf("unexpected question: %q", args.Question)
	}
	if args.AllowFreeform == nil || *args.AllowFreeform {
		t.Fatalf("expected allow_freeform=false")
	}
	if len(args.Options) != 1 || args.Options[0].Label != "A" {
		t.Fatalf("unexpected options: %#v", args.Options)
	}
}

func TestFormatUserAskResult(t *testing.T) {
	out, err := FormatUserAskResult(UserAskToolResult{Answer: "PostgreSQL", IsFreeform: false})
	if err != nil {
		t.Fatalf("format user.ask result: %v", err)
	}
	if !strings.Contains(out, "PostgreSQL") || !strings.Contains(out, "is_freeform") {
		t.Fatalf("unexpected result: %q", out)
	}
}
