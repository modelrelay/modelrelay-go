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

func multiAgentSpec(modelA, modelB, modelC, modelAgg string, runTimeoutMS int64) (sdk.WorkflowSpecV1, error) {
	maxPar := int64(3)
	nodeTimeout := int64(20_000)
	if runTimeoutMS == 0 {
		runTimeoutMS = 30_000
	}

	exec := sdk.WorkflowExecutionV1{
		MaxParallelism: &maxPar,
		NodeTimeoutMS:  &nodeTimeout,
		RunTimeoutMS:   &runTimeoutMS,
	}

	reqA, _, err := (sdk.ResponseBuilder{}).
		Model(sdk.NewModelID(modelA)).
		MaxOutputTokens(64).
		System("You are Agent A.").
		User("Write 3 ideas for a landing page.").
		Build()
	if err != nil {
		return sdk.WorkflowSpecV1{}, err
	}
	reqB, _, err := (sdk.ResponseBuilder{}).
		Model(sdk.NewModelID(modelB)).
		MaxOutputTokens(64).
		System("You are Agent B.").
		User("Write 3 objections a user might have.").
		Build()
	if err != nil {
		return sdk.WorkflowSpecV1{}, err
	}
	reqC, _, err := (sdk.ResponseBuilder{}).
		Model(sdk.NewModelID(modelC)).
		MaxOutputTokens(64).
		System("You are Agent C.").
		User("Write 3 alternative headlines.").
		Build()
	if err != nil {
		return sdk.WorkflowSpecV1{}, err
	}
	reqAgg, _, err := (sdk.ResponseBuilder{}).
		Model(sdk.NewModelID(modelAgg)).
		MaxOutputTokens(256).
		System("Synthesize the best answer from the following agent outputs (JSON).").
		User(""). // overwritten by bindings
		Build()
	if err != nil {
		return sdk.WorkflowSpecV1{}, err
	}

	b := sdk.WorkflowV1().
		Name("multi_agent_v1_example").
		Execution(exec)

	b, err = b.LLMResponsesNode("agent_a", reqA, sdk.BoolPtr(false))
	if err != nil {
		return sdk.WorkflowSpecV1{}, err
	}
	b, err = b.LLMResponsesNode("agent_b", reqB, nil)
	if err != nil {
		return sdk.WorkflowSpecV1{}, err
	}
	b, err = b.LLMResponsesNode("agent_c", reqC, nil)
	if err != nil {
		return sdk.WorkflowSpecV1{}, err
	}
	b = b.JoinAllNode("join")
	b, err = b.LLMResponsesNodeWithBindings("aggregate", reqAgg, nil, []sdk.LLMResponsesBindingV1{
		{
			From:     "join",
			To:       sdk.JSONPointer("/input/1/content/0/text"),
			Encoding: sdk.LLMResponsesBindingEncodingJSONStringV1,
		},
	})
	if err != nil {
		return sdk.WorkflowSpecV1{}, err
	}

	b = b.
		Edge("agent_a", "join").
		Edge("agent_b", "join").
		Edge("agent_c", "join").
		Edge("join", "aggregate").
		Output("final", "aggregate", "")

	return b.Build()
}

func runOnce(ctx context.Context, client *sdk.Client, label string, spec sdk.WorkflowSpecV1) error {
	raw, _ := json.MarshalIndent(spec, "", "  ")
	log.Printf("[%s] compiled workflow.v1: %s", label, string(raw))

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
			status, err := client.Runs.Get(ctx, created.RunID)
			if err != nil {
				return err
			}
			b, _ := json.MarshalIndent(status.Outputs, "", "  ")
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
	specSuccess, err := multiAgentSpec(modelOK, modelOK, modelOK, modelOK, 0)
	if err != nil {
		return err
	}
	runCtx, runCancel := context.WithTimeout(context.Background(), 45*time.Second)
	if runErr := runOnce(runCtx, client, "success", specSuccess); runErr != nil {
		runCancel()
		return runErr
	}
	runCancel()

	// Partial failure: one node uses an unknown model; the run fails and cancels downstream work.
	specFail, err := multiAgentSpec(modelOK, modelBad, modelOK, modelOK, 0)
	if err != nil {
		return err
	}
	runCtx, runCancel = context.WithTimeout(context.Background(), 45*time.Second)
	if runErr := runOnce(runCtx, client, "partial_failure", specFail); runErr != nil {
		runCancel()
		return runErr
	}
	runCancel()

	// Cancellation: an unrealistically short run timeout produces run_canceled.
	specCancel, err := multiAgentSpec(modelOK, modelOK, modelOK, modelOK, 1)
	if err != nil {
		return err
	}
	runCtx, runCancel = context.WithTimeout(context.Background(), 45*time.Second)
	if err := runOnce(runCtx, client, "cancellation", specCancel); err != nil {
		runCancel()
		return err
	}
	runCancel()

	fmt.Println("done")
	return nil
}
