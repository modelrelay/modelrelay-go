package sdk

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/modelrelay/modelrelay/platform/workflow"
)

type WorkflowBuildIssueCode string

const (
	WorkflowBuildIssueDuplicateNodeID       WorkflowBuildIssueCode = "duplicate_node_id"
	WorkflowBuildIssueDuplicateEdge         WorkflowBuildIssueCode = "duplicate_edge"
	WorkflowBuildIssueEdgeFromUnknownNode   WorkflowBuildIssueCode = "edge_from_unknown_node"
	WorkflowBuildIssueEdgeToUnknownNode     WorkflowBuildIssueCode = "edge_to_unknown_node"
	WorkflowBuildIssueDuplicateOutputName   WorkflowBuildIssueCode = "duplicate_output_name"
	WorkflowBuildIssueOutputFromUnknownNode WorkflowBuildIssueCode = "output_from_unknown_node"
	WorkflowBuildIssueMissingNodes          WorkflowBuildIssueCode = "missing_nodes"
	WorkflowBuildIssueMissingOutputs        WorkflowBuildIssueCode = "missing_outputs"
	WorkflowBuildIssueMissingKind           WorkflowBuildIssueCode = "missing_kind"
	WorkflowBuildIssueInvalidKind           WorkflowBuildIssueCode = "invalid_kind"
)

type WorkflowBuildIssue struct {
	Code    WorkflowBuildIssueCode `json:"code"`
	Message string                 `json:"message"`
}

type WorkflowBuildError struct {
	Issues []WorkflowBuildIssue
}

func (e WorkflowBuildError) Error() string {
	if len(e.Issues) == 0 {
		return "invalid workflow.v0 spec"
	}
	if len(e.Issues) == 1 {
		return e.Issues[0].Message
	}
	return fmt.Sprintf("%s (and %d more)", e.Issues[0].Message, len(e.Issues)-1)
}

func ValidateWorkflowSpecV0(spec workflow.SpecV0) []WorkflowBuildIssue {
	var issues []WorkflowBuildIssue

	if strings.TrimSpace(string(spec.Kind)) == "" {
		issues = append(issues, WorkflowBuildIssue{
			Code:    WorkflowBuildIssueMissingKind,
			Message: "kind is required",
		})
	} else if spec.Kind != workflow.KindWorkflowV0 {
		issues = append(issues, WorkflowBuildIssue{
			Code:    WorkflowBuildIssueInvalidKind,
			Message: "invalid kind: " + string(spec.Kind),
		})
	}

	if len(spec.Nodes) == 0 {
		issues = append(issues, WorkflowBuildIssue{
			Code:    WorkflowBuildIssueMissingNodes,
			Message: "at least one node is required",
		})
	}

	if len(spec.Outputs) == 0 {
		issues = append(issues, WorkflowBuildIssue{
			Code:    WorkflowBuildIssueMissingOutputs,
			Message: "at least one output is required",
		})
	}

	nodesByID := make(map[workflow.NodeID]struct{}, len(spec.Nodes))
	dupes := make(map[workflow.NodeID]struct{})
	for _, n := range spec.Nodes {
		if !n.ID.Valid() {
			continue
		}
		if _, ok := nodesByID[n.ID]; ok {
			dupes[n.ID] = struct{}{}
		} else {
			nodesByID[n.ID] = struct{}{}
		}
	}
	for id := range dupes {
		issues = append(issues, WorkflowBuildIssue{
			Code:    WorkflowBuildIssueDuplicateNodeID,
			Message: "duplicate node id: " + string(id),
		})
	}

	type edgeKey struct {
		From workflow.NodeID
		To   workflow.NodeID
	}
	edges := make(map[edgeKey]struct{}, len(spec.Edges))
	for _, e := range spec.Edges {
		if e.From.Valid() {
			if _, ok := nodesByID[e.From]; !ok {
				issues = append(issues, WorkflowBuildIssue{
					Code:    WorkflowBuildIssueEdgeFromUnknownNode,
					Message: "edge from unknown node: " + string(e.From),
				})
			}
		}
		if e.To.Valid() {
			if _, ok := nodesByID[e.To]; !ok {
				issues = append(issues, WorkflowBuildIssue{
					Code:    WorkflowBuildIssueEdgeToUnknownNode,
					Message: "edge to unknown node: " + string(e.To),
				})
			}
		}
		k := edgeKey{From: e.From, To: e.To}
		if _, ok := edges[k]; ok {
			issues = append(issues, WorkflowBuildIssue{
				Code:    WorkflowBuildIssueDuplicateEdge,
				Message: fmt.Sprintf("duplicate edge: %s -> %s", e.From, e.To),
			})
		} else {
			edges[k] = struct{}{}
		}
	}

	outputNames := make(map[workflow.OutputName]struct{}, len(spec.Outputs))
	outputDupes := make(map[workflow.OutputName]struct{})
	for _, o := range spec.Outputs {
		if o.Name.Valid() {
			if _, ok := outputNames[o.Name]; ok {
				outputDupes[o.Name] = struct{}{}
			} else {
				outputNames[o.Name] = struct{}{}
			}
		}
		if o.From.Valid() {
			if _, ok := nodesByID[o.From]; !ok {
				issues = append(issues, WorkflowBuildIssue{
					Code:    WorkflowBuildIssueOutputFromUnknownNode,
					Message: "output from unknown node: " + string(o.From),
				})
			}
		}
	}
	for name := range outputDupes {
		issues = append(issues, WorkflowBuildIssue{
			Code:    WorkflowBuildIssueDuplicateOutputName,
			Message: "duplicate output name: " + string(name),
		})
	}

	return issues
}

type llmResponsesNodeInputV0 struct {
	Request responseRequestPayload `json:"request"`
	Stream  *bool                  `json:"stream,omitempty"`
}

type TransformJSONNodeInputV0 struct {
	Object map[string]TransformJSONFieldRefV0 `json:"object,omitempty"`
	Merge  []TransformJSONRefV0              `json:"merge,omitempty"`
}

type TransformJSONFieldRefV0 struct {
	From    workflow.NodeID      `json:"from"`
	Pointer workflow.JSONPointer `json:"pointer,omitempty"`
}

type TransformJSONRefV0 struct {
	From    workflow.NodeID      `json:"from"`
	Pointer workflow.JSONPointer `json:"pointer,omitempty"`
}

type WorkflowBuilderV0 struct {
	name      string
	execution *workflow.ExecutionV0
	nodes     []workflow.NodeV0
	edges     []workflow.EdgeV0
	outputs   []workflow.OutputRefV0
}

func WorkflowV0() WorkflowBuilderV0 {
	return WorkflowBuilderV0{}
}

func (b WorkflowBuilderV0) Name(name string) WorkflowBuilderV0 {
	b.name = strings.TrimSpace(name)
	return b
}

func (b WorkflowBuilderV0) Execution(exec workflow.ExecutionV0) WorkflowBuilderV0 {
	b.execution = &exec
	return b
}

func (b WorkflowBuilderV0) Node(node workflow.NodeV0) WorkflowBuilderV0 {
	next := make([]workflow.NodeV0, len(b.nodes)+1)
	copy(next, b.nodes)
	next[len(b.nodes)] = node
	b.nodes = next
	return b
}

func (b WorkflowBuilderV0) LLMResponsesNode(id workflow.NodeID, req ResponseRequest, stream *bool) (WorkflowBuilderV0, error) {
	payload := llmResponsesNodeInputV0{
		Request: newResponseRequestPayload(req),
		Stream:  stream,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return WorkflowBuilderV0{}, err
	}
	return b.Node(workflow.NodeV0{
		ID:    id,
		Type:  workflow.NodeTypeLLMResponses,
		Input: raw,
	}), nil
}

func (b WorkflowBuilderV0) JoinAllNode(id workflow.NodeID) WorkflowBuilderV0 {
	return b.Node(workflow.NodeV0{
		ID:   id,
		Type: workflow.NodeTypeJoinAll,
	})
}

func (b WorkflowBuilderV0) TransformJSONNode(id workflow.NodeID, input TransformJSONNodeInputV0) (WorkflowBuilderV0, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return WorkflowBuilderV0{}, err
	}
	return b.Node(workflow.NodeV0{
		ID:    id,
		Type:  workflow.NodeTypeTransformJSON,
		Input: raw,
	}), nil
}

func (b WorkflowBuilderV0) Edge(from, to workflow.NodeID) WorkflowBuilderV0 {
	next := make([]workflow.EdgeV0, len(b.edges)+1)
	copy(next, b.edges)
	next[len(b.edges)] = workflow.EdgeV0{From: from, To: to}
	b.edges = next
	return b
}

func (b WorkflowBuilderV0) Output(name workflow.OutputName, from workflow.NodeID, pointer workflow.JSONPointer) WorkflowBuilderV0 {
	next := make([]workflow.OutputRefV0, len(b.outputs)+1)
	copy(next, b.outputs)
	next[len(b.outputs)] = workflow.OutputRefV0{Name: name, From: from, Pointer: pointer}
	b.outputs = next
	return b
}

func (b WorkflowBuilderV0) Build() (workflow.SpecV0, error) {
	spec := workflow.SpecV0{
		Kind:    workflow.KindWorkflowV0,
		Name:    b.name,
		Nodes:   append([]workflow.NodeV0(nil), b.nodes...),
		Edges:   append([]workflow.EdgeV0(nil), b.edges...),
		Outputs: append([]workflow.OutputRefV0(nil), b.outputs...),
	}
	if b.execution != nil {
		spec.Execution = *b.execution
	}

	sort.Slice(spec.Edges, func(i, j int) bool {
		if spec.Edges[i].From != spec.Edges[j].From {
			return spec.Edges[i].From < spec.Edges[j].From
		}
		return spec.Edges[i].To < spec.Edges[j].To
	})

	sort.Slice(spec.Outputs, func(i, j int) bool {
		if spec.Outputs[i].Name != spec.Outputs[j].Name {
			return spec.Outputs[i].Name < spec.Outputs[j].Name
		}
		if spec.Outputs[i].From != spec.Outputs[j].From {
			return spec.Outputs[i].From < spec.Outputs[j].From
		}
		return spec.Outputs[i].Pointer < spec.Outputs[j].Pointer
	})

	issues := ValidateWorkflowSpecV0(spec)
	if len(issues) > 0 {
		return workflow.SpecV0{}, WorkflowBuildError{Issues: issues}
	}
	return spec, nil
}
