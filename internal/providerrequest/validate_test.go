package providerrequest

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
)

func TestValidate_RequestArms(t *testing.T) {
	data := provider.TextDataContent("value")
	empty := ""
	valid := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewSystemMessage("system"),
			provider.NewUserMessage(provider.FilePartWithFilename("text/plain", data, "")),
			provider.NewAssistantMessage(provider.ToolCallPart("call-1", "lookup", json.RawMessage(`{}`))),
			provider.NewToolMessage(provider.ToolResultPart("call-1", "lookup", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"})),
		},
		Tools: []provider.Tool{
			{Type: provider.ToolTypeFunction, Name: "lookup", Description: &empty, InputSchema: json.RawMessage(`{}`)},
			{Type: provider.ToolTypeProvider, Name: "search", ID: "provider.search", ProviderOptions: provider.ProviderOptions{
				"provider": provider.RawProviderOption{Key: "provider", Raw: json.RawMessage(`{"enabled":true}`)},
			}},
		},
	}
	require.NoError(t, Validate(valid))

	invalid := []provider.CallOptions{
		{Prompt: []provider.Message{{Role: provider.RoleSystem, Content: []provider.ContentPart{provider.TextPart("one"), provider.TextPart("two")}}}},
		{Prompt: []provider.Message{{Role: provider.Role("unsupported")}}},
		{Prompt: []provider.Message{{}}},
		{Prompt: []provider.Message{provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeFile, Data: &data, MediaType: "text/plain", Filename: "response.txt"})}},
		{Prompt: []provider.Message{provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeText, Text: "value", ToolName: "inactive"})}},
		{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup", ID: "inactive"}}},
		{ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatText, Name: &empty}},
	}
	for _, options := range invalid {
		require.Error(t, Validate(options))
	}
}

func TestValidate_DomainStructFieldExhaustiveness(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  []string
	}{
		{
			name:  "content part",
			value: provider.ContentPart{},
			want: []string{
				"Type", "Text", "Data", "FilePartFilename", "Filename", "MediaType", "Kind", "SourceType", "ID", "URL", "Title",
				"ToolCallID", "ToolName", "Input", "Output", "ProviderExecuted", "ApprovalID", "Signature", "IsAutomatic", "Approved", "Reason", "ProviderOptions",
			},
		},
		{
			name:  "tool",
			value: provider.Tool{},
			want:  []string{"Type", "Name", "Description", "InputSchema", "InputExamples", "Strict", "ID", "Args", "ProviderOptions"},
		},
		{
			name:  "tool result output",
			value: provider.ToolResultOutput{},
			want:  []string{"Type", "Text", "JSON", "Content", "Reason", "ProviderOptions"},
		},
		{
			name:  "tool result content",
			value: provider.ToolResultContentValue{},
			want:  []string{"Type", "Text", "Data", "MediaType", "Filename", "ProviderOptions"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			typeOf := reflect.TypeOf(tc.value)
			fields := make([]string, typeOf.NumField())
			for index := range typeOf.NumField() {
				fields[index] = typeOf.Field(index).Name
			}
			require.Equal(t, tc.want, fields, "update providerrequest validation for the changed domain struct")
		})
	}
}

func TestValidate_InvalidNumbers(t *testing.T) {
	invalid := provider.LanguageModelNumber{}
	tests := []struct {
		name    string
		options provider.CallOptions
	}{
		{name: "max output tokens", options: provider.CallOptions{MaxOutputTokens: &invalid}},
		{name: "top k", options: provider.CallOptions{TopK: &invalid}},
		{name: "seed", options: provider.CallOptions{Seed: &invalid}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, Validate(tc.options))
		})
	}
}
