package conformance

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	openaiprovider "github.com/grafana/ai-sdk/providers/openai"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/azure"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type azureResponsesTransportFunc func(*http.Request) (*http.Response, error)

func (f azureResponsesTransportFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestAzureNativeClient_ResponsesTransport(t *testing.T) {
	const (
		endpoint = "https://azure.example.test"
		version  = "2025-04-01-preview"
		modelID  = "test-deployment"
		response = `{"id":"resp_azure_native","object":"response","status":"completed","created_at":1700000000,"model":"gpt-4o","output":[{"id":"msg_azure_native","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"azure reply","annotations":[]}]}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`
		events   = `data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_azure_native","object":"response","status":"in_progress","created_at":1700000000,"model":"gpt-4o","output":[]}}

data: {"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"id":"msg_azure_native","type":"message","role":"assistant","status":"in_progress","content":[]}}

data: {"type":"response.output_text.delta","sequence_number":2,"output_index":0,"content_index":0,"item_id":"msg_azure_native","delta":"azure reply","logprobs":[]}

data: {"type":"response.output_item.done","sequence_number":3,"output_index":0,"item":{"id":"msg_azure_native","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"azure reply","annotations":[]}]}}

data: {"type":"response.completed","sequence_number":4,"response":` + response + `}

`
	)

	for _, tc := range []struct {
		name   string
		stream bool
	}{
		{name: "unary"},
		{name: "stream", stream: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var bodies [][]byte
			httpClient := &http.Client{Transport: azureResponsesTransportFunc(func(req *http.Request) (*http.Response, error) {
				defer func() { _ = req.Body.Close() }()
				assert.Equal(t, http.MethodPost, req.Method)
				assert.Equal(t, endpoint+"/openai/responses?api-version="+version, req.URL.String())
				assert.Equal(t, "offline-azure", req.Header.Get("Api-Key"))
				assert.Empty(t, req.Header.Get("Authorization"))
				body, err := io.ReadAll(req.Body)
				if err != nil {
					return nil, err
				}
				bodies = append(bodies, body)
				payload, contentType := response, "application/json"
				if tc.stream {
					payload, contentType = events, "text/event-stream"
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": {contentType}, "X-Request-Id": {"azure-" + tc.name}},
					Body:       io.NopCloser(strings.NewReader(payload)),
					Request:    req,
				}, nil
			})}
			client := openaisdk.NewClient(
				azure.WithEndpoint(endpoint, version),
				azure.WithAPIKey("offline-azure"),
				option.WithHTTPClient(httpClient),
				option.WithMaxRetries(0),
				option.WithJSONSet("metadata.capture", tc.name),
			)
			model := openaiprovider.NewResponsesWithClient(client, modelID, openaiprovider.WithProviderName("azure.responses"))
			opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}}

			var request *provider.RequestMetadata
			var headers map[string]string
			if tc.stream {
				result, err := model.DoStream(t.Context(), opts)
				require.NoError(t, err)
				request = result.Request
				require.NotNil(t, result.Response)
				headers = result.Response.Headers
				var parts []provider.StreamPart
				var types []provider.StreamPartType
				for part := range result.Stream {
					parts = append(parts, part)
					types = append(types, part.Type)
				}
				require.Equal(t, []provider.StreamPartType{
					provider.PartStreamStart, provider.PartResponseMeta, provider.PartTextStart,
					provider.PartTextDelta, provider.PartTextEnd, provider.PartFinish,
				}, types)
				meta, delta, finish := parts[1], parts[3], parts[5]
				assert.Equal(t, "resp_azure_native", meta.ResponseID)
				assert.Equal(t, "gpt-4o", meta.ModelID)
				assert.Equal(t, "azure.responses", meta.Provider)
				assert.Equal(t, headers, meta.ResponseHeaders)
				assert.Equal(t, "msg_azure_native", delta.ID)
				assert.Equal(t, "azure reply", delta.Delta)
				assert.Equal(t, &provider.FinishReason{Unified: provider.FinishReasonStop}, finish.FinishReason)
				assert.JSONEq(t, `{"responseId":"resp_azure_native"}`, string(finish.ProviderMetadata["azure"]))
			} else {
				result, err := model.DoGenerate(t.Context(), opts)
				require.NoError(t, err)
				require.NotNil(t, result.Response)
				request, headers = result.Request, result.Response.Headers
				assert.Equal(t, "resp_azure_native", result.Response.ID)
				assert.Equal(t, "gpt-4o", result.Response.ModelID)
				assert.Equal(t, "azure.responses", result.Response.Provider)
				assert.JSONEq(t, response, string(result.Response.Body))
				require.Len(t, result.Content, 1)
				assert.Equal(t, "azure reply", result.Content[0].Text)
				assert.Equal(t, provider.FinishReason{Unified: provider.FinishReasonStop}, result.FinishReason)
				assert.JSONEq(t, `{"responseId":"resp_azure_native"}`, string(result.ProviderMetadata["azure"]))
			}
			require.Len(t, bodies, 1)
			require.NotNil(t, request)
			assert.JSONEq(t, string(bodies[0]), string(request.Body))
			assert.Equal(t, "azure-"+tc.name, headers["X-Request-Id"])
			var wire struct {
				Model    string            `json:"model"`
				Stream   *bool             `json:"stream"`
				Metadata map[string]string `json:"metadata"`
			}
			require.NoError(t, json.Unmarshal(bodies[0], &wire))
			assert.Equal(t, modelID, wire.Model)
			assert.Equal(t, tc.name, wire.Metadata["capture"])
			if tc.stream {
				assert.Equal(t, new(true), wire.Stream)
			} else {
				assert.Nil(t, wire.Stream)
			}
		})
	}
}
