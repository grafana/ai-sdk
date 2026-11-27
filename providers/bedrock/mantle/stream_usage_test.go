package mantle

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewResponses_StreamTextAndUsageThroughMantleClient(t *testing.T) {
	const (
		modelID = "openai.gpt-5.6-luna"
		baseURL = "https://mantle.example.test/openai/v1"
		usage   = `{"input_tokens":13,"input_tokens_details":{"cached_tokens":4,"cache_write_tokens":2},"output_tokens":7,"output_tokens_details":{"reasoning_tokens":3},"total_tokens":20,"future_counter":9}`
		message = `{"id":"msg_mantle_usage","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"first second","annotations":[]}]}`
		events  = `data: {"type":"response.created","sequence_number":0,"response":{"id":"resp_mantle_usage","model":"openai.gpt-5.6-luna","created_at":1700000000,"object":"response","status":"in_progress","output":[]}}

data: {"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"id":"msg_mantle_usage","type":"message","role":"assistant","status":"in_progress","content":[]}}

data: {"type":"response.output_text.delta","sequence_number":2,"output_index":0,"content_index":0,"item_id":"msg_mantle_usage","delta":"first","logprobs":[]}

data: {"type":"response.output_text.delta","sequence_number":3,"output_index":0,"content_index":0,"item_id":"msg_mantle_usage","delta":" second","logprobs":[]}

data: {"type":"response.output_item.done","sequence_number":4,"output_index":0,"item":` + message + `}

data: {"type":"response.completed","sequence_number":5,"response":{"id":"resp_mantle_usage","model":"openai.gpt-5.6-luna","created_at":1700000000,"object":"response","status":"completed","output":[` + message + `],"usage":` + usage + `}}

`
	)

	var bodies [][]byte
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		defer func() { _ = req.Body.Close() }()
		assert.Equal(t, http.MethodPost, req.Method)
		assert.Equal(t, baseURL+"/responses", req.URL.String())
		assert.Equal(t, "Bearer offline-mantle", req.Header.Get("Authorization"))
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		bodies = append(bodies, body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"text/event-stream"}, "X-Request-Id": {"mantle-usage"}},
			Body:       io.NopCloser(strings.NewReader(events)),
			Request:    req,
		}, nil
	})}
	model, err := NewResponses(t.Context(), modelID, Config{BaseURL: baseURL, AWSRegion: "us-west-2", APIKey: "offline-mantle"},
		option.WithHTTPClient(client), option.WithMaxRetries(0), option.WithJSONSet("metadata.capture", "mantle-usage"))
	require.NoError(t, err)

	result, err := model.DoStream(t.Context(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
	require.NoError(t, err)
	var parts []provider.StreamPart
	var types []provider.StreamPartType
	for part := range result.Stream {
		parts = append(parts, part)
		types = append(types, part.Type)
	}

	require.Len(t, bodies, 1)
	require.NotNil(t, result.Request)
	assert.JSONEq(t, string(bodies[0]), string(result.Request.Body))
	var wire struct {
		Model    string            `json:"model"`
		Stream   bool              `json:"stream"`
		Metadata map[string]string `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(bodies[0], &wire))
	assert.Equal(t, modelID, wire.Model)
	assert.True(t, wire.Stream)
	assert.Equal(t, "mantle-usage", wire.Metadata["capture"])

	require.Equal(t, []provider.StreamPartType{
		provider.PartStreamStart, provider.PartResponseMeta, provider.PartTextStart,
		provider.PartTextDelta, provider.PartTextDelta, provider.PartTextEnd, provider.PartFinish,
	}, types)
	meta, start, first, second, end, finish := parts[1], parts[2], parts[3], parts[4], parts[5], parts[6]
	require.NotNil(t, result.Response)
	assert.Equal(t, "mantle-usage", result.Response.Headers["X-Request-Id"])
	assert.Equal(t, result.Response.Headers, meta.ResponseHeaders)
	assert.Equal(t, "bedrock-mantle.responses", meta.Provider)
	assert.Equal(t, modelID, meta.ModelID)
	assert.Equal(t, "resp_mantle_usage", meta.ResponseID)
	for _, part := range []provider.StreamPart{start, first, second, end} {
		assert.Equal(t, "msg_mantle_usage", part.ID)
	}
	assert.Equal(t, "first", first.Delta)
	assert.Equal(t, " second", second.Delta)
	assert.JSONEq(t, `{"itemId":"msg_mantle_usage"}`, string(end.ProviderMetadata["openai"]))
	assert.JSONEq(t, `{"responseId":"resp_mantle_usage"}`, string(finish.ProviderMetadata["openai"]))
	assert.Equal(t, &provider.FinishReason{Unified: provider.FinishReasonStop}, finish.FinishReason)
	require.NotNil(t, finish.Usage)
	assert.Equal(t, provider.InputTokenUsage{Total: new(13), NoCache: new(7), CacheRead: new(4), CacheWrite: new(2)}, finish.Usage.InputTokens)
	assert.Equal(t, provider.OutputTokenUsage{Total: new(7), Text: new(4), Reasoning: new(3)}, finish.Usage.OutputTokens)
	assert.JSONEq(t, usage, string(finish.Usage.Raw))
}
