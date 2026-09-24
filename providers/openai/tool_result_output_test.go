package openai

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resultBody(t *testing.T, namespace string, opts provider.CallOptions) (map[string]any, []provider.Warning) {
	t.Helper()
	body, warnings, _, err := buildParamsForProvider("gpt-4o", opts, namespace)
	require.NoError(t, err)
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	return decoded, warnings
}

func resultOptions(t *testing.T, namespace string, breakpoint bool) provider.ProviderOptions {
	t.Helper()
	options := OpenAIPartOptions{}
	if breakpoint {
		options.PromptCacheBreakpoint = &PromptCacheBreakpoint{Mode: "explicit"}
	}
	if namespace == "azure" {
		return withAzureOptions(t, options)
	}
	return provider.BuildProviderOptions(options)
}

func TestScalarResultBreakpoint_Precedence(t *testing.T) {
	for _, namespace := range []string{"openai", "azure"} {
		t.Run(namespace, func(t *testing.T) {
			outputBreakpoint := &PromptCacheBreakpoint{Mode: "explicit"}
			partBreakpoint := &PromptCacheBreakpoint{Mode: "part"}
			options := func(breakpoint *PromptCacheBreakpoint) provider.ProviderOptions {
				value := OpenAIPartOptions{PromptCacheBreakpoint: breakpoint}
				if namespace == "azure" {
					return withAzureOptions(t, value)
				}
				return provider.BuildProviderOptions(value)
			}
			part := provider.ToolResultPart("call_1", "search", &provider.ToolResultOutput{Type: provider.ToolOutputText, ProviderOptions: options(outputBreakpoint)})
			part.ProviderOptions = options(partBreakpoint)
			ctx := inputConversionContext{providerOptionsName: namespace}
			assert.Equal(t, outputBreakpoint, ctx.scalarResultBreakpoint(part))
			part.Output.ProviderOptions = nil
			assert.Equal(t, partBreakpoint, ctx.scalarResultBreakpoint(part))
			part.Output.Type = provider.ToolOutputContent
			assert.Nil(t, ctx.scalarResultBreakpoint(part))
		})
	}
}

func TestBuildParams_FunctionResultScalarBreakpoints(t *testing.T) {
	for _, namespace := range []string{"openai", "azure"} {
		for _, tc := range []struct {
			name   string
			output provider.ToolResultOutput
			want   string
		}{
			{name: "text", output: provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"}, want: `"ok"`},
			{name: "error text", output: provider.ToolResultOutput{Type: provider.ToolOutputErrorText, Text: "failed"}, want: `"failed"`},
			{name: "json", output: provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"ok":true}`)}, want: `{"ok":true}`},
			{name: "error json", output: provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"error":"no"}`)}, want: `{"error":"no"}`},
			{name: "denied", output: provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied}, want: `"Tool call execution denied."`},
		} {
			t.Run(namespace+"/"+tc.name, func(t *testing.T) {
				for _, location := range []string{"none", "part", "output", "both"} {
					t.Run(location, func(t *testing.T) {
						output := tc.output
						output.ProviderOptions = resultOptions(t, namespace, location == "output" || location == "both")
						part := provider.ToolResultPart("call_1", "search", &output)
						part.ProviderOptions = resultOptions(t, namespace, location == "part" || location == "both")
						body, warnings := resultBody(t, namespace, provider.CallOptions{
							Prompt: []provider.Message{provider.NewToolMessage(part)},
							Tools:  []provider.Tool{{Type: provider.ToolTypeFunction, Name: "search", ProviderOptions: provider.BuildProviderOptions(OpenAIToolOptions{OutputSchema: json.RawMessage(`{"type":"string"}`)})}},
						})
						assert.Empty(t, warnings)
						item := body["input"].([]any)[0].(map[string]any)
						assert.Equal(t, "function_call_output", item["type"])
						assert.Equal(t, "call_1", item["call_id"])
						if location == "none" {
							assert.Equal(t, tc.want, item["output"])
						} else {
							assert.Equal(t, []any{map[string]any{"type": "input_text", "text": tc.want, "prompt_cache_breakpoint": map[string]any{"mode": "explicit"}}}, item["output"])
						}
					})
				}
			})
		}
	}
}

func TestBuildParams_FunctionResultMultipart(t *testing.T) {
	for _, namespace := range []string{"openai", "azure"} {
		t.Run(namespace, func(t *testing.T) {
			partOptions := resultOptions(t, namespace, true)
			imageOptions := OpenAIPartOptions{ImageDetail: "high", PromptCacheBreakpoint: &PromptCacheBreakpoint{Mode: "explicit"}}
			var imageProviderOptions provider.ProviderOptions
			if namespace == "azure" {
				imageProviderOptions = withAzureOptions(t, imageOptions)
			} else {
				imageProviderOptions = provider.BuildProviderOptions(imageOptions)
			}
			output := &provider.ToolResultOutput{
				Type:            provider.ToolOutputContent,
				ProviderOptions: partOptions,
				Content: []provider.ToolResultContentValue{
					{Type: provider.ToolContentText, Text: "first", ProviderOptions: partOptions},
					{Type: provider.ToolContentFile, Data: &provider.DataContent{URL: "https://example.com/image.png"}, MediaType: "image/png", ProviderOptions: imageProviderOptions},
					{Type: provider.ToolContentFile, Data: &provider.DataContent{Bytes: []byte("img")}, MediaType: "image/png", ProviderOptions: partOptions},
					{Type: provider.ToolContentFile, Data: &provider.DataContent{Reference: json.RawMessage(`{"openai":"file-openai","azure":"file-azure"}`)}, MediaType: "image/png", ProviderOptions: partOptions},
					{Type: provider.ToolContentFile, Data: &provider.DataContent{URL: "https://example.com/file.pdf"}, MediaType: "application/pdf"},
					{Type: provider.ToolContentFile, Data: &provider.DataContent{Bytes: []byte("doc")}, MediaType: "text/plain"},
					{Type: provider.ToolContentFile, Data: &provider.DataContent{Reference: json.RawMessage(`{"openai":"pdf-openai","azure":"pdf-azure"}`)}, MediaType: "application/pdf"},
					{Type: provider.ToolContentCustom},
					{Type: provider.ToolContentFile, Data: &provider.DataContent{Text: "unsupported"}, MediaType: "text/plain"},
					{Type: provider.ToolContentText, Text: "last"},
				},
			}
			part := provider.ToolResultPart("call_1", "search", output)
			part.ProviderOptions = partOptions
			body, warnings := resultBody(t, namespace, provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(part)}})
			require.Len(t, warnings, 2)
			assert.Equal(t, "unsupported tool content part type: custom", warnings[0].Message)
			assert.Equal(t, "unsupported tool content part type: file with data type: text", warnings[1].Message)
			item := body["input"].([]any)[0].(map[string]any)
			assert.Equal(t, "call_1", item["call_id"])
			content := item["output"].([]any)
			require.Len(t, content, 8)
			assert.Equal(t, "first", content[0].(map[string]any)["text"])
			assert.Equal(t, "https://example.com/image.png", content[1].(map[string]any)["image_url"])
			assert.Equal(t, "high", content[1].(map[string]any)["detail"])
			assert.Equal(t, "data:image/png;base64,aW1n", content[2].(map[string]any)["image_url"])
			assert.Equal(t, map[string]any{"openai": "file-openai", "azure": "file-azure"}[namespace], content[3].(map[string]any)["file_id"])
			assert.Equal(t, "https://example.com/file.pdf", content[4].(map[string]any)["file_url"])
			assert.Equal(t, "data:text/plain;base64,ZG9j", content[5].(map[string]any)["file_data"])
			assert.Equal(t, "data", content[5].(map[string]any)["filename"])
			assert.Equal(t, map[string]any{"openai": "pdf-openai", "azure": "pdf-azure"}[namespace], content[6].(map[string]any)["file_id"])
			assert.Equal(t, "last", content[7].(map[string]any)["text"])
			for _, index := range []int{0, 1, 2, 3} {
				assert.Equal(t, map[string]any{"mode": "explicit"}, content[index].(map[string]any)["prompt_cache_breakpoint"])
			}
			for _, index := range []int{4, 5, 6, 7} {
				assert.NotContains(t, content[index].(map[string]any), "prompt_cache_breakpoint")
			}
		})
	}
}

func TestBuildParams_FunctionResultMediaType(t *testing.T) {
	part := provider.ToolResultPart("call_1", "search", &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{
		{Type: provider.ToolContentFile, Data: &provider.DataContent{Bytes: []byte("hello")}, MediaType: "text/plain; charset=utf-8"},
	}})
	body, warnings := resultBody(t, "openai", provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(part)}})
	assert.Empty(t, warnings)
	output := body["input"].([]any)[0].(map[string]any)["output"].([]any)
	require.Len(t, output, 1)
	assert.Equal(t, "data:text/plain; charset=utf-8;base64,aGVsbG8=", output[0].(map[string]any)["file_data"])

	part.Output.Content[0] = provider.ToolResultContentValue{Type: provider.ToolContentFile, Data: &provider.DataContent{Bytes: []byte("unrecognized")}, MediaType: "image"}
	_, _, _, err := buildParamsForProvider("gpt-4o", provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(part)}}, "openai")
	require.ErrorContains(t, err, "could not be auto-detected")
}

func TestBuildParams_UnsupportedEmptyTextFileData(t *testing.T) {
	var value provider.ToolResultContentValue
	require.NoError(t, json.Unmarshal([]byte(`{"type":"file","data":{"type":"text","text":""},"mediaType":"text/plain"}`), &value))
	for _, tc := range []struct {
		name    string
		tools   []provider.Tool
		warning string
	}{
		{name: "function", warning: "unsupported tool content part type: file with data type: text"},
		{name: "custom", tools: []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDCustom, Name: "write_sql"}}, warning: "unsupported custom tool content part type: file with data type: text"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			part := provider.ToolResultPart("call_1", "write_sql", &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{value}})
			body, warnings := resultBody(t, "openai", provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(part)}, Tools: tc.tools})
			require.Len(t, warnings, 1)
			assert.Equal(t, tc.warning, warnings[0].Message)
			assert.Empty(t, body["input"].([]any)[0].(map[string]any)["output"])
		})
	}
}

func TestBuildParams_FunctionResultMissingReference(t *testing.T) {
	_, _, _, err := buildParamsForProvider("gpt-4o", provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call_1", "search", &provider.ToolResultOutput{
		Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{Type: provider.ToolContentFile, MediaType: "application/pdf", Data: &provider.DataContent{Reference: json.RawMessage(`{"azure":"file-1"}`)}}},
	}))}}, "openai")
	require.ErrorContains(t, err, `file reference has no "openai" provider entry`)
}

func TestBuildParams_CustomResultParity(t *testing.T) {
	breakpoint := resultOptions(t, "openai", true)
	plain := provider.ToolResultPart("call_custom", "write_sql", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "plain"})
	plainBody, plainWarnings := resultBody(t, "openai", provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(plain)}, Tools: []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDCustom, Name: "write_sql"}}})
	assert.Empty(t, plainWarnings)
	assert.Equal(t, "plain", plainBody["input"].([]any)[0].(map[string]any)["output"])
	for _, tc := range []struct {
		output provider.ToolResultOutput
		want   string
	}{
		{output: provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok", ProviderOptions: breakpoint}, want: "ok"},
		{output: provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"ok":true}`)}, want: `{"ok":true}`},
		{output: provider.ToolResultOutput{Type: provider.ToolOutputErrorText, Text: "failed"}, want: "failed"},
		{output: provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`{"error":true}`)}, want: `{"error":true}`},
		{output: provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied}, want: "Tool call execution denied."},
	} {
		out := tc.output
		part := provider.ToolResultPart("call_custom", "write_sql", &out)
		part.ProviderOptions = breakpoint
		body, warnings := resultBody(t, "openai", provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(part)}, Tools: []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDCustom, Name: "write_sql"}}})
		assert.Empty(t, warnings)
		item := body["input"].([]any)[0].(map[string]any)
		assert.Equal(t, "custom_tool_call_output", item["type"])
		assert.Equal(t, "call_custom", item["call_id"])
		require.IsType(t, []any{}, item["output"])
		content := item["output"].([]any)
		require.Len(t, content, 1)
		assert.Equal(t, map[string]any{"mode": "explicit"}, content[0].(map[string]any)["prompt_cache_breakpoint"])
		assert.Equal(t, tc.want, content[0].(map[string]any)["text"])
	}
	part := provider.ToolResultPart("call_custom", "write_sql", &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{
		{Type: provider.ToolContentText, Text: "first", ProviderOptions: breakpoint},
		{Type: provider.ToolContentFile, Data: &provider.DataContent{Reference: json.RawMessage(`{"openai":"file_123"}`)}, MediaType: "application/pdf"},
		{Type: provider.ToolContentFile, Data: &provider.DataContent{URL: "https://example.com/doc.pdf"}, MediaType: "application/pdf"},
		{Type: provider.ToolContentFile, Data: &provider.DataContent{Bytes: []byte("img")}, MediaType: "image/png"},
	}})
	body, warnings := resultBody(t, "openai", provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(part)}, Tools: []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDCustom, Name: "write_sql"}}})
	require.Len(t, warnings, 1)
	assert.Equal(t, "unsupported custom tool content part type: file with data type: reference", warnings[0].Message)
	content := body["input"].([]any)[0].(map[string]any)["output"].([]any)
	require.Len(t, content, 3)
	assert.Equal(t, "first", content[0].(map[string]any)["text"])
	assert.Equal(t, "https://example.com/doc.pdf", content[1].(map[string]any)["file_url"])
	assert.Equal(t, "data:image/png;base64,aW1n", content[2].(map[string]any)["image_url"])
}
