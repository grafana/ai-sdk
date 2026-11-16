package chatcompletions

import (
	"encoding/json"

	"github.com/grafana/ai-sdk/schema"
)

func compileSchema(raw json.RawMessage) (*schema.CompiledSchema, error) {
	if !jsonObject(raw) {
		return nil, errRequest
	}
	compiled, err := schema.CompileSelfContainedSchema(raw)
	if err != nil {
		return nil, errRequest
	}
	return compiled, nil
}
