package aisdk

import (
	"fmt"
	"strings"
)

// MissingToolResultsError is returned when a prompt contains non-provider-executed
// tool calls that have no corresponding tool results or approval responses.
type MissingToolResultsError struct {
	// ToolCallIDs identifies every unresolved tool call in prompt order.
	ToolCallIDs []string
}

func (e *MissingToolResultsError) Error() string {
	if len(e.ToolCallIDs) == 1 {
		return fmt.Sprintf("aisdk: tool result is missing for tool call %s", e.ToolCallIDs[0])
	}
	return fmt.Sprintf("aisdk: tool results are missing for tool calls %s", strings.Join(e.ToolCallIDs, ", "))
}
