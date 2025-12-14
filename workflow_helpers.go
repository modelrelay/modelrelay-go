package sdk

import "github.com/modelrelay/modelrelay/platform/workflow"

// TransformJSONValue constructs a transform.json value reference.
func TransformJSONValue(from workflow.NodeID, pointer workflow.JSONPointer) TransformJSONRefV0 {
	return TransformJSONRefV0{From: from, Pointer: pointer}
}

// TransformJSONFieldValue constructs a transform.json object field reference.
func TransformJSONFieldValue(from workflow.NodeID, pointer workflow.JSONPointer) TransformJSONFieldRefV0 {
	return TransformJSONFieldRefV0{From: from, Pointer: pointer}
}

// TransformJSONObject constructs a transform.json input using the "object" form.
func TransformJSONObject(fields map[string]TransformJSONFieldRefV0) TransformJSONNodeInputV0 {
	return TransformJSONNodeInputV0{Object: fields}
}

// TransformJSONMerge constructs a transform.json input using the "merge" form.
func TransformJSONMerge(items []TransformJSONRefV0) TransformJSONNodeInputV0 {
	return TransformJSONNodeInputV0{Merge: items}
}

