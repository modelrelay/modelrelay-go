package sdk

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestCrossSDKPromptSync verifies that the plugin conversion and orchestration prompts
// are in sync across Go, TypeScript, and Rust SDKs.
//
// This test exists because these prompts are intentionally duplicated across SDKs
// (see sdk/AGENTS.md for rationale). If you need to update a prompt, update it in
// all three SDKs and run this test to verify they match.
func TestCrossSDKPromptSync(t *testing.T) {
	// Get the directory of the Go SDK
	goSDKDir := "."
	tsFile := "../ts/src/plugins.ts"
	rustFile := "../rust/src/plugins.rs"

	// Skip if we can't find the other SDK files (e.g., in CI without full checkout)
	if _, err := os.Stat(tsFile); os.IsNotExist(err) {
		t.Skip("TypeScript SDK not found, skipping cross-SDK sync test")
	}
	if _, err := os.Stat(rustFile); os.IsNotExist(err) {
		t.Skip("Rust SDK not found, skipping cross-SDK sync test")
	}
	_ = goSDKDir

	t.Run("pluginToWorkflowSystemPrompt", func(t *testing.T) {
		goPrompt := normalizePrompt(pluginToWorkflowSystemPrompt)
		tsPrompt := normalizePrompt(extractTSPrompt(t, tsFile, "pluginToWorkflowSystemPrompt"))
		rustPrompt := normalizePrompt(extractRustWorkflowPrompt(t, rustFile))

		if goPrompt != tsPrompt {
			t.Errorf("Go and TypeScript pluginToWorkflowSystemPrompt differ:\nGo:\n%s\n\nTS:\n%s", goPrompt, tsPrompt)
		}
		if goPrompt != rustPrompt {
			t.Errorf("Go and Rust pluginToWorkflowSystemPrompt differ:\nGo:\n%s\n\nRust:\n%s", goPrompt, rustPrompt)
		}
	})

	t.Run("pluginOrchestrationSystemPrompt", func(t *testing.T) {
		goPrompt := normalizePrompt(pluginOrchestrationSystemPrompt)
		tsPrompt := normalizePrompt(extractTSPrompt(t, tsFile, "pluginOrchestrationSystemPrompt"))
		rustPrompt := normalizePrompt(extractRustOrchestrationPrompt(t, rustFile))

		if goPrompt != tsPrompt {
			t.Errorf("Go and TypeScript pluginOrchestrationSystemPrompt differ:\nGo:\n%s\n\nTS:\n%s", goPrompt, tsPrompt)
		}
		if goPrompt != rustPrompt {
			t.Errorf("Go and Rust pluginOrchestrationSystemPrompt differ:\nGo:\n%s\n\nRust:\n%s", goPrompt, rustPrompt)
		}
	})
}

// normalizePrompt normalizes a prompt for comparison by:
// - Trimming whitespace
// - Normalizing line endings
// - Replacing tool name lists with a placeholder (they're dynamically generated)
func normalizePrompt(prompt string) string {
	// Normalize line endings
	prompt = strings.ReplaceAll(prompt, "\r\n", "\n")

	// Replace tool name lists with a placeholder
	// Go: ` + AllowedToolNamesString() + `
	// TS: ${Object.values(PluginToolNames).join(", ")}
	// Rust: {tools} (interpolated)
	toolNamePattern := regexp.MustCompile(`(bash|write_file|fs_read_file|fs_list_files|fs_search|fs_edit|user_ask|execute_sql|list_tables|describe_table|sample_rows)(\s*,\s*(bash|write_file|fs_read_file|fs_list_files|fs_search|fs_edit|user_ask|execute_sql|list_tables|describe_table|sample_rows))*`)
	prompt = toolNamePattern.ReplaceAllString(prompt, "<TOOL_NAMES>")

	// Also replace the TS template literal pattern
	tsPattern := regexp.MustCompile(`\$\{Object\.values\(PluginToolNames\)\.join\(", "\)\}`)
	prompt = tsPattern.ReplaceAllString(prompt, "<TOOL_NAMES>")

	// Trim and normalize whitespace
	lines := strings.Split(prompt, "\n")
	var normalized []string
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		normalized = append(normalized, trimmed)
	}
	return strings.TrimSpace(strings.Join(normalized, "\n"))
}

// extractTSPrompt extracts a prompt constant from the TypeScript plugins file.
func extractTSPrompt(t *testing.T, path, name string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read TypeScript file: %v", err)
	}

	// Match: const <name> = `...`;
	pattern := regexp.MustCompile(`const\s+` + regexp.QuoteMeta(name) + `\s*=\s*` + "`" + `([^` + "`" + `]+)` + "`")
	matches := pattern.FindSubmatch(content)
	if len(matches) < 2 {
		t.Fatalf("failed to find %s in TypeScript file", name)
	}
	return string(matches[1])
}

// extractRustWorkflowPrompt extracts the workflow prompt from the Rust plugins file.
func extractRustWorkflowPrompt(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read Rust file: %v", err)
	}

	// The Rust prompt is in a format! macro, extract the format string
	// Look for the format! call inside plugin_to_workflow_system_prompt
	text := string(content)

	// Find the function and extract the format string
	fnStart := strings.Index(text, "fn plugin_to_workflow_system_prompt()")
	if fnStart == -1 {
		t.Fatal("failed to find plugin_to_workflow_system_prompt function in Rust file")
	}

	// Find format!( after the function start
	formatStart := strings.Index(text[fnStart:], "format!(")
	if formatStart == -1 {
		t.Fatal("failed to find format! in plugin_to_workflow_system_prompt")
	}
	formatStart += fnStart + len("format!(")

	// Find the opening quote
	quoteStart := strings.Index(text[formatStart:], "\"")
	if quoteStart == -1 {
		t.Fatal("failed to find opening quote in format!")
	}
	quoteStart += formatStart + 1

	// Find the closing quote (before the comma for {tools})
	quoteEnd := strings.Index(text[quoteStart:], "\",")
	if quoteEnd == -1 {
		t.Fatal("failed to find closing quote in format!")
	}

	prompt := text[quoteStart : quoteStart+quoteEnd]
	// Unescape Rust string escapes
	prompt = strings.ReplaceAll(prompt, "\\n", "\n")
	prompt = strings.ReplaceAll(prompt, "\\\"", "\"")
	prompt = strings.ReplaceAll(prompt, "{{", "{")
	prompt = strings.ReplaceAll(prompt, "}}", "}")
	// Replace {tools} placeholder
	prompt = strings.ReplaceAll(prompt, "{tools}", "<TOOL_NAMES>")
	return prompt
}

// extractRustOrchestrationPrompt extracts the orchestration prompt from the Rust plugins file.
func extractRustOrchestrationPrompt(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read Rust file: %v", err)
	}

	text := string(content)

	// Find the function
	fnStart := strings.Index(text, "fn plugin_orchestration_system_prompt()")
	if fnStart == -1 {
		t.Fatal("failed to find plugin_orchestration_system_prompt function in Rust file")
	}

	// Find the string literal (starts with ")
	quoteStart := strings.Index(text[fnStart:], "\"")
	if quoteStart == -1 {
		t.Fatal("failed to find opening quote in plugin_orchestration_system_prompt")
	}
	quoteStart += fnStart + 1

	// Find the closing quote
	quoteEnd := strings.Index(text[quoteStart:], "\"\n")
	if quoteEnd == -1 {
		t.Fatal("failed to find closing quote in plugin_orchestration_system_prompt")
	}

	prompt := text[quoteStart : quoteStart+quoteEnd]
	// Unescape Rust string escapes
	prompt = strings.ReplaceAll(prompt, "\\n", "\n")
	prompt = strings.ReplaceAll(prompt, "\\\"", "\"")
	return prompt
}
