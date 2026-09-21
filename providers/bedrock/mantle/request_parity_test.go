package mantle

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	openaiprovider "github.com/grafana/ai-sdk/providers/openai"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mantleSchemaOptions() provider.CallOptions {
	schema := json.RawMessage(`{"type":"object","propertyNames":{"type":"string","pattern":"^[A-Z]+$"}}`)
	return provider.CallOptions{
		Prompt:         []provider.Message{provider.UserText("Hi")},
		ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema},
		Tools: []provider.Tool{
			{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: schema, ProviderOptions: provider.BuildProviderOptions(openaiprovider.OpenAIToolOptions{OutputSchema: schema})},
			{Type: provider.ToolTypeFunction, Name: "nested", InputSchema: schema, ProviderOptions: provider.BuildProviderOptions(openaiprovider.OpenAIToolOptions{OutputSchema: schema, Namespace: &openaiprovider.OpenAIToolNamespaceOptions{Name: "ns", Description: "tools"}})},
			{Type: provider.ToolTypeProvider, Name: "search", ID: "openai.web_search"},
			{Type: provider.ToolTypeProvider, Name: "remote", ID: "openai.mcp", Args: map[string]json.RawMessage{"serverLabel": json.RawMessage(`"server-a"`), "serverUrl": json.RawMessage(`"https://example.com/mcp"`)}},
		},
		ProviderOptions: provider.BuildProviderOptions(openaiprovider.OpenAIResponsesOptions{AllowedTools: &openaiprovider.AllowedToolsOption{Mode: "required", ToolNames: []string{"search", "remote", "lookup", "search"}}}),
	}
}

func TestNewResponses_RequestParity(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "generate", true: "stream"}[streaming], func(t *testing.T) {
			var body map[string]any
			var compactResponse bytes.Buffer
			require.NoError(t, json.Compact(&compactResponse, []byte(responseWithoutOutput)))
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "https://provider.example.test/openai/v1/responses", req.URL.String())
				require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
				if streaming {
					return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.created\",\"response\":" + compactResponse.String() + "}\n\ndata: {\"type\":\"response.completed\",\"response\":" + compactResponse.String() + "}\n\n")), Request: req}, nil
				}
				return jsonHTTPResponse(req, http.StatusOK, responseWithMessage), nil
			})}
			model, err := NewResponses(t.Context(), "openai.gpt-5.6-luna", Config{BaseURL: "https://provider.example.test/openai/v1", SkipAuth: true}, option.WithHTTPClient(client), option.WithMaxRetries(0))
			require.NoError(t, err)
			opts := mantleSchemaOptions()
			before, err := json.Marshal(opts)
			require.NoError(t, err)
			var warnings []provider.Warning
			var metadata provider.ProviderMetadata
			if streaming {
				result, err := model.DoStream(t.Context(), opts)
				require.NoError(t, err)
				for part := range result.Stream {
					assert.NotEqual(t, provider.PartError, part.Type)
					switch part.Type {
					case provider.PartStreamStart:
						warnings = part.Warnings
					case provider.PartResponseMeta:
						assert.Equal(t, "bedrock-mantle.responses", part.Provider)
					case provider.PartFinish:
						metadata = part.ProviderMetadata
					}
				}
			} else {
				result, err := model.DoGenerate(t.Context(), opts)
				require.NoError(t, err)
				assert.Equal(t, "bedrock-mantle.responses", result.Response.Provider)
				warnings = result.Warnings
				metadata = result.ProviderMetadata
			}
			assert.Equal(t, "bedrock-mantle.responses", model.Provider())
			assert.Equal(t, "openai.gpt-5.6-luna", model.ModelID())
			assert.Equal(t, "openai.gpt-5.6-luna", body["model"])
			assert.Contains(t, metadata, "openai")
			assert.NotContains(t, metadata, "bedrock-mantle.responses")
			require.Len(t, warnings, 5)
			for _, warning := range warnings {
				assert.Equal(t, provider.WarnCompatibility, warning.Type)
				assert.Equal(t, "JSON Schema propertyNames", warning.Feature)
			}
			choice, err := json.Marshal(body["tool_choice"])
			require.NoError(t, err)
			assert.JSONEq(t, `{"type":"allowed_tools","mode":"required","tools":[{"type":"web_search"},{"type":"mcp","server_label":"server-a"},{"type":"function","name":"lookup"},{"type":"web_search"}]}`, string(choice))
			assert.Equal(t, map[string]any{"type": "object"}, body["text"].(map[string]any)["format"].(map[string]any)["schema"])
			tools := body["tools"].([]any)
			require.Len(t, tools, 4)
			for i := range 2 {
				tool := tools[i].(map[string]any)
				if i == 1 {
					tool = tool["tools"].([]any)[0].(map[string]any)
				}
				assert.Equal(t, map[string]any{"type": "object"}, tool["parameters"])
				assert.Equal(t, map[string]any{"type": "object"}, tool["output_schema"])
			}
			after, err := json.Marshal(opts)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after))
		})
	}
}

func TestNewResponses_RequestParityRejectsBeforeHTTP(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		return jsonHTTPResponse(req, http.StatusOK, responseWithoutOutput), nil
	})}
	model, err := NewResponses(t.Context(), "openai.gpt-5.6-luna", Config{BaseURL: "https://provider.example.test/openai/v1", SkipAuth: true}, option.WithHTTPClient(client), option.WithMaxRetries(0))
	require.NoError(t, err)
	invalidSchema := json.RawMessage(`{"type":"object","propertyNames":false}`)
	for _, site := range []string{"empty allowed", "dropped allowed", "response", "input", "output", "namespace input", "namespace output"} {
		t.Run(site, func(t *testing.T) {
			opts := mantleSchemaOptions()
			want := "propertyNames that does not use a string schema"
			switch site {
			case "empty allowed", "dropped allowed":
				names := []string{}
				if site == "dropped allowed" {
					names = []string{"nested"}
				}
				opts.ProviderOptions = provider.BuildProviderOptions(openaiprovider.OpenAIResponsesOptions{AllowedTools: &openaiprovider.AllowedToolsOption{ToolNames: names}})
				want = "allowedTools"
			case "response":
				opts.ResponseFormat.Schema = invalidSchema
			default:
				i := 0
				if strings.HasPrefix(site, "namespace") {
					i = 1
				}
				if strings.HasSuffix(site, "input") {
					opts.Tools[i].InputSchema = invalidSchema
				} else {
					toolOptions := openaiprovider.OpenAIToolOptions{OutputSchema: invalidSchema}
					if i == 1 {
						toolOptions.Namespace = &openaiprovider.OpenAIToolNamespaceOptions{Name: "ns", Description: "tools"}
					}
					opts.Tools[i].ProviderOptions = provider.BuildProviderOptions(toolOptions)
				}
			}
			_, err := model.DoGenerate(t.Context(), opts)
			require.ErrorContains(t, err, want)
			_, err = model.DoStream(t.Context(), opts)
			require.ErrorContains(t, err, want)
		})
	}
	assert.Zero(t, requests)
}
