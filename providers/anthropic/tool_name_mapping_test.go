package anthropic

import (
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
)

func TestNewToolNameMapping_BidirectionalLookup(t *testing.T) {
	mapping := newToolNameMapping([]provider.Tool{
		provider.Tool{Type: provider.ToolTypeProvider,
			ID:   "anthropic.web_search_20250305",
			Name: "search_docs",
		},
		provider.Tool{Type: provider.ToolTypeProvider,
			ID:   "anthropic.tool_search_bm25_20251119",
			Name: "search_tools",
		},
	})

	assert.Equal(t, "web_search", mapping.toProviderToolName("search_docs"))
	assert.Equal(t, "search_docs", mapping.toCustomToolName("web_search"))
	assert.Equal(t, "tool_search_tool_bm25", mapping.toProviderToolName("search_tools"))
	assert.Equal(t, "search_tools", mapping.toCustomToolName("tool_search_tool_bm25"))
}

func TestNewToolNameMapping_UnmappedNamesPassThrough(t *testing.T) {
	mapping := newToolNameMapping(nil)

	assert.Equal(t, "future_tool", mapping.toCustomToolName("future_tool"))
	assert.Equal(t, "custom_tool", mapping.toProviderToolName("custom_tool"))
}

func TestNewToolNameMapping_OnlyProviderDefinedToolsCreateEntries(t *testing.T) {
	mapping := newToolNameMapping([]provider.Tool{
		provider.Tool{Type: provider.ToolTypeFunction,
			Name: "web_search",
		},
		provider.Tool{Type: provider.ToolTypeProvider,
			ID:   "anthropic.unknown_tool",
			Name: "unknown_custom",
		},
		provider.Tool{Type: provider.ToolTypeProvider,
			ID:   "anthropic.tool_search_regex_20251119",
			Name: "search_regex",
		},
	})

	assert.Equal(t, "web_search", mapping.toCustomToolName("web_search"))
	assert.Equal(t, "unknown_custom", mapping.toProviderToolName("unknown_custom"))
	assert.Equal(t, "tool_search_tool_regex", mapping.toProviderToolName("search_regex"))
	assert.Equal(t, "search_regex", mapping.toCustomToolName("tool_search_tool_regex"))
}

func TestNewToolNameMapping_CodeExecutionTools(t *testing.T) {
	mapping := newToolNameMapping([]provider.Tool{
		provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20250825", Name: "code_exec"},
		provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.computer_20250124", Name: "my_computer"},
		provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.text_editor_20241022", Name: "editor_old"},
		provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.text_editor_20250429", Name: "editor_new"},
		provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.bash_20250124", Name: "my_bash"},
	})

	assert.Equal(t, "code_execution", mapping.toProviderToolName("code_exec"))
	assert.Equal(t, "code_exec", mapping.toCustomToolName("code_execution"))

	assert.Equal(t, "computer", mapping.toProviderToolName("my_computer"))
	assert.Equal(t, "my_computer", mapping.toCustomToolName("computer"))

	assert.Equal(t, "str_replace_editor", mapping.toProviderToolName("editor_old"))
	assert.Equal(t, "editor_old", mapping.toCustomToolName("str_replace_editor"))

	assert.Equal(t, "str_replace_based_edit_tool", mapping.toProviderToolName("editor_new"))
	assert.Equal(t, "editor_new", mapping.toCustomToolName("str_replace_based_edit_tool"))

	assert.Equal(t, "bash", mapping.toProviderToolName("my_bash"))
	assert.Equal(t, "my_bash", mapping.toCustomToolName("bash"))
}

func TestNewToolNameMapping_WebFetchMemoryTools(t *testing.T) {
	mapping := newToolNameMapping([]provider.Tool{
		provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.memory_20250818", Name: "my_memory"},
		provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.web_search_20260209", Name: "search_v2"},
		provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.web_fetch_20250910", Name: "fetch_v1"},
		provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.web_fetch_20260209", Name: "fetch_v2"},
	})

	assert.Equal(t, "memory", mapping.toProviderToolName("my_memory"))
	assert.Equal(t, "my_memory", mapping.toCustomToolName("memory"))

	assert.Equal(t, "web_search", mapping.toProviderToolName("search_v2"))
	assert.Equal(t, "search_v2", mapping.toCustomToolName("web_search"))

	assert.Equal(t, "web_fetch", mapping.toProviderToolName("fetch_v1"))
	assert.Equal(t, "web_fetch", mapping.toProviderToolName("fetch_v2"))
	// Both map to "web_fetch" wire name; last one wins the reverse lookup
	assert.Equal(t, "fetch_v2", mapping.toCustomToolName("web_fetch"))
}

func TestNewToolNameMapping_FunctionOnlyToolsProduceEmptyMapping(t *testing.T) {
	mapping := newToolNameMapping([]provider.Tool{
		provider.Tool{Type: provider.ToolTypeFunction, Name: "search"},
		provider.Tool{Type: provider.ToolTypeFunction, Name: "web_search"},
	})

	assert.Empty(t, mapping.customToolNameToProviderToolName)
	assert.Empty(t, mapping.providerToolNameToCustomToolName)
	assert.Equal(t, "search", mapping.toProviderToolName("search"))
	assert.Equal(t, "web_search", mapping.toCustomToolName("web_search"))
}
