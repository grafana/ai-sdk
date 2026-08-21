package provider_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderWireV4Contract_NumericSettings(t *testing.T) {
	optionsType := reflect.TypeOf(provider.CallOptions{})
	for _, fieldName := range []string{"MaxOutputTokens", "TopK", "Seed"} {
		t.Run(fieldName, func(t *testing.T) {
			field, ok := optionsType.FieldByName(fieldName)
			require.True(t, ok)
			assert.Equal(t, reflect.Pointer, field.Type.Kind())
			assert.Equal(t, reflect.TypeOf(provider.LanguageModelNumber{}), field.Type.Elem())
		})
	}

	fraction, err := provider.LanguageModelNumberFromFloat64(1.5)
	require.NoError(t, err)
	value, ok := fraction.Float64()
	require.True(t, ok)
	assert.Equal(t, 1.5, value)

	large := provider.LanguageModelNumberFromInt64(9007199254740993)
	valueInt, ok := large.Int64()
	require.True(t, ok)
	assert.Equal(t, int64(9007199254740993), valueInt)
	_, ok = large.Float64()
	assert.False(t, ok)
}

func TestProviderWireV4Contract_IncludeRawChunksPresence(t *testing.T) {
	explicitFalseValue := false
	absent := provider.CallOptions{}
	explicitFalse := provider.CallOptions{IncludeRawChunks: &explicitFalseValue}
	assert.NotEqual(t, absent, explicitFalse)
}

func TestProviderWireV4Contract_OptionalStringPresence(t *testing.T) {
	empty := ""
	approved := false
	tests := []struct {
		name     string
		absent   any
		explicit any
	}{
		{
			name:     "response format name",
			absent:   provider.ResponseFormat{Type: provider.ResponseFormatJSON},
			explicit: provider.ResponseFormat{Type: provider.ResponseFormatJSON, Name: &empty},
		},
		{
			name:     "response format description",
			absent:   provider.ResponseFormat{Type: provider.ResponseFormatJSON},
			explicit: provider.ResponseFormat{Type: provider.ResponseFormatJSON, Description: &empty},
		},
		{
			name: "function tool description",
			absent: provider.Tool{
				Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{}`),
			},
			explicit: provider.Tool{
				Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{}`), Description: &empty,
			},
		},
		{
			name: "file filename",
			absent: provider.ContentPart{
				Type: provider.ContentPartTypeFile, MediaType: "text/plain",
				Data: dataContentPointer(provider.TextDataContent("value")),
			},
			explicit: provider.ContentPart{
				Type: provider.ContentPartTypeFile, FilePartFilename: &empty, MediaType: "text/plain",
				Data: dataContentPointer(provider.TextDataContent("value")),
			},
		},
		{
			name: "approval reason",
			absent: provider.ContentPart{
				Type: provider.ContentPartTypeToolApprovalResponse, ApprovalID: "approval-1", Approved: &approved,
			},
			explicit: provider.ContentPart{
				Type: provider.ContentPartTypeToolApprovalResponse, ApprovalID: "approval-1", Approved: &approved, Reason: &empty,
			},
		},
		{
			name: "tool result file filename",
			absent: provider.ToolResultContentValue{
				Type: provider.ToolContentFile, MediaType: "text/plain",
				Data: dataContentPointer(provider.TextDataContent("value")),
			},
			explicit: provider.ToolResultContentValue{
				Type: provider.ToolContentFile, Filename: &empty, MediaType: "text/plain",
				Data: dataContentPointer(provider.TextDataContent("value")),
			},
		},
		{
			name:     "execution denied reason",
			absent:   provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied},
			explicit: provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied, Reason: &empty},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotEqual(t, tc.absent, tc.explicit)
		})
	}
}

func TestProviderWireV4Contract_ToolCallProviderExecutedPresence(t *testing.T) {
	absent := provider.ContentPart{
		Type: provider.ContentPartTypeToolCall, ToolCallID: "call-1", ToolName: "lookup",
		Input: json.RawMessage(`{}`),
	}
	explicitFalseValue := false
	explicitFalse := absent
	explicitFalse.ProviderExecuted = &explicitFalseValue
	assert.NotEqual(t, absent, explicitFalse)
}

func TestProviderWireV4Contract_EmptyInlineTextFileData(t *testing.T) {
	data := provider.TextDataContent("")
	require.NoError(t, data.Validate())
	dataType, ok := data.DataType()
	require.True(t, ok)
	assert.Equal(t, provider.DataContentTypeText, dataType)

	encoded, err := json.Marshal(data)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"text","text":""}`, string(encoded))
}

func dataContentPointer(value provider.DataContent) *provider.DataContent { return &value }
