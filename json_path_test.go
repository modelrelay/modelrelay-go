package sdk

import (
	"testing"

	"github.com/modelrelay/modelrelay/platform/workflow"
)

func TestLLMOutputPath_BuildsCorrectPointers(t *testing.T) {
	cases := []struct {
		name string
		got  JSONPointer
		want string
	}{
		{"Content(0).Text()", LLMOutput().Content(0).Text(), "/output/0/content/0/text"},
		{"Content(1).Text()", LLMOutput().Content(1).Text(), "/output/0/content/1/text"},
		{"Index(1).Content(0).Text()", LLMOutput().Index(1).Content(0).Text(), "/output/1/content/0/text"},
		{"Content(0).Type()", LLMOutput().Content(0).Type(), "/output/0/content/0/type"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestLLMInputPath_BuildsCorrectPointers(t *testing.T) {
	cases := []struct {
		name string
		got  JSONPointer
		want string
	}{
		{"Message(0).Text()", LLMInput().Message(0).Text(), "/input/0/content/0/text"},
		{"Message(1).Text()", LLMInput().Message(1).Text(), "/input/1/content/0/text"},
		{"SystemMessage().Text()", LLMInput().SystemMessage().Text(), "/input/0/content/0/text"},
		{"UserMessage().Text()", LLMInput().UserMessage().Text(), "/input/1/content/0/text"},
		{"Message(0).Content(1).Text()", LLMInput().Message(0).Content(1).Text(), "/input/0/content/1/text"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}

func TestPreBuiltPaths_MatchConstants(t *testing.T) {
	// Verify pre-built paths match the string constants
	if LLMOutputText != LLMTextOutput {
		t.Errorf("LLMOutputText %q != LLMTextOutput %q", LLMOutputText, LLMTextOutput)
	}

	if LLMInputUserText != LLMUserMessageText {
		t.Errorf("LLMInputUserText %q != LLMUserMessageText %q", LLMInputUserText, LLMUserMessageText)
	}
}

func TestPreBuiltPaths_MatchPlatformConstants(t *testing.T) {
	// Verify pre-built paths match platform canonical definitions
	if string(LLMOutputText) != workflow.LLMTextOutputPointer {
		t.Errorf("LLMOutputText %q != platform.LLMTextOutputPointer %q",
			LLMOutputText, workflow.LLMTextOutputPointer)
	}

	if string(LLMInputUserText) != workflow.LLMUserMessageTextPointerIndex1 {
		t.Errorf("LLMInputUserText %q != platform.LLMUserMessageTextPointerIndex1 %q",
			LLMInputUserText, workflow.LLMUserMessageTextPointerIndex1)
	}

	if string(LLMInputFirstMessageText) != workflow.LLMUserMessageTextPointer {
		t.Errorf("LLMInputFirstMessageText %q != platform.LLMUserMessageTextPointer %q",
			LLMInputFirstMessageText, workflow.LLMUserMessageTextPointer)
	}
}

func TestTypedPaths_ProduceValidRFC6901(t *testing.T) {
	paths := []JSONPointer{
		LLMOutput().Content(0).Text(),
		LLMOutput().Index(5).Content(3).Text(),
		LLMInput().Message(0).Text(),
		LLMInput().Message(10).Content(5).Type(),
	}

	for _, p := range paths {
		s := string(p)
		// Must start with /
		if len(s) == 0 || s[0] != '/' {
			t.Errorf("pointer %q does not start with /", s)
		}
		// Must not have empty segments
		for i := 0; i < len(s)-1; i++ {
			if s[i] == '/' && s[i+1] == '/' {
				t.Errorf("pointer %q has empty segment", s)
			}
		}
	}
}

func TestJoinOutputPath_BuildsCorrectPointers(t *testing.T) {
	cases := []struct {
		name string
		got  JSONPointer
		want string
	}{
		{"Text()", JoinOutput("cost_analyst").Text(), "/cost_analyst/output/0/content/0/text"},
		{"Output().Content(0).Text()", JoinOutput("revenue").Output().Content(0).Text(), "/revenue/output/0/content/0/text"},
		{"Output().Index(1).Content(0).Text()", JoinOutput("agent").Output().Index(1).Content(0).Text(), "/agent/output/1/content/0/text"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}
