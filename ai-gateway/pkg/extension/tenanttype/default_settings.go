package tenanttype

import "encoding/json"

var defaultSettings = newType(
	"default-settings",
	"Default sampling and limit settings for calls that leave them unset.",
	`{
  "type": "object",
  "properties": {
    "temperature": {
      "type": "number",
      "description": "The sampling temperature.",
      "minimum": -1.7976931348623157e308,
      "maximum": 1.7976931348623157e308
    },
    "topP": {
      "type": "number",
      "description": "The nucleus sampling probability mass.",
      "minimum": -1.7976931348623157e308,
      "maximum": 1.7976931348623157e308
    },
    "topK": {
      "type": "integer",
      "description": "The number of most likely tokens to sample from.",
      "minimum": -9223372036854775808,
      "maximum": 9223372036854775807
    },
    "maxOutputTokens": {
      "type": "integer",
      "description": "The maximum number of tokens to generate.",
      "minimum": 1,
      "maximum": 9223372036854775807
    },
    "stopSequences": {
      "type": "array",
      "description": "Sequences that stop generation.",
      "items": {
        "type": "string"
      }
    },
    "seed": {
      "type": "integer",
      "description": "The seed for deterministic sampling, where the model supports it.",
      "minimum": -9223372036854775808,
      "maximum": 9223372036854775807
    },
    "presencePenalty": {
      "type": "number",
      "description": "The presence penalty.",
      "minimum": -1.7976931348623157e308,
      "maximum": 1.7976931348623157e308
    },
    "frequencyPenalty": {
      "type": "number",
      "description": "The frequency penalty.",
      "minimum": -1.7976931348623157e308,
      "maximum": 1.7976931348623157e308
    }
  },
  "minProperties": 1,
  "additionalProperties": false
}`,
	json.RawMessage(`{"temperature":0.2,"maxOutputTokens":2048,"stopSequences":["END"]}`),
)

// DefaultSettings returns the default-settings type. Its member,
// defaultSettings, sets temperature, topP, topK, maxOutputTokens,
// stopSequences, seed, presencePenalty and frequencyPenalty for calls that
// leave them unset. It accepts no other call parameter and is off by
// default.
func DefaultSettings() Type { return defaultSettings.clone() }
