package provider_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderWireV4Loss_FractionalNumericSettings(t *testing.T) {
	optionsType := reflect.TypeOf(provider.CallOptions{})
	for _, fieldName := range []string{"MaxOutputTokens", "TopK", "Seed"} {
		t.Run(fieldName, func(t *testing.T) {
			field, ok := optionsType.FieldByName(fieldName)
			require.True(t, ok)
			assert.Equal(t, reflect.Pointer, field.Type.Kind())
			assert.Equal(t, reflect.Int, field.Type.Elem().Kind())
		})
	}
}

func TestProviderWireV4Loss_ExplicitFalseIncludeRawChunks(t *testing.T) {
	absent := provider.CallOptions{}
	explicitFalse := provider.CallOptions{IncludeRawChunks: false}
	assert.Equal(t, absent, explicitFalse)
}

func TestProviderWireV4Loss_ExplicitEmptyOptionalStrings(t *testing.T) {
	approved := false
	tests := []struct {
		name     string
		absent   any
		explicit any
	}{
		{
			name:     "response format name",
			absent:   provider.ResponseFormat{Type: provider.ResponseFormatJSON},
			explicit: provider.ResponseFormat{Type: provider.ResponseFormatJSON, Name: ""},
		},
		{
			name:     "response format description",
			absent:   provider.ResponseFormat{Type: provider.ResponseFormatJSON},
			explicit: provider.ResponseFormat{Type: provider.ResponseFormatJSON, Description: ""},
		},
		{
			name: "function tool description",
			absent: provider.Tool{
				Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{}`),
			},
			explicit: provider.Tool{
				Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{}`), Description: "",
			},
		},
		{
			name: "file filename",
			absent: provider.ContentPart{
				Type: provider.ContentPartTypeFile, MediaType: "text/plain",
				Data: &provider.DataContent{Text: "value"},
			},
			explicit: provider.ContentPart{
				Type: provider.ContentPartTypeFile, Filename: "", MediaType: "text/plain",
				Data: &provider.DataContent{Text: "value"},
			},
		},
		{
			name: "approval reason",
			absent: provider.ContentPart{
				Type: provider.ContentPartTypeToolApprovalResponse, ApprovalID: "approval-1", Approved: &approved,
			},
			explicit: provider.ContentPart{
				Type: provider.ContentPartTypeToolApprovalResponse, ApprovalID: "approval-1", Approved: &approved, Reason: "",
			},
		},
		{
			name: "tool result file filename",
			absent: provider.ToolResultContentValue{
				Type: provider.ToolContentFile, MediaType: "text/plain",
				Data: &provider.DataContent{Text: "value"},
			},
			explicit: provider.ToolResultContentValue{
				Type: provider.ToolContentFile, Filename: "", MediaType: "text/plain",
				Data: &provider.DataContent{Text: "value"},
			},
		},
		{
			name:     "execution denied reason",
			absent:   provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied},
			explicit: provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied, Reason: ""},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.absent, tc.explicit)
		})
	}
}

func TestProviderWireV4Loss_ExplicitFalseToolCallProviderExecuted(t *testing.T) {
	absent := provider.ContentPart{
		Type: provider.ContentPartTypeToolCall, ToolCallID: "call-1", ToolName: "lookup",
		Input: json.RawMessage(`{}`),
	}
	explicitFalse := absent
	explicitFalse.ProviderExecuted = false
	assert.Equal(t, absent, explicitFalse)
}

func TestProviderWireV4Loss_RequiredEmptyInlineTextFileData(t *testing.T) {
	direct := provider.DataContent{Text: ""}
	err := direct.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no data set")

	var decoded provider.DataContent
	require.NoError(t, json.Unmarshal([]byte(`{"type":"text","text":""}`), &decoded))
	require.NoError(t, decoded.Validate())
	assert.Equal(t, exportedDataContent(direct), exportedDataContent(decoded))
}

func exportedDataContent(data provider.DataContent) struct {
	Bytes     []byte
	Base64    string
	URL       string
	Reference json.RawMessage
	Text      string
} {
	return struct {
		Bytes     []byte
		Base64    string
		URL       string
		Reference json.RawMessage
		Text      string
	}{
		Bytes: data.Bytes, Base64: data.Base64, URL: data.URL,
		Reference: data.Reference, Text: data.Text,
	}
}
