package providerwire

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyRequestAdapter_RedesignedValues(t *testing.T) {
	fraction, err := provider.LanguageModelNumberFromFloat64(1.5)
	require.NoError(t, err)
	empty := ""
	explicitFalse := false
	data := provider.TextDataContent("")
	options := provider.CallOptions{
		MaxOutputTokens:  &fraction,
		TopK:             &fraction,
		Seed:             &fraction,
		IncludeRawChunks: &explicitFalse,
		ResponseFormat: &provider.ResponseFormat{
			Type: provider.ResponseFormatJSON, Name: &empty, Description: &empty,
		},
		Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "tool", Description: &empty}},
		Prompt: []provider.Message{
			provider.NewUserMessage(provider.FilePartWithFilename("text/plain", data, "")),
			provider.NewAssistantMessage(provider.ContentPart{
				Type: provider.ContentPartTypeToolCall, ToolCallID: "call", ToolName: "tool",
				ProviderExecuted: &explicitFalse,
			}),
			provider.NewToolMessage(
				provider.ContentPart{Type: provider.ContentPartTypeToolApprovalResponse, ApprovalID: "approval", Reason: &empty},
				provider.ToolResultPart("call", "tool", &provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied, Reason: &empty}),
				provider.ToolResultPart("file", "tool", &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{
					Type: provider.ToolContentFile, Data: dataPointer(provider.TextDataContent("value")), MediaType: "text/plain", Filename: &empty,
				}}}),
			),
		},
	}

	encoded, err := EncodeCallOptions(options)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"maxOutputTokens":1.5`)
	assert.Contains(t, string(encoded), `"includeRawChunks":false`)
	assert.Contains(t, string(encoded), `"filename":""`)
	decoded, err := DecodeCallOptions(encoded)
	require.NoError(t, err)
	assert.Equal(t, options, decoded)
}

func TestLegacyRequestAdapter_EmptyDataArms(t *testing.T) {
	for _, data := range []provider.DataContent{
		provider.BytesDataContent(nil),
		provider.Base64DataContent(""),
		provider.URLDataContent(""),
		provider.TextDataContent(""),
		provider.ReferenceDataContent(json.RawMessage(`{}`)),
	} {
		options := provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("text/plain", data))}}
		encoded, err := EncodeCallOptions(options)
		require.NoError(t, err)
		decoded, err := DecodeCallOptions(encoded)
		require.NoError(t, err)
		require.Len(t, decoded.Prompt, 1)
		decodedType, ok := decoded.Prompt[0].Content[0].Data.DataType()
		require.True(t, ok)
		wantType, ok := data.DataType()
		require.True(t, ok)
		assert.Equal(t, wantType, decodedType)
	}
}

func TestLegacyRequestAdapter_ExplicitEmptyCollectionDecodeAndParentEncode(t *testing.T) {
	decoded, err := DecodeCallOptions([]byte(`{"prompt":[],"tools":[],"stopSequences":[],"headers":{},"providerOptions":{}}`))
	require.NoError(t, err)
	assert.NotNil(t, decoded.Prompt)
	assert.NotNil(t, decoded.Tools)
	assert.NotNil(t, decoded.StopSequences)
	assert.NotNil(t, decoded.Headers)
	assert.NotNil(t, decoded.ProviderOptions)

	encoded, err := EncodeCallOptions(decoded)
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(encoded))
}

func TestLegacyRequestAdapter_ToleratesArbitraryReferenceJSON(t *testing.T) {
	body := []byte(`{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"reference","reference":{"count":1,"nested":[true,null]}},"mediaType":"application/json"}]}]}`)
	decoded, err := DecodeCallOptions(body)
	require.NoError(t, err)
	require.Len(t, decoded.Prompt, 1)
	assert.JSONEq(t, `{"count":1,"nested":[true,null]}`, string(decoded.Prompt[0].Content[0].Data.Reference))
	reencoded, err := EncodeCallOptions(decoded)
	require.NoError(t, err)
	assert.Equal(t, string(body), string(reencoded))
}

func TestLegacyRequestAdapter_RejectsIntrinsicInvalidValues(t *testing.T) {
	_, err := EncodeCallOptions(provider.CallOptions{MaxOutputTokens: &provider.LanguageModelNumber{}})
	require.ErrorContains(t, err, "invalid language model number")

	_, err = DecodeCallOptions([]byte(`{"maxOutputTokens":1e400}`))
	require.Error(t, err)

	data := provider.TextDataContent("value")
	_, err = EncodeCallOptions(provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.ContentPart{
		Type: provider.ContentPartTypeFile, Data: &data, MediaType: "text/plain",
		FilePartFilename: ptrString("request.txt"), Filename: "response.txt",
	})}})
	require.ErrorContains(t, err, "both request and response filenames")
}

func dataPointer(value provider.DataContent) *provider.DataContent { return &value }
