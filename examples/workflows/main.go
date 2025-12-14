package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/modelrelay/modelrelay/platform/workflow"
	llm "github.com/modelrelay/modelrelay/providers"
	sdk "github.com/modelrelay/modelrelay/sdk/go"
)

type devLoginResponse struct {
	AccessToken string `json:"access_token"`
}

type meResponse struct {
	User struct {
		ProjectID string `json:"project_id"`
	} `json:"user"`
}

type apiKeyCreateResponse struct {
	APIKey struct {
		SecretKey string `json:"secret_key"`
	} `json:"api_key"`
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		log.Fatal(err)
	}
	return b
}

func decodeJSON[T any](resp *http.Response) (T, error) {
	var zero T
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return zero, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return zero, err
	}
	return out, nil
}

func bootstrapSecretKey(ctx context.Context, apiBaseURL string) (string, error) {
	client := &http.Client{}

	loginReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBaseURL+"/auth/dev-login", http.NoBody)
	if err != nil {
		return "", err
	}
	loginResp, err := client.Do(loginReq)
	if err != nil {
		return "", err
	}
	defer func() { _ = loginResp.Body.Close() }()
	login, err := decodeJSON[devLoginResponse](loginResp)
	if err != nil {
		return "", err
	}
	if login.AccessToken == "" {
		return "", errors.New("dev-login returned empty access_token")
	}

	meReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBaseURL+"/auth/me", http.NoBody)
	if err != nil {
		return "", err
	}
	meReq.Header.Set("Authorization", "Bearer "+login.AccessToken)
	meResp, err := client.Do(meReq)
	if err != nil {
		return "", err
	}
	defer func() { _ = meResp.Body.Close() }()
	me, err := decodeJSON[meResponse](meResp)
	if err != nil {
		return "", err
	}
	if me.User.ProjectID == "" {
		return "", errors.New("auth/me returned empty user.project_id")
	}

	payload := mustJSON(map[string]any{
		"label":      "Workflows example (dev)",
		"project_id": me.User.ProjectID,
		"kind":       "secret",
	})
	keyReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBaseURL+"/api-keys", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	keyReq.Header.Set("Authorization", "Bearer "+login.AccessToken)
	keyReq.Header.Set("Content-Type", "application/json")
	keyResp, err := client.Do(keyReq)
	if err != nil {
		return "", err
	}
	defer func() { _ = keyResp.Body.Close() }()
	created, err := decodeJSON[apiKeyCreateResponse](keyResp)
	if err != nil {
		return "", err
	}
	if created.APIKey.SecretKey == "" {
		return "", errors.New("api-keys create response missing api_key.secret_key")
	}
	return created.APIKey.SecretKey, nil
}

func multiAgentSpec(model string) workflow.SpecV0 {
	maxPar := int64(3)
	nodeTimeout := int64(20_000)
	runTimeout := int64(30_000)

	return workflow.SpecV0{
		Kind: workflow.KindWorkflowV0,
		Name: "multi_agent_v0_example",
		Execution: workflow.ExecutionV0{
			MaxParallelism: &maxPar,
			NodeTimeoutMS:  &nodeTimeout,
			RunTimeoutMS:   &runTimeout,
		},
		Nodes: []workflow.NodeV0{
			{
				ID:   "agent_a",
				Type: workflow.NodeTypeLLMResponses,
				Input: mustJSON(map[string]any{
					"request": llm.ResponseRequest{
						Model: model,
						Input: []llm.InputItem{
							llm.NewSystemText("You are Agent A."),
							llm.NewUserText("Write 3 ideas for a landing page."),
						},
					},
				}),
			},
			{
				ID:   "agent_b",
				Type: workflow.NodeTypeLLMResponses,
				Input: mustJSON(map[string]any{
					"request": llm.ResponseRequest{
						Model: model,
						Input: []llm.InputItem{
							llm.NewSystemText("You are Agent B."),
							llm.NewUserText("Write 3 objections a user might have."),
						},
					},
				}),
			},
			{
				ID:   "agent_c",
				Type: workflow.NodeTypeLLMResponses,
				Input: mustJSON(map[string]any{
					"request": llm.ResponseRequest{
						Model: model,
						Input: []llm.InputItem{
							llm.NewSystemText("You are Agent C."),
							llm.NewUserText("Write 3 alternative headlines."),
						},
					},
				}),
			},
			{ID: "join", Type: workflow.NodeTypeJoinAll},
			{
				ID:   "aggregate",
				Type: workflow.NodeTypeTransformJSON,
				Input: mustJSON(map[string]any{
					"object": map[string]any{
						"agent_a": map[string]any{"from": "join", "pointer": "/agent_a"},
						"agent_b": map[string]any{"from": "join", "pointer": "/agent_b"},
						"agent_c": map[string]any{"from": "join", "pointer": "/agent_c"},
					},
				}),
			},
		},
		Edges: []workflow.EdgeV0{
			{From: "agent_a", To: "join"},
			{From: "agent_b", To: "join"},
			{From: "agent_c", To: "join"},
			{From: "join", To: "aggregate"},
		},
		Outputs: []workflow.OutputRefV0{
			{Name: "result", From: "aggregate"},
		},
	}
}

func runOnce(ctx context.Context, client *sdk.Client, label string, spec workflow.SpecV0) error {
	created, err := client.Runs.Create(ctx, spec)
	if err != nil {
		return err
	}
	log.Printf("[%s] run_id=%s", label, created.RunID.String())

	stream, err := client.Runs.StreamEvents(ctx, created.RunID)
	if err != nil {
		return err
	}
	defer stream.Close() //nolint:errcheck // best-effort cleanup on return

	for {
		ev, ok, err := stream.Next()
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		switch e := ev.(type) {
		case sdk.RunEventRunFailedV0:
			log.Printf("[%s] run_failed: %s", label, e.Error.Message)
		case sdk.RunEventRunCanceledV0:
			log.Printf("[%s] run_canceled: %s", label, e.Error.Message)
		case sdk.RunEventRunCompletedV0:
			b, _ := json.MarshalIndent(e.Outputs, "", "  ")
			log.Printf("[%s] outputs: %s", label, string(b))
		}
	}
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var apiBaseURL string
	flag.StringVar(&apiBaseURL, "base-url", os.Getenv("MODELRELAY_API_BASE_URL"), "API base URL (e.g. http://localhost:8080/api/v1)")
	flag.Parse()

	if apiBaseURL == "" {
		apiBaseURL = "http://localhost:8080/api/v1"
	}
	modelOK := os.Getenv("MODELRELAY_MODEL_OK")
	if modelOK == "" {
		modelOK = "claude-sonnet-4-20250514"
	}
	modelBad := os.Getenv("MODELRELAY_MODEL_BAD")
	if modelBad == "" {
		modelBad = "does-not-exist"
	}

	bootstrapCtx, bootstrapCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer bootstrapCancel()
	secretKeyStr, err := bootstrapSecretKey(bootstrapCtx, apiBaseURL)
	if err != nil {
		return err
	}
	key, err := sdk.ParseAPIKeyAuth(secretKeyStr)
	if err != nil {
		return err
	}
	client, err := sdk.NewClientWithKey(key, sdk.WithBaseURL(apiBaseURL))
	if err != nil {
		return err
	}

	// Use per-run contexts so the example doesn't fail due to cumulative wall-clock time.
	runCtx, runCancel := context.WithTimeout(context.Background(), 45*time.Second)
	if err := runOnce(runCtx, client, "success", multiAgentSpec(modelOK)); err != nil {
		runCancel()
		return err
	}
	runCancel()

	// Partial failure: one node uses an unknown model; the run fails and cancels downstream work.
	specFail := multiAgentSpec(modelOK)
	for i := range specFail.Nodes {
		if specFail.Nodes[i].ID == "agent_b" {
			specFail.Nodes[i].Input = mustJSON(map[string]any{
				"request": llm.ResponseRequest{
					Model: modelBad,
					Input: []llm.InputItem{
						llm.NewSystemText("You are Agent B."),
						llm.NewUserText("Write 3 objections a user might have."),
					},
				},
			})
		}
	}
	runCtx, runCancel = context.WithTimeout(context.Background(), 45*time.Second)
	if err := runOnce(runCtx, client, "partial_failure", specFail); err != nil {
		runCancel()
		return err
	}
	runCancel()

	// Cancellation: an unrealistically short run timeout produces run_canceled.
	specCancel := multiAgentSpec(modelOK)
	runTimeout := int64(1)
	specCancel.Execution.RunTimeoutMS = &runTimeout
	runCtx, runCancel = context.WithTimeout(context.Background(), 45*time.Second)
	if err := runOnce(runCtx, client, "cancellation", specCancel); err != nil {
		runCancel()
		return err
	}
	runCancel()

	fmt.Println("done")
	return nil
}
