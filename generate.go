package sdk

// Generate SDK types from OpenAPI spec.
// Requires: go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest

//go:generate oapi-codegen -config oapi-codegen.yaml ../../api/openapi/api.json
