package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_ProviderToolsMCPOptionsReachNativeTransport(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.JSONEq(t, `[{"type":"code_execution_20260120","name":"code_execution"}]`, string(body["tools"]))
		assert.JSONEq(t, `[{"type":"url","name":"echo","url":"https://mcp.example.test/tools","authorization_token":"mcp-private-token","tool_configuration":{"enabled":false,"allowed_tools":[]}}]`, string(body["mcp_servers"]))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"claude-sonnet-4-6","stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()
	model := New("key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
	maxOutputTokens := 64
	_, err := model.DoGenerate(context.Background(), provider.CallOptions{
		MaxOutputTokens: &maxOutputTokens,
		Prompt:          []provider.Message{provider.UserText("Use tools")},
		Tools:           []provider.Tool{{Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20260120", Name: "python", Args: map[string]json.RawMessage{}}},
		ProviderOptions: provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example.test/tools","authorizationToken":"mcp-private-token","toolConfiguration":{"enabled":false,"allowedTools":[]}}]}`)}},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, requests)
}

func TestDoGenerate_LargeDefaultOutputUsesContextDeadline(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.JSONEq(t, `128000`, string(body["max_tokens"]))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"claude-sonnet-4-6","stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer server.Close()
	model := New("key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
	options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := model.DoGenerate(ctx, options)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, int32(1), requests.Load())

	_, err = model.DoGenerate(context.Background(), options)
	require.ErrorContains(t, err, "streaming is required")
	assert.Equal(t, int32(1), requests.Load())
}

func TestDoGenerate_ExplicitRequestTimeoutOverridesContextDefault(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		select {
		case <-r.Context().Done():
		case <-time.After(500 * time.Millisecond):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`))
		}
	}))
	defer server.Close()
	model := New("key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0), option.WithRequestTimeout(50*time.Millisecond)))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := model.DoGenerate(ctx, provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
	require.Error(t, err)
	assert.NoError(t, ctx.Err())
	assert.Equal(t, int32(1), requests.Load())
}

func TestDoGenerate_ExpiredDeadlineDoesNotInvokeProvider(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	model := New("key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err := model.DoGenerate(ctx, provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
	require.True(t, errors.Is(err, context.DeadlineExceeded), "expected deadline cause, got %v", err)
	assert.Zero(t, requests.Load())
}

func TestNew_MessagesRequestTarget(t *testing.T) {
	for _, tc := range []struct {
		name           string
		requestOptions []option.RequestOption
		wantTarget     string
		betas          []string
		wantHeaderBeta string
		wantMetadata   string
	}{
		{name: "default", wantTarget: "/v1/messages"},
		{name: "provider beta headers", wantTarget: "/v1/messages", betas: []string{"test-beta"}},
		{
			name: "caller repeated query and body options",
			requestOptions: []option.RequestOption{
				option.WithQuery("api-version", "v1"),
				option.WithQueryAdd("feature", "a"),
				option.WithQueryAdd("feature", "b"),
				option.WithJSONSet("metadata", map[string]string{"user_id": "test-user"}),
			},
			wantTarget:   "/v1/messages?api-version=v1&feature=a&feature=b",
			betas:        []string{"test-beta"},
			wantMetadata: `{"user_id":"test-user"}`,
		},
		{
			name:           "caller beta query override",
			requestOptions: []option.RequestOption{option.WithQuery("beta", "false")},
			wantTarget:     "/v1/messages?beta=false",
			betas:          []string{"test-beta"},
		},
		{
			name:           "caller beta header",
			requestOptions: []option.RequestOption{option.WithHeader("anthropic-beta", "caller-beta")},
			wantTarget:     "/v1/messages",
			betas:          []string{"test-beta"},
			wantHeaderBeta: "caller-beta",
		},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				type capturedRequest struct {
					method, target string
					headers        http.Header
					body           []byte
					err            error
				}
				requests := make(chan capturedRequest, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					requests <- capturedRequest{r.Method, r.RequestURI, r.Header.Clone(), body, err}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"captured"}}`)
				}))
				defer server.Close()

				requestOptions := []option.RequestOption{
					option.WithBaseURL(server.URL),
					option.WithHTTPClient(server.Client()),
					option.WithMaxRetries(0),
					option.WithHeader("X-Test", "preserved"),
				}
				requestOptions = append(requestOptions, tc.requestOptions...)
				model := New("test-key", "claude-sonnet-4-6", WithRequestOptions(requestOptions...))
				maxTokens := 64
				opts := provider.CallOptions{
					Prompt:          []provider.Message{provider.UserText("hello")},
					MaxOutputTokens: &maxTokens,
					ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{Betas: tc.betas}),
				}
				var err error
				if stream {
					_, err = model.DoStream(t.Context(), opts)
				} else {
					_, err = model.DoGenerate(t.Context(), opts)
				}
				require.ErrorContains(t, err, "captured")
				captured := <-requests
				require.NoError(t, captured.err)
				assert.Equal(t, http.MethodPost, captured.method)
				assert.Equal(t, tc.wantTarget, captured.target)
				assert.Equal(t, "preserved", captured.headers.Get("X-Test"))
				betas := strings.Join(captured.headers.Values("anthropic-beta"), ",")
				for _, beta := range tc.betas {
					assert.Contains(t, betas, beta)
				}
				if tc.wantHeaderBeta != "" {
					assert.Contains(t, betas, tc.wantHeaderBeta)
				}
				if len(tc.betas) == 0 && tc.wantHeaderBeta == "" {
					assert.Empty(t, betas)
				}
				var body map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(captured.body, &body))
				assert.JSONEq(t, `"claude-sonnet-4-6"`, string(body["model"]))
				assert.Equal(t, stream, string(body["stream"]) == "true")
				if tc.wantMetadata != "" {
					assert.JSONEq(t, tc.wantMetadata, string(body["metadata"]))
				}
			})
		}
	}
}

func TestNew_ToolResultFileReferencesReachNativeTransport(t *testing.T) {
	file := func(mediaType string, data provider.DataContent) provider.ToolResultContentValue {
		return provider.ToolResultContentValue{Type: provider.ToolContentFile, MediaType: mediaType, Data: &data}
	}
	reference := func(mediaType, raw string) provider.ToolResultContentValue {
		return file(mediaType, provider.ReferenceDataContent(json.RawMessage(raw)))
	}
	const missingAnthropicID = "anthropic: no provider reference found for provider anthropic; available providers: "
	tests := []struct {
		name            string
		userReference   bool
		results         [][]provider.ToolResultContentValue
		providerOptions provider.ProviderOptions
		wantContent     []string
		wantBetas       []string
		wantWarnings    []string
		wantErr         string
	}{
		{
			name:        "image reference",
			results:     [][]provider.ToolResultContentValue{{reference("image/png", `{"anthropic":"file_img"}`)}},
			wantContent: []string{`[{"type":"image","source":{"type":"file","file_id":"file_img"}}]`},
			wantBetas:   []string{string(filesAPIBeta)},
		},
		{
			name: "document references",
			results: [][]provider.ToolResultContentValue{{
				reference("application/pdf", `{"anthropic":"file_pdf"}`),
				reference("text/plain", `{"anthropic":"file_txt"}`),
				reference("application/zip", `{"anthropic":"file_zip"}`),
			}},
			wantContent: []string{`[
				{"type":"document","source":{"type":"file","file_id":"file_pdf"}},
				{"type":"document","source":{"type":"file","file_id":"file_txt"}},
				{"type":"document","source":{"type":"file","file_id":"file_zip"}}
			]`},
			wantBetas: []string{string(filesAPIBeta)},
		},
		{
			name: "empty IDs for image and document targets",
			results: [][]provider.ToolResultContentValue{{
				reference("image/png", `{"anthropic":""}`),
				reference("application/pdf", `{"anthropic":""}`),
			}},
			wantContent: []string{`[
				{"type":"image","source":{"type":"file","file_id":""}},
				{"type":"document","source":{"type":"file","file_id":""}}
			]`},
			wantBetas: []string{string(filesAPIBeta)},
		},
		{
			name:    "other provider reference",
			results: [][]provider.ToolResultContentValue{{reference("application/pdf", `{"openai":"file-123"}`)}},
			wantErr: missingAnthropicID + "openai",
		},
		{
			name:    "reference key is not normalized",
			results: [][]provider.ToolResultContentValue{{reference("application/pdf", `{"Anthropic":"file-123"}`)}},
			wantErr: missingAnthropicID + "Anthropic",
		},
		{
			name:    "empty reference",
			results: [][]provider.ToolResultContentValue{{reference("application/pdf", `{}`)}},
			wantErr: missingAnthropicID,
		},
		{
			name: "mixed entries keep order",
			results: [][]provider.ToolResultContentValue{{
				{Type: provider.ToolContentText, Text: "attachments"},
				file("application/zip", provider.Base64DataContent("UEsDBA==")),
				reference("application/pdf", `{"anthropic":"file_1"}`),
				file("text/plain", provider.TextDataContent("inline notes")),
				file("text/plain", provider.URLDataContent("https://example.com/notes.txt")),
				file("image/png", provider.URLDataContent("https://example.com/map.png")),
				file("image/png", provider.Base64DataContent("aW1n")),
				file("application/pdf", provider.BytesDataContent([]byte("%PDF-"))),
			}},
			wantContent: []string{`[
				{"type":"text","text":"attachments"},
				{"type":"document","source":{"type":"file","file_id":"file_1"}},
				{"type":"document","source":{"type":"url","url":"https://example.com/notes.txt"}},
				{"type":"image","source":{"type":"url","url":"https://example.com/map.png"}},
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aW1n"}},
				{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"JVBERi0="}}
			]`},
			wantBetas: []string{string(filesAPIBeta), "pdfs-2024-09-25"},
			wantWarnings: []string{
				"unsupported tool content part type: file with media type: application/zip",
				"unsupported tool content part type: file with data type: text",
			},
		},
		{
			name:          "files API beta is sent once",
			userReference: true,
			results: [][]provider.ToolResultContentValue{
				{reference("image/png", `{"anthropic":"file_img"}`)},
				{reference("application/pdf", `{"anthropic":"file_pdf"}`)},
			},
			providerOptions: makeProviderOpts(`{"betas":["files-api-2025-04-14"]}`),
			wantContent: []string{
				`[{"type":"image","source":{"type":"file","file_id":"file_img"}}]`,
				`[{"type":"document","source":{"type":"file","file_id":"file_pdf"}}]`,
			},
			wantBetas: []string{string(filesAPIBeta)},
		},
	}
	for _, tc := range tests {
		for _, vertex := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/vertex=%t/stream=%t", tc.name, vertex, stream), func(t *testing.T) {
					type capturedRequest struct {
						body  map[string]json.RawMessage
						betas []string
					}
					requests := make(chan capturedRequest, 1)
					var count atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						count.Add(1)
						var body map[string]json.RawMessage
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							http.Error(w, err.Error(), http.StatusBadRequest)
							return
						}
						var betas []string
						for _, value := range r.Header.Values("anthropic-beta") {
							for token := range strings.SplitSeq(value, ",") {
								betas = append(betas, strings.TrimSpace(token))
							}
						}
						requests <- capturedRequest{body: body, betas: betas}
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							_, _ = fmt.Fprint(w, "event: message_start\ndata: "+`{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`+"\n\n")
							_, _ = fmt.Fprint(w, "event: message_delta\ndata: "+`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`+"\n\n")
							_, _ = fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
							return
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(`{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"claude-sonnet-4-6","stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`))
					}))
					defer server.Close()
					m := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0))).(*model)
					if vertex {
						m.capabilities = vertexProviderCapabilities
					}

					userParts := []provider.ContentPart{provider.TextPart("Summarize the attachments.")}
					if tc.userReference {
						userParts = append(userParts, provider.FilePart("application/pdf", provider.ReferenceDataContent(json.RawMessage(`{"anthropic":"file_user"}`))))
					}
					var calls, results []provider.ContentPart
					for i, entries := range tc.results {
						callID := fmt.Sprintf("call_%d", i+1)
						calls = append(calls, provider.ToolCallPart(callID, "attachments", json.RawMessage(`{}`)))
						results = append(results, provider.ToolResultPart(callID, "attachments", &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: entries}))
					}
					maxTokens := 1024
					opts := provider.CallOptions{
						Prompt: []provider.Message{
							provider.NewUserMessage(userParts...),
							provider.NewAssistantMessage(calls...),
							provider.NewToolMessage(results...),
						},
						MaxOutputTokens: &maxTokens,
						ProviderOptions: tc.providerOptions,
					}

					var warnings []provider.Warning
					var err error
					if stream {
						var result *provider.StreamResult
						result, err = m.DoStream(t.Context(), opts)
						if err == nil {
							for part := range result.Stream {
								require.NotEqual(t, provider.PartError, part.Type, "%+v", part)
								if part.Type == provider.PartStreamStart {
									warnings = part.Warnings
								}
							}
						}
					} else {
						var result *provider.GenerateResult
						result, err = m.DoGenerate(t.Context(), opts)
						if err == nil {
							warnings = result.Warnings
						}
					}
					if tc.wantErr != "" {
						require.EqualError(t, err, tc.wantErr)
						assert.Zero(t, count.Load())
						return
					}
					require.NoError(t, err)
					require.Equal(t, int32(1), count.Load())
					captured := <-requests

					assert.ElementsMatch(t, tc.wantBetas, captured.betas)
					var messages []struct {
						Content []struct {
							Type    string          `json:"type"`
							Content json.RawMessage `json:"content"`
						} `json:"content"`
					}
					require.NoError(t, json.Unmarshal(captured.body["messages"], &messages))
					require.Len(t, messages, 3)
					require.Len(t, messages[2].Content, len(tc.wantContent))
					for i, want := range tc.wantContent {
						assert.Equal(t, "tool_result", messages[2].Content[i].Type)
						assert.JSONEq(t, want, string(messages[2].Content[i].Content))
					}
					var wantWarnings []provider.Warning
					for _, message := range tc.wantWarnings {
						wantWarnings = append(wantWarnings, provider.Warning{Type: provider.WarnOther, Message: message})
					}
					assert.Equal(t, wantWarnings, warnings)
				})
			}
		}
	}
}
