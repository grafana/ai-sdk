// Command testserver is the bidirectional upstream-client conformance server.
// It serves mock provider.LanguageModel implementations through the public
// gateway/providerwire handler so a stock upstream Vercel AI SDK client
// (@ai-sdk/gateway + ai) can drive the complete Go transport end-to-end.
//
// The gateway client selects a scenario via the ai-language-model-id request
// header. Scenarios:
//
//   - "stream-text"            -> streaming text with a system + user prompt
//   - "tool-call"              -> client-executed tool-call round trip
//   - "provider-tool-result"   -> provider-executed tool-result part
//   - "file-input"             -> decode an upstream file/image input part
//   - "file-output"            -> emit an inline-data file part in the response stream
//   - "file-output-url"        -> emit URL-valued file and reasoning-file parts
//   - "error-mid-stream"       -> mid-stream provider error part
//   - "error-pre-stream"       -> pre-stream HTTP error envelope
//
// It also exposes GET /health for the vitest global setup.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/grafana/ai-sdk/gateway/providerwire"
	"github.com/grafana/ai-sdk/provider"
)

const mountPrefix = "/api/v1/aisdk"

func intPtr(i int) *int    { return &i }
func boolPtr(b bool) *bool { return &b }

func streamResult(parts ...provider.StreamPart) *provider.StreamResult {
	stream := make(chan provider.StreamPart, len(parts))
	for _, part := range parts {
		stream <- part
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}
}

func finish(reason provider.UnifiedFinishReason, raw string, in, out int) provider.StreamPart {
	return provider.StreamPart{
		Type:         provider.PartFinish,
		FinishReason: &provider.FinishReason{Unified: reason, Raw: raw},
		Usage:        &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(in)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(out)}},
	}
}

func summarizeSystem(opts provider.CallOptions) string {
	for _, message := range opts.Prompt {
		if message.Role != provider.RoleSystem {
			continue
		}
		var summary strings.Builder
		for _, part := range message.Content {
			if part.Type == provider.ContentPartTypeText {
				summary.WriteString(part.Text)
			}
		}
		return summary.String()
	}
	return ""
}

func promptToolResults(opts provider.CallOptions) []string {
	var results []string
	for _, message := range opts.Prompt {
		for _, part := range message.Content {
			if part.Type != provider.ContentPartTypeToolResult || part.Output == nil {
				continue
			}
			output := part.Output
			results = append(results, fmt.Sprintf("toolName=%s output.type=%s output.text=%q output.json=%s", part.ToolName, output.Type, output.Text, string(output.JSON)))
		}
	}
	return results
}

func promptFileParts(opts provider.CallOptions) []string {
	var files []string
	for _, message := range opts.Prompt {
		for _, part := range message.Content {
			if part.Type != provider.ContentPartTypeFile || part.Data == nil {
				continue
			}
			data := part.Data
			files = append(files, fmt.Sprintf("bytes=%dB base64Len=%d url=%q", len(data.Bytes), len(data.Base64), data.URL))
		}
	}
	return files
}

func streamText(opts provider.CallOptions) *provider.StreamResult {
	return streamResult(
		provider.StreamPart{Type: provider.PartStreamStart},
		provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "resp_text", ModelID: "stream-text", Timestamp: time.Now().UTC()},
		provider.StreamPart{Type: provider.PartTextStart, ID: "t0"},
		provider.StreamPart{Type: provider.PartTextDelta, ID: "t0", Delta: "system=" + summarizeSystem(opts) + "; "},
		provider.StreamPart{Type: provider.PartTextDelta, ID: "t0", Delta: "hello from go"},
		provider.StreamPart{Type: provider.PartTextEnd, ID: "t0"},
		finish(provider.FinishReasonStop, "end_turn", 10, 5),
	)
}

func streamToolCall(opts provider.CallOptions) *provider.StreamResult {
	if results := promptToolResults(opts); len(results) > 0 {
		final := "done: " + strings.Join(results, " | ")
		return streamResult(
			provider.StreamPart{Type: provider.PartStreamStart},
			provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "resp_tc_2", ModelID: "tool-call", Timestamp: time.Now().UTC()},
			provider.StreamPart{Type: provider.PartTextStart, ID: "t1"},
			provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: final},
			provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"},
			finish(provider.FinishReasonStop, "end_turn", 20, 5),
		)
	}
	const toolCallID, toolName = "call_echo_1", "echoTool"
	const input = `{"text":"hello from go tool call"}`
	return streamResult(
		provider.StreamPart{Type: provider.PartStreamStart},
		provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "resp_tc_1", ModelID: "tool-call", Timestamp: time.Now().UTC()},
		provider.StreamPart{Type: provider.PartToolInputStart, ID: toolCallID, ToolName: toolName},
		provider.StreamPart{Type: provider.PartToolInputDelta, ID: toolCallID, Delta: input},
		provider.StreamPart{Type: provider.PartToolInputEnd, ID: toolCallID},
		provider.StreamPart{Type: provider.PartToolCall, ToolCallID: toolCallID, ToolName: toolName, Input: input},
		finish(provider.FinishReasonToolCalls, "tool_use", 10, 3),
	)
}

func streamProviderToolResult() *provider.StreamResult {
	const toolCallID, toolName = "call_pe_1", "webSearch"
	return streamResult(
		provider.StreamPart{Type: provider.PartStreamStart},
		provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "resp_ptr_1", ModelID: "provider-tool-result", Timestamp: time.Now().UTC()},
		provider.StreamPart{Type: provider.PartToolInputStart, ID: toolCallID, ToolName: toolName, ProviderExecuted: true},
		provider.StreamPart{Type: provider.PartToolCall, ToolCallID: toolCallID, ToolName: toolName, Input: `{"query":"grafana"}`, ProviderExecuted: true},
		provider.StreamPart{
			Type:             provider.PartToolResult,
			ToolCallID:       toolCallID,
			ToolName:         toolName,
			ProviderExecuted: true,
			Result:           json.RawMessage(`"Grafana is an observability platform"`),
			ProviderMetadata: provider.ProviderMetadata{
				"grafana-ai-sdk": json.RawMessage(`{"customer":"keep"}`),
			},
		},
		finish(provider.FinishReasonStop, "end_turn", 15, 8),
	)
}

func streamFileInput(opts provider.CallOptions) *provider.StreamResult {
	files := promptFileParts(opts)
	summary := fmt.Sprintf("decoded %d file part(s): %s", len(files), strings.Join(files, " | "))
	return streamResult(
		provider.StreamPart{Type: provider.PartStreamStart},
		provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "resp_file_in", ModelID: "file-input", Timestamp: time.Now().UTC()},
		provider.StreamPart{Type: provider.PartTextStart, ID: "t0"},
		provider.StreamPart{Type: provider.PartTextDelta, ID: "t0", Delta: summary},
		provider.StreamPart{Type: provider.PartTextEnd, ID: "t0"},
		finish(provider.FinishReasonStop, "end_turn", 12, 6),
	)
}

func streamFileOutput() *provider.StreamResult {
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	return streamResult(
		provider.StreamPart{Type: provider.PartStreamStart},
		provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "resp_file_out", ModelID: "file-output", Timestamp: time.Now().UTC()},
		provider.StreamPart{Type: provider.PartFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: png}, MediaType: "image/png"},
		finish(provider.FinishReasonStop, "end_turn", 4, 2),
	)
}

func streamFileOutputURL() *provider.StreamResult {
	return streamResult(
		provider.StreamPart{Type: provider.PartStreamStart},
		provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "resp_file_out_url", ModelID: "file-output-url", Timestamp: time.Now().UTC()},
		provider.StreamPart{Type: provider.PartFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: "https://example.com/generated.png"}, MediaType: "image/png"},
		provider.StreamPart{Type: provider.PartReasoningFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: "https://example.com/reasoning.png"}, MediaType: "image/png"},
		finish(provider.FinishReasonStop, "end_turn", 4, 2),
	)
}

func streamMidStreamError() *provider.StreamResult {
	return streamResult(
		provider.StreamPart{Type: provider.PartStreamStart},
		provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "resp_err", ModelID: "error-mid-stream", Timestamp: time.Now().UTC()},
		provider.StreamPart{Type: provider.PartTextStart, ID: "t0"},
		provider.StreamPart{Type: provider.PartTextDelta, ID: "t0", Delta: "partial before error "},
		provider.StreamPart{
			Type: provider.PartError,
			APICallError: provider.NewAPICallError(provider.APICallErrorOptions{
				Message:     "boom mid-stream",
				StatusCode:  http.StatusInternalServerError,
				IsRetryable: boolPtr(false),
			}),
		},
		provider.StreamPart{Type: provider.PartTextDelta, ID: "t0", Delta: "continued after error"},
		provider.StreamPart{Type: provider.PartTextEnd, ID: "t0"},
		finish(provider.FinishReasonError, "error", 8, 4),
	)
}

type scenarioModel struct {
	modelID string
}

func (m *scenarioModel) SpecificationVersion() string               { return "v4" }
func (m *scenarioModel) Provider() string                           { return "interop" }
func (m *scenarioModel) ModelID() string                            { return m.modelID }
func (m *scenarioModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }

func (m *scenarioModel) DoStream(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	switch {
	case strings.Contains(m.modelID, "provider-tool-result"):
		return streamProviderToolResult(), nil
	case strings.Contains(m.modelID, "tool-call"):
		return streamToolCall(opts), nil
	case strings.Contains(m.modelID, "file-input"):
		return streamFileInput(opts), nil
	case strings.Contains(m.modelID, "file-output-url"):
		return streamFileOutputURL(), nil
	case strings.Contains(m.modelID, "file-output"):
		return streamFileOutput(), nil
	case strings.Contains(m.modelID, "error-pre-stream"):
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{
			Message:    "rate limited pre-stream",
			StatusCode: http.StatusTooManyRequests,
		})
	case strings.Contains(m.modelID, "error-mid-stream"):
		return streamMidStreamError(), nil
	case strings.Contains(m.modelID, "stream-text"):
		return streamText(opts), nil
	default:
		return nil, unknownScenarioError(m.modelID)
	}
}

func (m *scenarioModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	if strings.Contains(m.modelID, "error-pre-stream") {
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{
			Message:    "rate limited pre-stream",
			StatusCode: http.StatusTooManyRequests,
		})
	}
	return nil, unknownScenarioError(m.modelID)
}

func unknownScenarioError(modelID string) *provider.APICallError {
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:    "unknown scenario model id: " + modelID,
		StatusCode: http.StatusBadRequest,
	})
}

func main() {
	handler, err := providerwire.NewHandler(providerwire.ModelResolverFunc(func(_ *http.Request, modelID string) (provider.LanguageModel, error) {
		return &scenarioModel{modelID: modelID}, nil
	}))
	if err != nil {
		log.Fatalf("failed to construct provider-wire handler: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	})
	mux.Handle(mountPrefix+providerwire.PathLanguageModel, handler)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if _, err := fmt.Fprintf(os.Stdout, "PORT=%d\n", port); err != nil {
		log.Fatalf("failed to write port: %v", err)
	}
	_ = os.Stdout.Sync()

	log.Printf("interop test server listening on :%d (route %s)", port, mountPrefix+providerwire.PathLanguageModel)
	if err := http.Serve(listener, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
