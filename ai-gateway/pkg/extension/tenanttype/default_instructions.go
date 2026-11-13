package tenanttype

import "encoding/json"

var defaultInstructions = newType(
	"default-instructions",
	"Default system messages for calls that have none.",
	`{
  "type": "object",
  "properties": {
    "messages": {
      "type": "array",
      "description": "System messages prepended, in order, to calls whose prompt has no system message.",
      "minItems": 1,
      "items": {
        "type": "object",
        "properties": {
          "content": {
            "type": "string",
            "description": "The system message text.",
            "minLength": 1
          }
        },
        "required": ["content"],
        "additionalProperties": false
      }
    }
  },
  "required": ["messages"],
  "additionalProperties": false
}`,
	json.RawMessage(`{"messages":[{"content":"You are an assistant for this Grafana stack. Answer concisely."}]}`),
)

// DefaultInstructions returns the default-instructions type. Its member,
// defaultInstructions, lists system messages, each with a content string,
// that are prepended in order to calls whose prompt has no system message.
// It is off by default.
func DefaultInstructions() Type { return defaultInstructions.clone() }
