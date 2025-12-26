package sdk

import (
	"encoding/json"
	"testing"
	"time"

	llm "github.com/modelrelay/modelrelay/sdk/go/llm"
)

func TestResponseBuilderMethods(t *testing.T) {
	client := &Client{}
	builder := (&ResponsesClient{client: client}).New()

	format := llm.OutputFormat{
		Type: llm.OutputFormatTypeJSONSchema,
		JSONSchema: &llm.JSONSchemaFormat{
			Name:   "schema",
			Schema: []byte(`{"type":"object"}`),
		},
	}
	tools := []llm.Tool{
		{Type: llm.ToolTypeFunction, Function: &llm.FunctionTool{Name: "tool1", Parameters: json.RawMessage(`{"type":"object"}`)}},
	}

	req, opts, err := builder.
		Provider(NewProviderID("openai")).
		Model(NewModelID("gpt-4o")).
		Temperature(0.7).
		Stop("  stop ", "").
		OutputFormat(format).
		Tools(tools).
		Tool(llm.Tool{Type: llm.ToolTypeFunction, Function: &llm.FunctionTool{Name: "tool2"}}).
		FunctionTool("tool3", "desc", json.RawMessage(`{"type":"object"}`)).
		ToolChoiceAuto().
		System("sys").
		User("user").
		Assistant("assistant").
		ToolResultText("call_1", "result").
		MaxOutputTokens(120).
		RequestID("req-123").
		CustomerID("cust-1").
		Header("X-Test", "value").
		Headers(map[string]string{"X-Test-2": "value2", "": "skip"}).
		Timeout(2 * time.Second).
		StreamTTFTTimeout(100 * time.Millisecond).
		StreamIdleTimeout(200 * time.Millisecond).
		StreamTotalTimeout(300 * time.Millisecond).
		Retry(RetryConfig{MaxAttempts: 2}).
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if req.provider.String() != "openai" {
		t.Fatalf("unexpected provider %s", req.provider)
	}
	if req.model.String() != "gpt-4o" {
		t.Fatalf("unexpected model %s", req.model)
	}
	if req.temperature == nil || *req.temperature != 0.7 {
		t.Fatalf("unexpected temperature")
	}
	if len(req.stop) != 1 || req.stop[0] != "stop" {
		t.Fatalf("unexpected stop values: %+v", req.stop)
	}
	if req.outputFormat == nil || req.outputFormat.Type != llm.OutputFormatTypeJSONSchema {
		t.Fatalf("unexpected output format")
	}
	if len(req.tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(req.tools))
	}
	if req.toolChoice == nil || req.toolChoice.Type != llm.ToolChoiceAuto {
		t.Fatalf("unexpected tool choice")
	}
	if len(req.input) != 4 {
		t.Fatalf("unexpected input size %d", len(req.input))
	}
	if req.maxOutputTokens != 120 {
		t.Fatalf("unexpected max output tokens %d", req.maxOutputTokens)
	}

	callOpts := buildResponseCallOptions(opts)
	if got := callOpts.headers.Get("X-ModelRelay-Request-Id"); got != "req-123" {
		t.Fatalf("unexpected request id header %q", got)
	}
	if got := callOpts.headers.Get("X-ModelRelay-Customer-Id"); got != "cust-1" {
		t.Fatalf("unexpected customer id header %q", got)
	}
	if got := callOpts.headers.Get("X-Test"); got != "value" {
		t.Fatalf("unexpected header %q", got)
	}
	if callOpts.timeout == nil || *callOpts.timeout != 2*time.Second {
		t.Fatalf("unexpected timeout")
	}
	if callOpts.stream.TTFT != 100*time.Millisecond || callOpts.stream.Idle != 200*time.Millisecond || callOpts.stream.Total != 300*time.Millisecond {
		t.Fatalf("unexpected stream timeouts %+v", callOpts.stream)
	}
	if callOpts.retry == nil || callOpts.retry.MaxAttempts != 2 {
		t.Fatalf("unexpected retry config %+v", callOpts.retry)
	}

	disableReq, disableOpts, err := builder.Model(NewModelID("gpt-4o")).User("u").DisableRetry().Build()
	if err != nil {
		t.Fatalf("disable retry build: %v", err)
	}
	if disableReq.model.String() != "gpt-4o" {
		t.Fatalf("unexpected model in disable retry")
	}
	disableCallOpts := buildResponseCallOptions(disableOpts)
	if disableCallOpts.retry == nil || disableCallOpts.retry.MaxAttempts != 1 {
		t.Fatalf("expected retry disabled")
	}
}
