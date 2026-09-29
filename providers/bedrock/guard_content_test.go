package bedrock

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	_ provider.ProviderOption = TextPartOptions{}
	_ provider.ProviderOption = ImagePartOptions{}
)

func rawPartOptions(key, value string) provider.ProviderOptions {
	return provider.ProviderOptions{key: provider.RawProviderOption{Key: key, Raw: json.RawMessage(value)}}
}

func withOptions(part provider.ContentPart, opts provider.ProviderOptions) provider.ContentPart {
	part.ProviderOptions = opts
	return part
}

func guardImage(opts provider.ProviderOptions) provider.ContentPart {
	return withOptions(provider.FilePart("image/png", provider.DataContent{Base64: "aGVsbG8="}), opts)
}

func requestContentJSON(t *testing.T, parts ...provider.ContentPart) string {
	t.Helper()
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(parts...)}})
	data, err := json.Marshal(req.Messages[0].Content)
	require.NoError(t, err)
	return string(data)
}

func TestGuardContent_SelectiveParts(t *testing.T) {
	guarded := withOptions(provider.TextPart("source"), rawPartOptions("amazonBedrock", `{"guardContent":true,"guardContentQualifiers":["grounding_source","query","guard_content"]}`))
	empty := withOptions(provider.TextPart(""), rawPartOptions("bedrock", `{"guardContent":true,"guardContentQualifiers":[]}`))
	actual := requestContentJSON(t, guarded, provider.TextPart("plain"),
		withOptions(provider.TextPart("false"), rawPartOptions("bedrock", `{"guardContent":false}`)),
		empty, guardImage(rawPartOptions("amazonBedrock", `{"guardContent":true}`)),
		guardImage(rawPartOptions("amazonBedrock", `{"guardContent":false}`)))
	assert.JSONEq(t, `[
		{"guardContent":{"text":{"text":"source","qualifiers":["grounding_source","query","guard_content"]}}},
		{"text":"plain"},{"text":"false"},{"guardContent":{"text":{"text":"","qualifiers":[]}}},
		{"guardContent":{"image":{"format":"png","source":{"bytes":"aGVsbG8="}}}},
		{"image":{"format":"png","source":{"bytes":"aGVsbG8="}}}
	]`, actual)
}

func TestGuardContent_NamespaceAndTypedOptions(t *testing.T) {
	flag := true
	falseFlag := false
	qualifiers := []GuardContentQualifier{GuardContentQualifierQuery}
	tests := []struct {
		name string
		part provider.ContentPart
		want string
	}{
		{"typed text", withOptions(provider.TextPart("x"), provider.BuildProviderOptions(TextPartOptions{GuardContent: &flag, GuardContentQualifiers: &qualifiers})), `[{"guardContent":{"text":{"text":"x","qualifiers":["query"]}}}]`},
		{"typed empty qualifiers", withOptions(provider.TextPart("x"), provider.BuildProviderOptions(TextPartOptions{GuardContent: &flag, GuardContentQualifiers: &[]GuardContentQualifier{}})), `[{"guardContent":{"text":{"text":"x","qualifiers":[]}}}]`},
		{"typed absent qualifiers", withOptions(provider.TextPart("x"), provider.BuildProviderOptions(TextPartOptions{GuardContent: &flag})), `[{"guardContent":{"text":{"text":"x"}}}]`},
		{"typed image", guardImage(provider.BuildProviderOptions(ImagePartOptions{GuardContent: &flag})), `[{"guardContent":{"image":{"format":"png","source":{"bytes":"aGVsbG8="}}}}]`},
		{"typed false", withOptions(provider.TextPart("x"), provider.BuildProviderOptions(TextPartOptions{GuardContent: &falseFlag, GuardContentQualifiers: &qualifiers})), `[{"text":"x"}]`},
		{"unrelated typed file option", withOptions(provider.TextPart("x"), provider.BuildProviderOptions(FilePartOptions{Citations: &FilePartCitations{Enabled: true}})), `[{"text":"x"}]`},
		{"unrelated typed file option image", guardImage(provider.BuildProviderOptions(FilePartOptions{Citations: &FilePartCitations{Enabled: true}})), `[{"image":{"format":"png","source":{"bytes":"aGVsbG8="}}}]`},
		{"unrelated typed text option image", guardImage(provider.BuildProviderOptions(TextPartOptions{GuardContentQualifiers: &qualifiers})), `[{"image":{"format":"png","source":{"bytes":"aGVsbG8="}}}]`},
		{"document with typed image option", withOptions(provider.FilePart("text/plain", provider.DataContent{Text: "hi"}), provider.BuildProviderOptions(ImagePartOptions{GuardContent: &flag})), `[{"document":{"format":"txt","name":"document-1","source":{"bytes":"aGk="}}}]`},
		{"document with typed text option", withOptions(provider.FilePart("text/plain", provider.DataContent{Text: "hi"}), provider.BuildProviderOptions(TextPartOptions{GuardContent: &flag})), `[{"document":{"format":"txt","name":"document-1","source":{"bytes":"aGk="}}}]`},
		{"document with typed citations", withOptions(provider.FilePart("text/plain", provider.DataContent{Text: "hi"}), provider.BuildProviderOptions(FilePartOptions{Citations: &FilePartCitations{Enabled: true}})), `[{"document":{"format":"txt","name":"document-1","source":{"bytes":"aGk="},"citations":{"enabled":true}}}]`},
		{"S3 image with guard option", withOptions(provider.FilePart("image/png", provider.DataContent{URL: "s3://bucket/image.png"}), provider.BuildProviderOptions(ImagePartOptions{GuardContent: &flag})), `[{"image":{"format":"png","source":{"s3Location":{"uri":"s3://bucket/image.png"}}}}]`},
		{"video with guard option", withOptions(provider.FilePart("video/mp4", provider.DataContent{Base64: "aGVsbG8="}), provider.BuildProviderOptions(ImagePartOptions{GuardContent: &flag})), `[{"video":{"format":"mp4","source":{"bytes":"aGVsbG8="}}}]`},
		{"legacy fallback from null", withOptions(provider.TextPart("x"), provider.ProviderOptions{"amazonBedrock": provider.RawProviderOption{Key: "amazonBedrock", Raw: json.RawMessage(`null`)}, "bedrock": provider.RawProviderOption{Key: "bedrock", Raw: json.RawMessage(`{"guardContent":true}`)}}), `[{"guardContent":{"text":{"text":"x"}}}]`},
		{"legacy fallback from typed nil", withOptions(provider.TextPart("x"), provider.ProviderOptions{"amazonBedrock": (*TextPartOptions)(nil), "bedrock": provider.RawProviderOption{Key: "bedrock", Raw: json.RawMessage(`{"guardContent":true}`)}}), `[{"guardContent":{"text":{"text":"x"}}}]`},
		{"modern false wins", withOptions(provider.TextPart("x"), provider.ProviderOptions{"amazonBedrock": provider.RawProviderOption{Key: "amazonBedrock", Raw: json.RawMessage(`{"guardContent":false}`)}, "bedrock": provider.RawProviderOption{Key: "bedrock", Raw: json.RawMessage(`{"guardContent":true}`)}}), `[{"text":"x"}]`},
		{"modern unknown wins", withOptions(provider.TextPart("x"), provider.ProviderOptions{"amazonBedrock": provider.RawProviderOption{Key: "amazonBedrock", Raw: json.RawMessage(`{"citations":{"enabled":true}}`)}, "bedrock": provider.RawProviderOption{Key: "bedrock", Raw: json.RawMessage(`{"guardContent":true}`)}}), `[{"text":"x"}]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.JSONEq(t, tc.want, requestContentJSON(t, tc.part))
		})
	}
}

func TestGuardContent_InvalidOptions(t *testing.T) {
	tests := []struct {
		name string
		part provider.ContentPart
	}{
		{"text string flag", withOptions(provider.TextPart("x"), rawPartOptions("amazonBedrock", `{"guardContent":"true"}`))},
		{"text null flag", withOptions(provider.TextPart("x"), rawPartOptions("amazonBedrock", `{"guardContent":null}`))},
		{"text null qualifier", withOptions(provider.TextPart("x"), rawPartOptions("amazonBedrock", `{"guardContent":false,"guardContentQualifiers":null}`))},
		{"text wrong qualifier type", withOptions(provider.TextPart("x"), rawPartOptions("amazonBedrock", `{"guardContentQualifiers":"query"}`))},
		{"text non-string qualifier", withOptions(provider.TextPart("x"), rawPartOptions("amazonBedrock", `{"guardContentQualifiers":[1]}`))},
		{"text null array entry", withOptions(provider.TextPart("x"), rawPartOptions("amazonBedrock", `{"guardContentQualifiers":[null]}`))},
		{"text invalid qualifier", withOptions(provider.TextPart("x"), rawPartOptions("amazonBedrock", `{"guardContent":false,"guardContentQualifiers":["other"]}`))},
		{"typed invalid qualifier", withOptions(provider.TextPart("x"), provider.BuildProviderOptions(TextPartOptions{GuardContentQualifiers: &[]GuardContentQualifier{"other"}}))},
		{"image null flag", guardImage(rawPartOptions("amazonBedrock", `{"guardContent":null}`))},
		{"image numeric flag", guardImage(rawPartOptions("bedrock", `{"guardContent":1}`))},
		{"malformed", withOptions(provider.TextPart("x"), rawPartOptions("bedrock", `{`))},
		{"non-object", withOptions(provider.TextPart("x"), rawPartOptions("amazonBedrock", `[]`))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var called atomic.Int32
			lm := newStubBedrockProvider(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called.Add(1); w.WriteHeader(http.StatusOK) }))
			opts := provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(tc.part)}}
			_, err := lm.DoGenerate(context.Background(), opts)
			require.Error(t, err)
			assert.Zero(t, called.Load())
			_, err = lm.DoStream(context.Background(), opts)
			require.Error(t, err)
			assert.Zero(t, called.Load())
		})
	}
}

func TestGuardContent_EndpointRequests(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "generate"
		if stream {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			bodies := make(chan []byte, 1)
			frames := encodeFixtures(t, `{"messageStart":{"role":"assistant"}}`, `{"messageStop":{"stopReason":"end_turn"}}`, `{"metadata":{"usage":{"inputTokens":1,"outputTokens":1}}}`)
			lm := newStubBedrockProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				bodies <- body
				if stream {
					assert.Contains(t, r.URL.Path, "/converse-stream")
					w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
					_, _ = w.Write(frames)
				} else {
					assert.Contains(t, r.URL.Path, "/converse")
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}},"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1}}`))
				}
			}))
			flag := true
			opts := provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(withOptions(provider.TextPart("guarded"), rawPartOptions("bedrock", `{"guardContent":true,"guardContentQualifiers":["query"]}`)), provider.TextPart("plain"), guardImage(rawPartOptions("amazonBedrock", `{"guardContent":true}`)), withOptions(provider.FilePart("text/plain", provider.DataContent{Text: "hi"}), provider.BuildProviderOptions(ImagePartOptions{GuardContent: &flag})))}}
			if stream {
				result, err := lm.DoStream(context.Background(), opts)
				require.NoError(t, err)
				for range result.Stream {
				}
			} else {
				_, err := lm.DoGenerate(context.Background(), opts)
				require.NoError(t, err)
			}
			var body struct {
				Messages []struct {
					Content json.RawMessage `json:"content"`
				} `json:"messages"`
			}
			require.NoError(t, json.Unmarshal(<-bodies, &body))
			require.Len(t, body.Messages, 1)
			assert.JSONEq(t, `[{"guardContent":{"text":{"text":"guarded","qualifiers":["query"]}}},{"text":"plain"},{"guardContent":{"image":{"format":"png","source":{"bytes":"aGVsbG8="}}}},{"document":{"format":"txt","name":"document-1","source":{"bytes":"aGk="}}}]`, string(body.Messages[0].Content))
		})
	}
}
