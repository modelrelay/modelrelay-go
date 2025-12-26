package sdk

import (
	"encoding/json"
	"testing"
)

func TestCustomerMetadataValueAccessors(t *testing.T) {
	str := CustomerMetadataString("ok")
	if v, err := str.StringValue(); err != nil || v != "ok" {
		t.Fatalf("unexpected string value %q err=%v", v, err)
	}
	if _, err := str.BoolValue(); err == nil {
		t.Fatalf("expected bool type error")
	}

	b := CustomerMetadataBool(true)
	if v, err := b.BoolValue(); err != nil || !v {
		t.Fatalf("unexpected bool value %v err=%v", v, err)
	}

	n := CustomerMetadataNumber(json.Number("42"))
	if v, err := n.NumberValue(); err != nil || v.String() != "42" {
		t.Fatalf("unexpected number value %v err=%v", v, err)
	}

	obj := CustomerMetadataObject(CustomerMetadata{"k": CustomerMetadataString("v")})
	if v, err := obj.ObjectValue(); err != nil || v == nil {
		t.Fatalf("unexpected object value %v err=%v", v, err)
	}

	arr := CustomerMetadataArray([]CustomerMetadataValue{CustomerMetadataString("a")})
	if v, err := arr.ArrayValue(); err != nil || len(v) != 1 {
		t.Fatalf("unexpected array value %v err=%v", v, err)
	}
}

func TestCustomerMetadataGetSetAndValidate(t *testing.T) {
	var meta CustomerMetadata
	meta.Set("plan", CustomerMetadataString("pro"))

	if _, ok := meta.Get("plan"); !ok {
		t.Fatalf("expected plan key")
	}
	if v, err := meta.GetString("plan"); err != nil || v != "pro" {
		t.Fatalf("unexpected get string %q err=%v", v, err)
	}

	if _, err := meta.GetString("missing"); err == nil {
		t.Fatalf("expected missing error")
	}

	meta.Set("flag", CustomerMetadataBool(true))
	if err := meta.Validate(); err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}

	bad := CustomerMetadata{"broken": CustomerMetadataValue{}}
	if err := bad.Validate(); err == nil {
		t.Fatalf("expected invalid type error")
	}
}

func TestCustomerMetadataMarshalJSON(t *testing.T) {
	meta := CustomerMetadata{
		"s": CustomerMetadataString("hi"),
		"b": CustomerMetadataBool(true),
		"n": CustomerMetadataNumber(json.Number("3")),
		"o": CustomerMetadataObject(CustomerMetadata{"nested": CustomerMetadataString("ok")}),
		"a": CustomerMetadataArray([]CustomerMetadataValue{CustomerMetadataString("x")}),
		"z": CustomerMetadataNull(),
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(data) == 0 {
		t.Fatalf("expected json output")
	}
}
