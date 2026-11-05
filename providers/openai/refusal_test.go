package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const refusalResponse = `{"id":"resp_refusal","created_at":1,"model":"gpt-4o","object":"response","status":"completed","output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"refusal","refusal":"Cannot help."}]}],"usage":{"input_tokens":1,"output_tokens":3,"total_tokens":4,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`

func TestRefusal_NativeHTTP(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("enabled=%v/stream=%v", enabled, streaming), func(t *testing.T) {
				client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					request, err := io.ReadAll(req.Body)
					require.NoError(t, err)
					assert.NotContains(t, string(request), "preserveRefusal")
					assert.NotContains(t, string(request), "PreserveRefusal")
					body, contentType := refusalResponse, "application/json"
					if streaming {
						events := []string{
							`{"type":"response.created","sequence_number":0,"response":{"id":"resp_refusal","model":"gpt-4o","status":"in_progress","output":[]}}`,
							`{"type":"response.output_item.added","sequence_number":1,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"in_progress","content":[]}}`,
							`{"type":"response.refusal.delta","sequence_number":2,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"Cannot "}`,
							`{"type":"response.refusal.delta","sequence_number":3,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"help."}`,
							`{"type":"response.refusal.done","sequence_number":4,"item_id":"msg_1","output_index":0,"content_index":0,"refusal":"Cannot help."}`,
							`{"type":"response.output_item.done","sequence_number":5,"output_index":0,"item":{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"refusal","refusal":"Cannot help."}]}}`,
							`{"type":"response.completed","sequence_number":6,"response":` + refusalResponse + `}`,
						}
						body = ""
						for _, event := range events {
							body += "data: " + event + "\n\n"
						}
						contentType = "text/event-stream"
					}
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
				})}
				model := NewResponses("synthetic", "gpt-4o", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
				options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("test")}, ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{PreserveRefusal: enabled})}
				var metadata provider.ProviderMetadata
				if streaming {
					result, err := model.DoStream(context.Background(), options)
					require.NoError(t, err)
					var delta strings.Builder
					for part := range result.Stream {
						assert.NotEqual(t, provider.PartError, part.Type)
						if part.Type == provider.PartCustom {
							require.True(t, enabled)
							assert.Equal(t, "openai.responses-refusal", part.Kind)
							m, err := ParseResponsesMetadata(part.ProviderMetadata, "openai")
							require.NoError(t, err)
							require.NotNil(t, m.Refusal)
							delta.WriteString(*m.Refusal)
						}
						if part.Type == provider.PartTextDelta {
							t.Fatal("refusal must not become ordinary text")
						}
						if part.Type == provider.PartFinish {
							metadata = part.ProviderMetadata
						}
					}
					if enabled {
						assert.Equal(t, "Cannot help.", delta.String())
					} else {
						assert.Empty(t, delta.String())
					}
				} else {
					result, err := model.DoGenerate(context.Background(), options)
					require.NoError(t, err)
					require.Empty(t, result.Content)
					metadata = result.ProviderMetadata
				}
				m, err := ParseResponsesMetadata(metadata, "openai")
				require.NoError(t, err)
				if enabled {
					require.NotNil(t, m.Refusal)
					assert.Equal(t, "Cannot help.", *m.Refusal)
				} else {
					assert.Nil(t, m.Refusal)
				}
			})
		}
	}
}

func TestRefusal_FinalOnlyAndInvalidMetadata(t *testing.T) {
	parts := collectPartsWithBuildResult(t, buildResult{preserveRefusal: true}, `{"type":"response.completed","sequence_number":1,"response":`+refusalResponse+`}`)
	require.Len(t, parts, 3)
	assert.Equal(t, provider.PartCustom, parts[1].Type)
	m, err := ParseResponsesMetadata(parts[1].ProviderMetadata, "openai")
	require.NoError(t, err)
	require.NotNil(t, m.Refusal)
	assert.Equal(t, "Cannot help.", *m.Refusal)
	for _, raw := range []string{`{"responses":{"refusal":42}}`, `invalid`} {
		_, err := ParseResponsesMetadata(provider.ProviderMetadata{"openai": json.RawMessage(raw)}, "openai")
		require.Error(t, err)
	}
	invalid := string([]byte{0xff})
	require.Error(t, (ResponsesMetadata{Refusal: &invalid}).Validate())
}

func TestRefusal_InconsistentDoneFails(t *testing.T) {
	parts := collectPartsWithBuildResult(t, buildResult{preserveRefusal: true},
		`{"type":"response.refusal.delta","sequence_number":1,"output_index":0,"content_index":0,"item_id":"msg_1","delta":"first"}`,
		`{"type":"response.refusal.done","sequence_number":2,"output_index":0,"content_index":0,"item_id":"msg_1","refusal":"changed"}`)
	require.Contains(t, partTypes(parts), provider.PartError)
}

func TestRefusal_DoneSuffixAndMetadataIsolation(t *testing.T) {
	parts := collectPartsWithBuildResult(t, buildResult{preserveRefusal: true, providerOptionsName: "azure"},
		`{"type":"response.refusal.delta","sequence_number":1,"output_index":0,"content_index":0,"item_id":"msg_1","delta":"Cannot "}`,
		`{"type":"response.refusal.done","sequence_number":2,"output_index":0,"content_index":0,"item_id":"msg_1","refusal":"Cannot help."}`,
		`{"type":"response.completed","sequence_number":3,"response":`+refusalResponse+`}`)
	var text strings.Builder
	for _, part := range parts {
		if part.Type == provider.PartCustom {
			m, err := ParseResponsesMetadata(part.ProviderMetadata, "azure")
			require.NoError(t, err)
			require.NotNil(t, m.Refusal)
			text.WriteString(*m.Refusal)
		}
	}
	assert.Equal(t, "Cannot help.", text.String())
	original := provider.ProviderMetadata{"azure": json.RawMessage(`{"responseId":"original"}`), "other": json.RawMessage(`{"keep":true}`)}
	refusal := "No"
	enriched := withResponsesMetadata(original, "azure", ResponsesMetadata{Refusal: &refusal})
	assert.JSONEq(t, `{"responseId":"original"}`, string(original["azure"]))
	assert.JSONEq(t, `{"responseId":"original","responses":{"refusal":"No"}}`, string(enriched["azure"]))
	assert.Equal(t, original["other"], enriched["other"])
}
