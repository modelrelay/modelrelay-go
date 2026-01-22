package rlm

import "testing"

func TestLocalInterpreter_Defaults(t *testing.T) {
	interp := NewLocalInterpreter(LocalInterpreterConfig{})
	if interp.cfg.PythonPath == "" {
		t.Fatal("expected default PythonPath")
	}
	if interp.cfg.Limits.MaxTimeoutMS == 0 {
		t.Fatal("expected default MaxTimeoutMS")
	}
	if interp.cfg.Caps.MaxInlineBytes == 0 {
		t.Fatal("expected default MaxInlineBytes")
	}
}

func TestLocalInterpreter_PlanContextInline(t *testing.T) {
	interp := NewLocalInterpreter(LocalInterpreterConfig{})
	plan, err := interp.PlanContext([]byte(`{"a":1}`), "/tmp/context.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Mode != "inline" {
		t.Fatalf("mode = %s, want inline", plan.Mode)
	}
}
