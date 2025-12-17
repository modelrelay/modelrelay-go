package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	sdk "github.com/modelrelay/modelrelay/sdk/go"
	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	rawKey := strings.TrimSpace(os.Getenv("MODELRELAY_API_KEY"))
	if rawKey == "" {
		fmt.Fprintln(os.Stderr, "MODELRELAY_API_KEY required")
		return 2
	}
	key, err := sdk.ParseAPIKeyAuth(rawKey)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid MODELRELAY_API_KEY:", err)
		return 2
	}
	client, err := sdk.NewClientWithKey(key)
	if err != nil {
		fmt.Fprintln(os.Stderr, "client init error:", err)
		return 2
	}

	pluginURL := strings.TrimSpace(os.Getenv("MODELRELAY_PLUGIN_URL"))
	if pluginURL == "" {
		pluginURL = "github.com/org/repo/my-plugin"
	}
	command := strings.TrimSpace(os.Getenv("MODELRELAY_PLUGIN_COMMAND"))
	if command == "" {
		command = "analyze"
	}
	task := strings.TrimSpace(os.Getenv("MODELRELAY_PLUGIN_TASK"))
	if task == "" {
		task = "Review the authentication module"
	}

	registry := sdk.NewToolRegistry().
		Register("bash", bashTool).
		Register("write_file", writeFileTool)

	result, err := client.Plugins().QuickRun(
		ctx,
		pluginURL,
		command,
		task,
		sdk.WithToolRegistry(registry),
		sdk.WithPluginModel("claude-opus-4-5-20251101"),
		sdk.WithConverterModel("claude-3-5-haiku-latest"),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "plugin run error:", err)
		return 1
	}

	fmt.Println("run_id:", result.RunID.String())
	fmt.Println("status:", result.Status)
	fmt.Println("outputs:")
	for k, v := range result.Outputs {
		fmt.Printf("- %s: %s\n", k, string(v))
	}
	return 0
}

func bashTool(args map[string]any, _ llm.ToolCall) (any, error) {
	rawVal, exists := args["command"]
	if !exists {
		return nil, &sdk.ToolArgsError{Message: "command is required"}
	}
	raw, ok := rawVal.(string)
	if !ok {
		return nil, &sdk.ToolArgsError{Message: "command must be a string"}
	}
	cmd := strings.TrimSpace(raw)
	if cmd == "" {
		return nil, &sdk.ToolArgsError{Message: "command cannot be empty"}
	}
	//nolint:gosec // G204: intentional user-provided command for plugin execution example
	out, err := exec.CommandContext(context.Background(), "bash", "-lc", cmd).CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

func writeFileTool(args map[string]any, _ llm.ToolCall) (any, error) {
	pathVal, exists := args["path"]
	if !exists {
		return nil, &sdk.ToolArgsError{Message: "path is required"}
	}
	rawPath, ok := pathVal.(string)
	if !ok {
		return nil, &sdk.ToolArgsError{Message: "path must be a string"}
	}
	contentVal, exists := args["contents"]
	if !exists {
		return nil, &sdk.ToolArgsError{Message: "contents is required"}
	}
	rawContent, ok := contentVal.(string)
	if !ok {
		return nil, &sdk.ToolArgsError{Message: "contents must be a string"}
	}
	p := strings.TrimSpace(rawPath)
	if p == "" {
		return nil, &sdk.ToolArgsError{Message: "path cannot be empty"}
	}
	if strings.Contains(p, "\x00") {
		return nil, errors.New("invalid path")
	}
	if filepath.IsAbs(p) {
		return nil, errors.New("path must be relative")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	full := filepath.Clean(filepath.Join(cwd, p))
	if !strings.HasPrefix(full, cwd+string(os.PathSeparator)) && full != cwd {
		return nil, errors.New("path escapes working directory")
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return nil, err
	}
	if err := os.WriteFile(full, []byte(rawContent), 0o600); err != nil {
		return nil, err
	}
	return map[string]any{"written": p, "bytes": len(rawContent)}, nil
}
