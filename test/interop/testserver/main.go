// Command testserver is the bidirectional upstream-client conformance server.
// It serves mock provider.LanguageModel implementations through the public
// gateway/providerwire handler so a stock upstream Vercel AI SDK client
// (@ai-sdk/gateway + ai) can drive the complete Go transport end-to-end.
//
// The gateway client selects a scenario via the ai-language-model-id request
// header. Scenarios:
//
//   - "generate-rich"          -> rich successful unary result
//   - "tool-result-file-input" -> decode canonical tool-result file data
//   - "stream-text"            -> streaming text with a system + user prompt
//   - "tool-call"              -> client-executed tool-call round trip
//   - "file-input"             -> decode an upstream file/image input part
//   - "stream-sources"         -> emit URL and document sources
//   - "error-mid-stream"       -> mid-stream provider error part
//   - "error-pre-stream"       -> pre-stream HTTP error envelope
//   - "strict-error-internal"  -> nil unary result for safe HTTP 500 projection
//
// It also exposes GET /health for the vitest global setup.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/gateway/providerwire"
	providerwirev4 "github.com/grafana/ai-sdk/gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
)

const (
	legacyMountPrefix = "/api/v1/aisdk/legacy"
	strictMountPrefix = "/api/v1/aisdk/strict"
)

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

func promptToolResultFiles(opts provider.CallOptions) []string {
	var files []string
	for _, message := range opts.Prompt {
		for _, part := range message.Content {
			if part.Type != provider.ContentPartTypeToolResult || part.Output == nil {
				continue
			}
			for _, content := range part.Output.Content {
				if content.Type != provider.ToolContentFile || content.Data == nil {
					continue
				}
				encoded := content.Data.Base64
				if content.Data.Bytes != nil {
					encoded = base64.StdEncoding.EncodeToString(content.Data.Bytes)
				}
				files = append(files, fmt.Sprintf("data=%s mediaType=%s filename=%s", encoded, content.MediaType, content.Filename))
			}
		}
	}
	return files
}

func generateRich() *provider.GenerateResult {
	preliminary := false
	return &provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{Type: provider.ContentText, Text: "hello from unary"},
			{Type: provider.ContentReasoning, Text: "concise reasoning"},
			{Type: provider.ContentToolCall, ToolCallID: "call_1", ToolName: "lookup", Input: json.RawMessage(`{"query":"grafana"}`)},
			{Type: provider.ContentToolResult, ToolCallID: "call_1", ToolName: "lookup", Result: json.RawMessage(`{"answer":42}`), Preliminary: &preliminary},
			{Type: provider.ContentFile, Data: &provider.DataContent{Base64: "AAEC"}, MediaType: "application/octet-stream"},
			{Type: provider.ContentSource, ID: "source_1", SourceType: provider.SourceTypeURL, URL: "https://example.com/source", Title: "Example source"},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "end_turn"},
		Usage: provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: intPtr(11), NoCache: intPtr(7), CacheRead: intPtr(4)},
			OutputTokens: provider.OutputTokenUsage{Total: intPtr(6), Text: intPtr(4), Reasoning: intPtr(2)},
			Raw:          json.RawMessage(`{"input_tokens":11,"output_tokens":6}`),
		},
		ProviderMetadata: provider.ProviderMetadata{"interop": json.RawMessage(`{"trace":"public"}`)},
		Warnings:         []provider.Warning{{Type: provider.WarnOther, Message: "server warning must be replaced by gateway transport warnings"}},
		Request:          &provider.RequestMetadata{Body: json.RawMessage(`{"serverRequest":"private"}`)},
		Response: &provider.GenerateResponse{
			ResponseMetadata: provider.ResponseMetadata{ID: "backend_response", ModelID: "backend-model", Provider: "backend", Timestamp: time.Unix(1_700_000_000, 0).UTC()},
			Headers:          map[string]string{"x-backend-secret": "private"},
			Body:             json.RawMessage(`{"serverResponse":"private"}`),
		},
	}
}

func generateToolResultFile(opts provider.CallOptions) *provider.GenerateResult {
	return &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: strings.Join(promptToolResultFiles(opts), " | ")}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "end_turn"},
		Usage:        provider.Usage{InputTokens: provider.InputTokenUsage{Total: intPtr(1)}, OutputTokens: provider.OutputTokenUsage{Total: intPtr(1)}},
		Warnings:     []provider.Warning{},
	}
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

func streamSources() *provider.StreamResult {
	return streamResult(
		provider.StreamPart{Type: provider.PartStreamStart},
		provider.StreamPart{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "url-source", URL: "https://example.com", Title: "Example"}},
		provider.StreamPart{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeDocument, ID: "document-source", Title: "Document", MediaType: "application/pdf", Filename: "document.pdf"}},
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
	case strings.Contains(m.modelID, "tool-call"):
		return streamToolCall(opts), nil
	case strings.Contains(m.modelID, "file-input"):
		return streamFileInput(opts), nil
	case strings.Contains(m.modelID, "error-pre-stream"):
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{
			Message:    "rate limited pre-stream",
			StatusCode: http.StatusTooManyRequests,
		})
	case strings.Contains(m.modelID, "error-mid-stream"):
		return streamMidStreamError(), nil
	case strings.Contains(m.modelID, "stream-sources"):
		return streamSources(), nil
	case strings.Contains(m.modelID, "stream-text"):
		return streamText(opts), nil
	default:
		return nil, unknownScenarioError(m.modelID)
	}
}

func (m *scenarioModel) DoGenerate(_ context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
	switch {
	case strings.Contains(m.modelID, "generate-rich"):
		return generateRich(), nil
	case strings.Contains(m.modelID, "tool-result-file-input"):
		return generateToolResultFile(opts), nil
	case strings.Contains(m.modelID, "error-pre-stream"):
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{
			Message:    "rate limited pre-stream",
			StatusCode: http.StatusTooManyRequests,
		})
	case strings.Contains(m.modelID, "strict-error-internal"):
		return nil, nil
	default:
		return nil, unknownScenarioError(m.modelID)
	}
}

func unknownScenarioError(modelID string) *provider.APICallError {
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:    "unknown scenario model id: " + modelID,
		StatusCode: http.StatusBadRequest,
	})
}

type interopCatalogResolver func(context.Context, string) (catalog.ResolvedModel, error)

func (f interopCatalogResolver) ResolveModel(ctx context.Context, modelID string) (catalog.ResolvedModel, error) {
	return f(ctx, modelID)
}

func main() {
	legacyHandler, err := providerwire.NewHandler(providerwire.ModelResolverFunc(func(_ *http.Request, modelID string) (provider.LanguageModel, error) {
		return &scenarioModel{modelID: modelID}, nil
	}))
	if err != nil {
		log.Fatalf("failed to construct legacy provider-wire handler: %v", err)
	}
	strictHandler, err := providerwirev4.NewHandler(interopCatalogResolver(func(_ context.Context, modelID string) (catalog.ResolvedModel, error) {
		return catalog.ResolvedModel{ID: modelID, Model: &scenarioModel{modelID: modelID}}, nil
	}))
	if err != nil {
		log.Fatalf("failed to construct strict provider-wire handler: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	})
	mux.Handle(legacyMountPrefix+providerwire.PathLanguageModel, legacyHandler)
	mux.Handle(strictMountPrefix+providerwirev4.PathLanguageModel, strictHandler)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if _, err := fmt.Fprintf(os.Stdout, "PORT=%d\n", port); err != nil {
		log.Fatalf("failed to write port: %v", err)
	}
	_ = os.Stdout.Sync()

	log.Printf("interop test server listening on :%d (legacy %s, strict %s)", port, legacyMountPrefix+providerwire.PathLanguageModel, strictMountPrefix+providerwirev4.PathLanguageModel)
	if err := http.Serve(listener, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
