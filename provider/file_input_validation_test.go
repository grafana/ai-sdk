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
