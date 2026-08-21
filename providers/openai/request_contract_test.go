package openai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openAIStringPointer(value string) *string { return &value }

func openAIBoolPointer(value bool) *bool { return &value }

func openAIIntegerPointer(value int64) *provider.LanguageModelNumber {
	number := provider.LanguageModelNumberFromInt64(value)
	return &number
}

func openAIFloatPointer(t *testing.T, value float64) *provider.LanguageModelNumber {
	t.Helper()
	number, err := provider.LanguageModelNumberFromFloat64(value)
	require.NoError(t, err)
	return &number
}

func TestBuildParams_ExactMaxOutputTokens(t *testing.T) {
	tests := []struct {
		name   string
		number *provider.LanguageModelNumber
		want   string
	}{
		{name: "integer", number: openAIIntegerPointer(1024), want: `"max_output_tokens":1024`},
		{name: "fraction", number: openAIFloatPointer(t, 1024.5), want: `"max_output_tokens":1024.5`},
		{name: "large integer", number: openAIIntegerPointer(9007199254740993), want: `"max_output_tokens":9007199254740993`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _, _, err := buildParams("gpt-4.1", provider.CallOptions{MaxOutputTokens: tc.number})
			require.NoError(t, err)
			encoded, err := json.Marshal(body)
			require.NoError(t, err)
			text := string(encoded)
			assert.Contains(t, text, tc.want)
			assert.Equal(t, 1, strings.Count(text, `"max_output_tokens"`))
		})
	}
}

func TestBuildParams_EmptyFileDataArms(t *testing.T) {
	t.Run("empty image data is serialized", func(t *testing.T) {
		data := provider.BytesDataContent(nil)
		body, _ := buildBody(t, "gpt-4.1", provider.CallOptions{
			Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("image/png", data))},
		})
		input := body["input"].([]any)
		content := input[0].(map[string]any)["content"].([]any)
		assert.Equal(t, "data:image/png;base64,", content[0].(map[string]any)["image_url"])
	})

	t.Run("empty PDF data is serialized", func(t *testing.T) {
		data := provider.BytesDataContent(nil)
		body, _ := buildBody(t, "gpt-4.1", provider.CallOptions{
			Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("application/pdf", data))},
		})
		input := body["input"].([]any)
		content := input[0].(map[string]any)["content"].([]any)
		assert.Equal(t, "data:application/pdf;base64,", content[0].(map[string]any)["file_data"])
	})

	t.Run("empty text arm is explicitly unsupported", func(t *testing.T) {
		data := provider.TextDataContent("")
		_, _, _, err := buildParams("gpt-4.1", provider.CallOptions{
			Prompt: []provider.Message{provider.NewUserMessage(provider.FilePart("text/plain", data))},
		})
		require.ErrorContains(t, err, "text file parts are not supported")
	})
}

func TestBuildParams_CustomToolFileFilenamePresence(t *testing.T) {
	noStore := false
	values := []*string{nil, openAIStringPointer(""), openAIStringPointer("report.pdf")}
	want := []string{"data", "", "report.pdf"}
	content := make([]provider.ToolResultContentValue, len(values))
	for index, filename := range values {
		content[index] = provider.ToolResultContentValue{
			Type: provider.ToolContentFile, Data: dataContentPointer(provider.Base64DataContent("ZGF0YQ==")),
			MediaType: "application/pdf", Filename: filename,
		}
	}
	body, _ := buildBody(t, "gpt-5", provider.CallOptions{
		Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call-custom", "custom", &provider.ToolResultOutput{
			Type: provider.ToolOutputContent, Content: content,
		}))},
		Tools:           []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDCustom, Name: "custom"}},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &noStore}),
	})
	output := findInput(body, "custom_tool_call_output")["output"].([]any)
	require.Len(t, output, 3)
	for index, filename := range want {
		assert.Equal(t, filename, output[index].(map[string]any)["filename"])
	}
}

func TestBuildParams_RejectsInvalidRequestArms(t *testing.T) {
	data := provider.TextDataContent("value")
	invalidNumber := provider.LanguageModelNumber{}
	invalid := []provider.CallOptions{
		{Prompt: []provider.Message{{Role: provider.Role("unsupported")}}},
		{TopK: &invalidNumber},
		{Prompt: []provider.Message{provider.NewUserMessage(provider.ContentPart{
			Type: provider.ContentPartTypeFile, Data: &data, MediaType: "text/plain", Filename: "response.txt",
		})}},
	}
	for _, options := range invalid {
		_, _, _, err := buildParams("gpt-4.1", options)
		require.ErrorContains(t, err, "invalid")
	}
}

func dataContentPointer(value provider.DataContent) *provider.DataContent { return &value }
