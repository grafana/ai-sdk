package openai

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponses_InputTransformations(t *testing.T) {
	const entries = `[{"id":"rs_1","type":"reasoning","action":"removed","reason":"prefix_mismatch"},{"type":"future_type","action":"future_action","reason":"future_reason"},{"id":null,"type":"reasoning","action":"removed","reason":"model_mismatch"}]`
	const entriesWithExtra = `[{"id":"rs_1","type":"reasoning","action":"removed","reason":"prefix_mismatch","extension":true},{"type":"future_type","action":"future_action","reason":"future_reason"},{"id":null,"type":"reasoning","action":"removed","reason":"model_mismatch"}]`
	for _, tc := range []struct {
		name string
		wire string
		want string
	}{
		{name: "entries", wire: entriesWithExtra, want: entries},
		{name: "empty", wire: `[]`, want: `[]`},
		{name: "null", wire: `null`},
		{name: "absent"},
	} {
		for _, mode := range []struct {
			name         string
			finish       provider.FinishReason
			statusFields string
		}{
			{name: "generate", statusFields: `"status":"completed"`},
			{name: "completed", statusFields: `"status":"completed"`, finish: provider.FinishReason{Unified: provider.FinishReasonStop}},
			{name: "incomplete", statusFields: `"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}`, finish: provider.FinishReason{Unified: provider.FinishReasonLength, Raw: "max_output_tokens"}},
			{name: "failed", statusFields: `"status":"failed","error":{"code":"server_error","message":"processing failed"}`, finish: provider.FinishReason{Unified: provider.FinishReasonError, Raw: "error"}},
		} {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				response := `{"id":"resp_1","created_at":0,"model":"gpt-4.1-mini","output":[],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5},"service_tier":"default","reasoning":{"context":"all"},` + mode.statusFields
				if tc.wire != "" {
					response += `,"input_transformations":` + tc.wire
				}
				response += `}`
				model := inputTransformationsTestModel(t, "openai", mode.name, response)
				opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}}
				var metadata provider.ProviderMetadata
				if mode.name == "generate" {
					result, err := model.DoGenerate(t.Context(), opts)
					require.NoError(t, err)
					metadata = result.ProviderMetadata
				} else {
					result, err := model.DoStream(t.Context(), opts)
					require.NoError(t, err)
					var errors, finishes int
					for part := range result.Stream {
						switch part.Type {
						case provider.PartError:
							errors++
						case provider.PartFinish:
							finishes++
							metadata = part.ProviderMetadata
							assert.Equal(t, &mode.finish, part.FinishReason)
						}
					}
					assert.Equal(t, 1, finishes)
					if mode.name == "failed" {
						assert.Equal(t, 1, errors)
					} else {
						assert.Zero(t, errors)
					}
				}
				expected := `{"responseId":"resp_1","serviceTier":"default","reasoningContext":"all"`
				if tc.want != "" {
					expected += `,"inputTransformations":` + tc.want
				}
				require.Len(t, metadata, 1)
				assert.JSONEq(t, expected+`}`, string(metadata["openai"]))
			})
		}
	}
}

func TestResponses_InputTransformationsTextFacade(t *testing.T) {
	model := inputTransformationsTestModel(t, "azure.responses", "completed", `{"id":"resp_1","status":"completed","output":[],"input_transformations":[]}`)
	want := `{"responseId":"resp_1","inputTransformations":[]}`
	streamed := aisdk.StreamText(t.Context(), model, aisdk.WithModelMessages(provider.UserText("hi")))
	var steps int
	for part := range streamed.FullStream() {
		switch step := part.(type) {
		case aisdk.StreamFinishStep:
			steps++
			require.Len(t, step.ProviderMetadata, 1)
			assert.JSONEq(t, want, string(step.ProviderMetadata["azure"]))
		}
	}
	require.NoError(t, streamed.Err())
	assert.Equal(t, 1, steps)
	assert.Equal(t, "answer", streamed.Text())
	require.Len(t, streamed.ProviderMetadata(), 1)
	assert.JSONEq(t, want, string(streamed.ProviderMetadata()["azure"]))

	generated, err := aisdk.GenerateText(t.Context(), model, aisdk.WithModelMessages(provider.UserText("hi")))
	require.NoError(t, err)
	assert.Equal(t, "answer", generated.Text)
	require.Len(t, generated.ProviderMetadata, 1)
	assert.JSONEq(t, want, string(generated.ProviderMetadata["azure"]))
}

func inputTransformationsTestModel(t *testing.T, identity, mode, response string) provider.LanguageModel {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if mode == "generate" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, response)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, transportSSE([]string{
			`{"type":"response.created","sequence_number":0,"response":{"id":"resp_1","created_at":0,"model":"gpt-4.1-mini","input_transformations":[]}}`,
			`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}`,
			`{"type":"response.output_text.delta","sequence_number":2,"output_index":0,"content_index":0,"item_id":"msg_1","delta":"answer"}`,
			`{"type":"response.output_item.done","sequence_number":3,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"answer","annotations":[]}]}}`,
			fmt.Sprintf(`{"type":"response.%s","sequence_number":4,"response":%s}`, mode, response),
		}))
	}))
	t.Cleanup(server.Close)
	return NewResponses("test", "gpt-4.1-mini", WithProviderName(identity), WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
}
