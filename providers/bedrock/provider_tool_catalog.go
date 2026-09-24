package bedrock

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed provider_tool_schemas.json
var providerToolSchemasJSON []byte

var loadProviderToolSchemas = sync.OnceValues(func() (map[string]json.RawMessage, error) {
	return decodeProviderToolSchemas(providerToolSchemasJSON)
})

func decodeProviderToolSchemas(data []byte) (map[string]json.RawMessage, error) {
	var schemas map[string]json.RawMessage
	if err := json.Unmarshal(data, &schemas); err != nil {
		return nil, fmt.Errorf("bedrock: decoding provider-tool schemas: %w", err)
	}
	return schemas, nil
}

func providerToolSchema(id string) (json.RawMessage, error) {
	schemas, err := loadProviderToolSchemas()
	if err != nil {
		return nil, err
	}
	schema, ok := schemas[id]
	if !ok {
		return nil, fmt.Errorf("bedrock: missing provider-tool schema for %q", id)
	}
	return schema, nil
}

var anthropicProviderToolBetas = map[string]string{
	"anthropic.advisor_20260301":           "advisor-tool-2026-03-01",
	"anthropic.bash_20241022":              "computer-use-2024-10-22",
	"anthropic.bash_20250124":              "computer-use-2025-01-24",
	"anthropic.code_execution_20250522":    "code-execution-2025-05-22",
	"anthropic.code_execution_20250825":    "code-execution-2025-08-25",
	"anthropic.code_execution_20260120":    "",
	"anthropic.computer_20241022":          "computer-use-2024-10-22",
	"anthropic.computer_20250124":          "computer-use-2025-01-24",
	"anthropic.computer_20251124":          "computer-use-2025-11-24",
	"anthropic.memory_20250818":            "context-management-2025-06-27",
	"anthropic.text_editor_20241022":       "computer-use-2024-10-22",
	"anthropic.text_editor_20250124":       "computer-use-2025-01-24",
	"anthropic.text_editor_20250429":       "computer-use-2025-01-24",
	"anthropic.text_editor_20250728":       "",
	"anthropic.tool_search_bm25_20251119":  "",
	"anthropic.tool_search_regex_20251119": "",
	"anthropic.web_fetch_20250910":         "web-fetch-2025-09-10",
	"anthropic.web_fetch_20260209":         "code-execution-web-tools-2026-02-09",
	"anthropic.web_search_20260209":        "code-execution-web-tools-2026-02-09",
}
