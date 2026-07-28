package aisdk

import (
	"errors"

	"github.com/grafana/ai-sdk/provider"
)

// ErrNoObjectGenerated is returned when structured output validation fails.
// The LLM's raw text response is still available via result.Text().
var ErrNoObjectGenerated = errors.New("aisdk: no object generated")

// Output defines a structured output specification that controls how
// LLM responses are formatted, parsed, and validated.
//
// Implementations are provided by the output package: Object[T], Array[T],
// Choice, JSON, and Text.
type Output interface {
	// ResponseFormat returns the provider response format configuration
	// to be sent with the LLM request.
	ResponseFormat() *provider.ResponseFormat

	// ParseComplete validates and parses the complete LLM response text.
	// Returns the parsed value (type depends on the Output mode) or an error
	// if the response is invalid.
	ParseComplete(text string) (any, error)

	// ParsePartial attempts a best-effort parse of incomplete JSON text
	// during streaming. Returns the partial value and true if parsing
	// succeeded, or (nil, false) if the text is not yet parseable.
	ParsePartial(text string) (any, bool)
}
