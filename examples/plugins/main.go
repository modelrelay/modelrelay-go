package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	sdk "github.com/modelrelay/modelrelay/sdk/go"
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

	// Local workspace tools (tools.v0). Point this at the repo/workspace you want the plugin to analyze.
	registry := sdk.NewLocalFSTools(".")
	sdk.NewLocalWriteFileToolPack(
		".",
		sdk.WithLocalWriteFileAllow(),
	).RegisterInto(registry)

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
