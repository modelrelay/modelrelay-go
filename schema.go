package sdk

import (
	"encoding/json"

	llm "github.com/modelrelay/modelrelay/providers"
)

// SchemaFromType generates a JSON Schema from a Go type using reflection.
// It leverages the existing TypeToJSONSchema function but returns json.RawMessage
// for use with response_format.
//
// See TypeToJSONSchema for supported struct tags and type mappings.
func SchemaFromType[T any]() (json.RawMessage, error) {
	var zero T
	schema := TypeToJSONSchema(zero, nil)
	return json.Marshal(schema)
}

// MustSchemaFromType generates a JSON Schema from a Go type, panicking on error.
// Use this for compile-time known types where errors should never occur.
func MustSchemaFromType[T any]() json.RawMessage {
	schema, err := SchemaFromType[T]()
	if err != nil {
		panic(err)
	}
	return schema
}

// ResponseFormatFromType creates a ResponseFormat for structured outputs from a Go type.
// The schema is generated via reflection with strict mode enabled.
func ResponseFormatFromType[T any](name string) (*llm.ResponseFormat, error) {
	schema, err := SchemaFromType[T]()
	if err != nil {
		return nil, err
	}
	strict := true
	return &llm.ResponseFormat{
		Type: llm.ResponseFormatTypeJSONSchema,
		JSONSchema: &llm.JSONSchemaFormat{
			Name:   name,
			Schema: schema,
			Strict: &strict,
		},
	}, nil
}

// MustResponseFormatFromType creates a ResponseFormat, panicking on error.
func MustResponseFormatFromType[T any](name string) *llm.ResponseFormat {
	rf, err := ResponseFormatFromType[T](name)
	if err != nil {
		panic(err)
	}
	return rf
}
