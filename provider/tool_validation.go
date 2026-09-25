package provider

import (
	"encoding/json"
	"fmt"
)

func ValidateTools(tools []Tool) error {
	for index, tool := range tools {
		switch tool.Type {
		case ToolTypeProvider:
			if tool.Description != "" || tool.InputSchema != nil || tool.InputExamples != nil || tool.Strict != nil || tool.ProviderOptions != nil {
				return fmt.Errorf("provider: tool %d has function-only fields", index)
			}
			for key, value := range tool.Args {
				if !json.Valid(value) {
					return fmt.Errorf("provider: tool %d has invalid argument %q", index, key)
				}
			}
		case ToolTypeFunction:
			if tool.ID != "" || tool.Args != nil {
				return fmt.Errorf("provider: tool %d has provider-only fields", index)
			}
		}
	}
	return nil
}
