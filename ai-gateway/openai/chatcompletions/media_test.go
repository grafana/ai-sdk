package chatcompletions

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	anthropic "github.com/grafana/ai-sdk/providers/anthropic"
	openai "github.com/grafana/ai-sdk/providers/openai"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
	sdk "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mediaTransport func(*http.Request) (*http.Response, error)

func (f mediaTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMultimodal_NativeRequests(t *testing.T) {
	const content = `[{"type":"text","text":"inspect"},{"type":"image_url","image_url":{"url":"https://example.test/image.png?signature=synthetic","detail":"original"}},{"type":"image_url","image_url":{"url":"data:image/png;base64,aGVsbG8=","detail":"low"}},{"type":"file","file":{"file_data":"data:application/pdf;base64,JVBERi0=","filename":"report.pdf"}},{"type":"file","file":{"file_id":"file-uploaded"}}]`
	for _, target := range []execution.Target{{Type: execution.TargetOpenAI}, {Type: execution.TargetAnthropic}, {Type: execution.TargetOpenAICompatible, ProviderName: "custom-vendor.chat"}} {
		t.Run(string(target.Type), func(t *testing.T) {
			r, err := mapRequest([]byte(`{"model":"alias","messages":[{"role":"user","content":` + content + `}]}`))
			require.NoError(t, err)
			before, err := json.Marshal(r.options)
			require.NoError(t, err)
			for _, streaming := range []bool{false, true} {
				options, err := chatTranslator(r)(target, r.options)
				require.NoError(t, err)
				captured := captureMediaRequest(t, target, options, streaming)
				switch target.Type {
				case execution.TargetOpenAI:
					assert.JSONEq(t, `[{"role":"user","content":[{"type":"input_text","text":"inspect"},{"type":"input_image","image_url":"https://example.test/image.png?signature=synthetic","detail":"original"},{"type":"input_image","image_url":"data:image/png;base64,aGVsbG8=","detail":"low"},{"type":"input_file","file_data":"data:application/pdf;base64,JVBERi0=","filename":"report.pdf"},{"type":"input_file","file_id":"file-uploaded"}]}]`, string(captured["input"]))
				case execution.TargetAnthropic:
					assert.JSONEq(t, `[{"role":"user","content":[{"type":"text","text":"inspect"},{"type":"image","source":{"type":"url","url":"https://example.test/image.png?signature=synthetic"}},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGVsbG8="}},{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"JVBERi0="},"title":"report.pdf"},{"type":"document","source":{"type":"file","file_id":"file-uploaded"}}]}]`, string(captured["messages"]))
				case execution.TargetOpenAICompatible:
					assert.JSONEq(t, `[{"role":"user","content":`+content+`}]`, string(captured["messages"]))
				}
			}
			after, err := json.Marshal(r.options)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after))
		})
	}
}

func captureMediaRequest(t *testing.T, target execution.Target, options provider.CallOptions, streaming bool) map[string]json.RawMessage {
	t.Helper()
	options.MaxOutputTokens = ptr(32)
	var captured map[string]json.RawMessage
	calls := 0
	client := &http.Client{Transport: mediaTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		require.NoError(t, json.NewDecoder(req.Body).Decode(&captured))
		return &http.Response{StatusCode: 418, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"synthetic capture","type":"invalid_request_error"}}`)), Request: req}, nil
	})}
	var model provider.LanguageModel
	switch target.Type {
	case execution.TargetOpenAI:
		model = openai.NewResponses("synthetic-key", "native", openai.WithRequestOptions(openaioption.WithHTTPClient(client), openaioption.WithMaxRetries(0)))
	case execution.TargetAnthropic:
		model = anthropic.New("synthetic-key", "claude-sonnet-4-5", anthropic.WithRequestOptions(anthropicoption.WithHTTPClient(client), anthropicoption.WithMaxRetries(0)))
	case execution.TargetOpenAICompatible:
		model = compatible.New("native", compatible.WithHTTPClient(client), compatible.WithProviderName(target.ProviderName))
	}
	var err error
	if streaming {
		_, err = model.DoStream(context.Background(), options)
	} else {
		_, err = model.DoGenerate(context.Background(), options)
	}
	require.Error(t, err)
	require.Equal(t, 1, calls, "%v", err)
	return captured
}

func TestMultimodal_AudioAndCapabilities(t *testing.T) {
	for _, format := range []string{"wav", "mp3"} {
		content := `[{"type":"input_audio","input_audio":{"format":"` + format + `","data":"aGVsbG8="}}]`
		r, err := mapRequest([]byte(`{"model":"alias","messages":[{"role":"user","content":` + content + `}]}`))
		require.NoError(t, err)
		for _, target := range []execution.TargetType{execution.TargetOpenAI, execution.TargetAnthropic} {
			_, err := chatTranslator(r)(execution.Target{Type: target}, r.options)
			require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
		}
		target := execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: "vendor"}
		options, err := chatTranslator(r)(target, r.options)
		require.NoError(t, err)
		for _, streaming := range []bool{false, true} {
			body := captureMediaRequest(t, target, options, streaming)
			assert.JSONEq(t, `[{"role":"user","content":`+content+`}]`, string(body["messages"]))
		}
	}
}

func TestMultimodal_InvalidInput(t *testing.T) {
	for _, content := range []string{
		`[{"type":"image_url","image_url":{"url":"file:///etc/passwd"}}]`,
		`[{"type":"image_url","image_url":{"url":"https://user:password@example.test/i"}}]`,
		`[{"type":"image_url","image_url":{"url":"https://example.test/i","detail":"invented"}}]`,
		`[{"type":"image_url","image_url":{"url":"data:text/plain;base64,aGVsbG8="}}]`,
		`[{"type":"image_url","image_url":{"url":"data:image/png;base64,%%%"}}]`,
		`[{"type":"image_url","text":"wrong arm","image_url":{"url":"https://example.test/i"}}]`,
		`[{"type":"input_audio","input_audio":{"format":"ogg","data":"aGVsbG8="}}]`,
		`[{"type":"input_audio","input_audio":{"format":"wav","data":"%%%"}}]`,
		`[{"type":"file","file":{"file_id":"f","file_data":"JVBERi0="}}]`,
		`[{"type":"file","file":{}}]`,
		`[{"type":"file","file":{"file_id":""}}]`,
		`[{"type":"file","file":{"file_data":"data:application/pdf,raw"}}]`,
	} {
		_, err := mapRequest([]byte(`{"model":"alias","messages":[{"role":"user","content":` + content + `}]}`))
		require.Error(t, err, content)
	}
	for _, role := range []string{"assistant", "system", "developer"} {
		_, err := mapRequest([]byte(`{"model":"alias","messages":[{"role":"` + role + `","content":[{"type":"image_url","image_url":{"url":"https://example.test/i"}}]}]}`))
		require.Error(t, err, role)
	}
}

func TestMultimodal_Base64FileAndReferenceIsolation(t *testing.T) {
	r, err := mapRequest([]byte(`{"model":"alias","messages":[{"role":"user","content":[{"type":"file","file":{"file_data":"JVBERi0=","filename":"report.pdf"}},{"type":"file","file":{"file_id":"file-1"}}]}]}`))
	require.NoError(t, err)
	assert.Equal(t, "application/pdf", r.options.Prompt[0].Content[0].MediaType)
	for _, name := range []string{"vendor-one.chat", "vendor_two.chat"} {
		options, err := chatTranslator(r)(execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: name}, r.options)
		require.NoError(t, err)
		namespace, _, _ := strings.Cut(name, ".")
		assert.JSONEq(t, `{"`+namespace+`":"file-1"}`, string(options.Prompt[0].Content[1].Data.Reference))
	}
	assert.JSONEq(t, `{"openai":"file-1"}`, string(r.options.Prompt[0].Content[1].Data.Reference))
}

func TestMultimodal_OfficialGoClient(t *testing.T) {
	handler := testHandler(t, &fakeModel{generate: func(_ context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
		require.Len(t, options.Prompt[0].Content, 3)
		assert.Equal(t, "image/*", options.Prompt[0].Content[1].MediaType)
		assert.Equal(t, "application/pdf", options.Prompt[0].Content[2].MediaType)
		return textResult(), nil
	}})
	client := sdk.NewClient(openaioption.WithBaseURL("https://gateway.test/v1/"), openaioption.WithAPIKey("synthetic"), openaioption.WithMaxRetries(0), openaioption.WithHTTPClient(&http.Client{Transport: mediaTransport(func(req *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		return recorder.Result(), nil
	})}))
	var params sdk.ChatCompletionNewParams
	require.NoError(t, json.Unmarshal([]byte(`{"model":"alias","messages":[{"role":"user","content":[{"type":"text","text":"inspect"},{"type":"image_url","image_url":{"url":"https://example.test/i","detail":"original"}},{"type":"file","file":{"file_data":"data:application/pdf;base64,JVBERi0=","filename":"report.pdf"}}]}]}`), &params))
	result, err := client.Chat.Completions.New(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, "hello", result.Choices[0].Message.Content)
}

func TestMultimodalUnsupportedHTTP(t *testing.T) {
	for _, target := range []execution.TargetType{execution.TargetOpenAI, execution.TargetAnthropic} {
		for _, streaming := range []bool{false, true} {
			calls := 0
			model := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				calls++
				return textResult(), nil
			}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				calls++
				return partsStream(textParts()...), nil
			}}
			c, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "alias"}, Model: execution.Translate(model, execution.Target{Type: target})}})
			require.NoError(t, err)
			runtime, err := execution.New(execution.Config{Selector: execution.CatalogSelector(c), Limits: execution.DefaultLimits})
			require.NoError(t, err)
			h, err := New(runtime)
			require.NoError(t, err)
			body := `{"model":"alias","messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"format":"wav","data":"aGVsbG8="}}]}]`
			if streaming {
				body += `,"stream":true`
			}
			body += "}"
			req := httptest.NewRequest("POST", Path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			assert.Equal(t, http.StatusBadRequest, w.Code, "target=%s streaming=%v body=%s", target, streaming, w.Body.String())
			assert.Zero(t, calls)
		}
	}
}
