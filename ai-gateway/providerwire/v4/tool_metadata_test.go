package v4

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolMetadata_OpaqueTransport(t *testing.T) {
	metadata := provider.ProviderMetadata{
		"anthropic": json.RawMessage(`{"caller":{"type":"code_execution_20260120","toolId":"parent","secret":"private"},"secret":"private"}`),
		"openai":    json.RawMessage(`{"itemId":"item-1","namespace":"tools","caller":{"type":"program","callerId":"parent","private":"hidden"},"backendModel":"private"}`),
		"private":   json.RawMessage(`{"credential":"private"}`),
	}
	mapped, err := mapToolMetadataForTest(metadata, 1024)
	require.NoError(t, err)
	require.Len(t, mapped, 3)
	assert.Equal(t, metadata, mapped)
}

func TestToolMetadata_OptionalFieldsAndExtensions(t *testing.T) {
	metadata := provider.ProviderMetadata{
		"anthropic": json.RawMessage(`{"type":"other","caller":{"type":"direct","callerId":"private"},"credential":"private"}`),
		"openai":    json.RawMessage(`{"itemId":"","namespace":"","caller":{"type":"direct","toolId":"private"},"credential":"private"}`),
		"azure":     json.RawMessage(`{"private":"hidden"}`),
	}
	mapped, err := mapToolMetadataForTest(metadata, 1024)
	require.NoError(t, err)
	require.Len(t, mapped, 3)
	assert.Equal(t, metadata, mapped)
}

func TestToolMetadata_OpaqueSemanticFields(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata provider.ProviderMetadata
		limit    int64
	}{
		{name: "unsupported MCP metadata", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"type":"mcp-tool-use","serverName":"other"}`)}, limit: 1024},
		{name: "unsupported caller", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":{"type":"unknown"}}`)}, limit: 1024},
		{name: "null caller", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":null}`)}, limit: 1024},
		{name: "null type", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"type":null}`)}, limit: 1024},
		{name: "missing anthropic tool id", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":{"type":"code_execution_20260120"}}`)}, limit: 1024},
		{name: "null anthropic tool id", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":{"type":"code_execution_20260120","toolId":null}}`)}, limit: 1024},
		{name: "direct caller with null id", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":{"type":"direct","toolId":null}}`)}, limit: 1024},
		{name: "empty anthropic tool id", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":{"type":"code_execution_20260120","toolId":""}}`)}, limit: 1024},
		{name: "missing openai caller id", metadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"caller":{"type":"program"}}`)}, limit: 1024},
		{name: "empty azure caller id", metadata: provider.ProviderMetadata{"azure": json.RawMessage(`{"caller":{"type":"program","callerId":""}}`)}, limit: 1024},
		{name: "wrong item id type", metadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":42}`)}, limit: 1024},
		{name: "null item id", metadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":null}`)}, limit: 1024},
		{name: "null namespace", metadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"namespace":null}`)}, limit: 1024},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mapped, err := mapToolMetadataForTest(tc.metadata, tc.limit)
			require.NoError(t, err)
			assert.Equal(t, tc.metadata, mapped)
		})
	}
}

func TestToolMetadata_StructuralAndEncodingBounds(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata provider.ProviderMetadata
		limit    int64
	}{
		{name: "null known namespace", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`null`)}, limit: 1024},
		{name: "encoded response exceeds limit", metadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"<&>"}`)}, limit: 25},
		{name: "malformed metadata", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{`)}, limit: 1024},
		{name: "oversized known field", metadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"type":"mcp-tool-use","serverName":"` + strings.Repeat("x", 200) + `"}`)}, limit: 128},
		{name: "oversized unknown namespace", metadata: provider.ProviderMetadata{"private": json.RawMessage(`{"secret":"` + strings.Repeat("x", 200) + `"}`)}, limit: 128},
		{name: "excess cardinality", metadata: provider.ProviderMetadata{"one": nil, "two": nil}, limit: 1},
		{name: "array namespace", metadata: provider.ProviderMetadata{"future": json.RawMessage(`[]`)}, limit: 1024},
		{name: "invalid UTF-8 namespace", metadata: provider.ProviderMetadata{"future": json.RawMessage([]byte{'{', '"', 0xff, '"', ':', '0', '}'})}, limit: 1024},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := mapToolMetadataForTest(tc.metadata, tc.limit)
			require.Error(t, err)
		})
	}
}

func mapToolMetadataForTest(metadata provider.ProviderMetadata, limit int64) (provider.ProviderMetadata, error) {
	result := validGenerateResult()
	result.Content = []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "echo", Input: json.RawMessage("{}"), ProviderMetadata: metadata}}
	mapped, err := mapUnarySuccess(result, limit)
	if err != nil {
		return nil, err
	}
	body, ok := encodeUnarySuccess(mapped, limit)
	if !ok {
		return nil, errInvalidUnarySuccess
	}
	var response struct {
		Content []struct {
			Metadata provider.ProviderMetadata `json:"providerMetadata"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	return response.Content[0].Metadata, nil
}
