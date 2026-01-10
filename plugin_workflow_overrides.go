package sdk

import (
	"errors"
	"strings"
)

func applyWorkflowModelOverride(spec *WorkflowSpec, model ModelID) error {
	if spec == nil {
		return errors.New("workflow spec required")
	}
	if model.IsEmpty() {
		return nil
	}
	spec.Model = strings.TrimSpace(model.String())
	for i := range spec.Nodes {
		spec.Nodes[i].Model = ""
		if spec.Nodes[i].SubNode != nil {
			spec.Nodes[i].SubNode.Model = ""
		}
	}
	return nil
}
