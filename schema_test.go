package sdk

import (
	"encoding/json"
	"testing"

	llm "github.com/modelrelay/modelrelay/providers"
)

func TestSchemaFromType(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	schema, err := SchemaFromType[Person]()
	if err != nil {
		t.Fatalf("SchemaFromType failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(schema, &decoded); err != nil {
		t.Fatalf("Failed to decode schema: %v", err)
	}

	if decoded["type"] != "object" {
		t.Errorf("Expected type=object, got %v", decoded["type"])
	}

	props, ok := decoded["properties"].(map[string]any)
	if !ok {
		t.Fatalf("Expected properties to be a map, got %T", decoded["properties"])
	}

	nameSchema, ok := props["name"].(map[string]any)
	if !ok {
		t.Fatalf("Expected name to be a schema, got %T", props["name"])
	}
	if nameSchema["type"] != "string" {
		t.Errorf("Expected name.type=string, got %v", nameSchema["type"])
	}

	ageSchema, ok := props["age"].(map[string]any)
	if !ok {
		t.Fatalf("Expected age to be a schema, got %T", props["age"])
	}
	if ageSchema["type"] != "integer" {
		t.Errorf("Expected age.type=integer, got %v", ageSchema["type"])
	}

	required, ok := decoded["required"].([]any)
	if !ok {
		t.Fatalf("Expected required to be an array, got %T", decoded["required"])
	}
	if len(required) != 2 {
		t.Errorf("Expected 2 required fields, got %d", len(required))
	}
}

func TestSchemaFromType_OptionalFields(t *testing.T) {
	type Config struct {
		Name     string  `json:"name"`
		Optional string  `json:"optional,omitempty"`
		Pointer  *string `json:"pointer"`
	}

	schema, err := SchemaFromType[Config]()
	if err != nil {
		t.Fatalf("SchemaFromType failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(schema, &decoded); err != nil {
		t.Fatalf("Failed to decode schema: %v", err)
	}

	required, ok := decoded["required"].([]any)
	if !ok {
		t.Fatalf("Expected required to be an array, got %T", decoded["required"])
	}

	// Only "name" should be required
	if len(required) != 1 || required[0] != "name" {
		t.Errorf("Expected [name] as required, got %v", required)
	}
}

func TestSchemaFromType_NestedStruct(t *testing.T) {
	type Address struct {
		Street string `json:"street"`
		City   string `json:"city"`
	}
	type Person struct {
		Name    string  `json:"name"`
		Address Address `json:"address"`
	}

	schema, err := SchemaFromType[Person]()
	if err != nil {
		t.Fatalf("SchemaFromType failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(schema, &decoded); err != nil {
		t.Fatalf("Failed to decode schema: %v", err)
	}

	props := decoded["properties"].(map[string]any)
	addressSchema := props["address"].(map[string]any)

	if addressSchema["type"] != "object" {
		t.Errorf("Expected address.type=object, got %v", addressSchema["type"])
	}

	addressProps := addressSchema["properties"].(map[string]any)
	if _, ok := addressProps["street"]; !ok {
		t.Error("Expected address to have street property")
	}
	if _, ok := addressProps["city"]; !ok {
		t.Error("Expected address to have city property")
	}
}

func TestSchemaFromType_Arrays(t *testing.T) {
	type Data struct {
		Tags []string `json:"tags"`
	}

	schema, err := SchemaFromType[Data]()
	if err != nil {
		t.Fatalf("SchemaFromType failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(schema, &decoded); err != nil {
		t.Fatalf("Failed to decode schema: %v", err)
	}

	props := decoded["properties"].(map[string]any)
	tagsSchema := props["tags"].(map[string]any)

	if tagsSchema["type"] != "array" {
		t.Errorf("Expected tags.type=array, got %v", tagsSchema["type"])
	}

	items := tagsSchema["items"].(map[string]any)
	if items["type"] != "string" {
		t.Errorf("Expected tags.items.type=string, got %v", items["type"])
	}
}

func TestSchemaFromType_WithTags(t *testing.T) {
	type Status struct {
		Code  string `json:"code" description:"Status code" enum:"active,inactive,pending"`
		Email string `json:"email" format:"email"`
	}

	schema, err := SchemaFromType[Status]()
	if err != nil {
		t.Fatalf("SchemaFromType failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(schema, &decoded); err != nil {
		t.Fatalf("Failed to decode schema: %v", err)
	}

	props := decoded["properties"].(map[string]any)

	codeSchema := props["code"].(map[string]any)
	if codeSchema["description"] != "Status code" {
		t.Errorf("Expected description='Status code', got %v", codeSchema["description"])
	}
	enum, ok := codeSchema["enum"].([]any)
	if !ok || len(enum) != 3 {
		t.Errorf("Expected 3 enum values, got %v", codeSchema["enum"])
	}

	emailSchema := props["email"].(map[string]any)
	if emailSchema["format"] != "email" {
		t.Errorf("Expected format=email, got %v", emailSchema["format"])
	}
}

func TestMustSchemaFromType(t *testing.T) {
	type Simple struct {
		Value string `json:"value"`
	}

	// Should not panic
	schema := MustSchemaFromType[Simple]()
	if len(schema) == 0 {
		t.Error("Expected non-empty schema")
	}
}

func TestOutputFormatFromType(t *testing.T) {
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	rf, err := OutputFormatFromType[Person]("person")
	if err != nil {
		t.Fatalf("OutputFormatFromType failed: %v", err)
	}

	if rf.Type != llm.OutputFormatTypeJSONSchema {
		t.Errorf("Expected type=json_schema, got %v", rf.Type)
	}

	if rf.JSONSchema == nil {
		t.Fatal("Expected JSONSchema to be set")
	}

	if rf.JSONSchema.Name != "person" {
		t.Errorf("Expected name=person, got %v", rf.JSONSchema.Name)
	}

	if rf.JSONSchema.Strict == nil || !*rf.JSONSchema.Strict {
		t.Error("Expected strict=true")
	}

	if len(rf.JSONSchema.Schema) == 0 {
		t.Error("Expected non-empty schema")
	}
}

func TestMustOutputFormatFromType(t *testing.T) {
	type Simple struct {
		Value string `json:"value"`
	}

	// Should not panic
	rf := MustOutputFormatFromType[Simple]("simple")
	if rf.JSONSchema.Name != "simple" {
		t.Errorf("Expected name=simple, got %v", rf.JSONSchema.Name)
	}
}
