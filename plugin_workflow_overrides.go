package sdk

import (
	"encoding/json"
	"errors"
	"fmt"
)

func applyWorkflowModelOverride(spec *WorkflowSpecV1, model ModelID) error {
	if spec == nil {
		return errors.New("workflow spec required")
	}
	if model.IsEmpty() {
		return nil
	}
	for i := range spec.Nodes {
		if spec.Nodes[i].Type != WorkflowNodeTypeV1LLMResponses {
			continue
		}
		var input llmResponsesNodeInputV1
		if err := json.Unmarshal(spec.Nodes[i].Input, &input); err != nil {
			return fmt.Errorf("node %q: invalid input JSON: %w", spec.Nodes[i].ID, err)
		}
		input.Request.Model = model.String()
		raw, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("node %q: failed to marshal input: %w", spec.Nodes[i].ID, err)
		}
		spec.Nodes[i].Input = raw
	}
	return nil
}
