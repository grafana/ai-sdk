package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateFileInputs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prompt []Message
		want   bool
	}{
		{name: "ordinary file", prompt: []Message{NewUserMessage(FilePart("text/plain", TextDataContent("")))}},
		{name: "reasoning empty data", prompt: []Message{NewAssistantMessage(ReasoningFilePart("image/png", Base64DataContent("")))}},
		{name: "reasoning URL", prompt: []Message{NewAssistantMessage(ReasoningFilePart("image/png", URLDataContent("https://example.test/file")))}},
		{name: "reasoning wrong role", prompt: []Message{NewUserMessage(ReasoningFilePart("image/png", Base64DataContent("")))}, want: true},
		{name: "reasoning missing data", prompt: []Message{NewAssistantMessage(ContentPart{Type: ContentPartTypeReasoningFile})}, want: true},
		{name: "reasoning filename", prompt: []Message{NewAssistantMessage(ContentPart{Type: ContentPartTypeReasoningFile, Data: &DataContent{Bytes: []byte{}}, Filename: new("")})}, want: true},
		{name: "reasoning kind", prompt: []Message{NewAssistantMessage(ContentPart{Type: ContentPartTypeReasoningFile, Data: &DataContent{Bytes: []byte{}}, Kind: "image"})}, want: true},
		{name: "reasoning text", prompt: []Message{NewAssistantMessage(ReasoningFilePart("text/plain", TextDataContent("")))}, want: true},
		{name: "reasoning reference", prompt: []Message{NewAssistantMessage(ReasoningFilePart("image/png", ReferenceDataContent(json.RawMessage(`{}`))))}, want: true},
		{name: "reasoning mixed arms", prompt: []Message{NewAssistantMessage(ReasoningFilePart("image/png", DataContent{Bytes: []byte{}, URL: "https://example.test/file"}))}, want: true},
		{name: "file result", prompt: []Message{NewToolMessage(ToolResultPart("call-1", "tool", &ToolResultOutput{
			Type: ToolOutputContent, Content: []ToolResultContentValue{{Type: ToolContentFile, Data: dataPtr(Base64DataContent(""))}},
		}))}},
		{name: "missing ordinary data", prompt: []Message{NewUserMessage(ContentPart{Type: ContentPartTypeFile})}, want: true},
		{name: "mixed ordinary arms", prompt: []Message{NewUserMessage(FilePart("text/plain", DataContent{Text: "text", URL: "https://example.test"}))}, want: true},
		{name: "invalid ordinary reference", prompt: []Message{NewUserMessage(FilePart("application/pdf", ReferenceDataContent(json.RawMessage(`{"openai":null}`))))}, want: true},
		{name: "invalid nested file", prompt: []Message{NewToolMessage(ToolResultPart("call-1", "tool", &ToolResultOutput{
			Type: ToolOutputContent, Content: []ToolResultContentValue{{Type: ToolContentFile, Data: dataPtr(DataContent{Bytes: []byte{}, URL: "https://example.test"})}},
		}))}, want: true},
		{name: "empty reference is structurally valid", prompt: []Message{NewUserMessage(FilePart("application/pdf", ReferenceDataContent(json.RawMessage(`{}`))))}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ValidateFileInputs(tc.prompt) != nil)
		})
	}
}

func dataPtr(data DataContent) *DataContent { return &data }
