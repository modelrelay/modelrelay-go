package sdk

import "github.com/modelrelay/modelrelay/sdk/go/generated"

// Plugin is the server-side loaded plugin representation.
//
// Source of truth: OpenAPI-generated types in sdk/go/generated.
type Plugin = generated.PluginsLoadResponseV0

// PluginRunStartResponse is the response returned by POST /plugins/runs.
type PluginRunStartResponse = generated.PluginsRunResponseV0
