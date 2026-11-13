package calldefaults

import (
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
)

// Definitions returns the definitions of the shipped tenant extensions.
func Definitions() []extension.Definition {
	return []extension.Definition{instructionsDefinition(), settingsDefinition()}
}
