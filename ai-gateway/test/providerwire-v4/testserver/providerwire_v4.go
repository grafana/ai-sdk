package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sync"
	"sync/atomic"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	providerwirev4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

const providerWireV4Prefix = "/providerwire-v4"

var syntheticRawUsage = json.RawMessage(`{"input_tokens":32,"cache_read_input_tokens":18,"service_tier":"standard","inference_geo":"us","nested":{"tokens":[1,2]}}`)

type providerWireV4Stats struct {
	successCalls        atomic.Int64
	streamCalls         atomic.Int64
	blockingCalls       atomic.Int64
	streamBlockingCalls atomic.Int64
	cancellations       atomic.Int64

	mu          sync.Mutex
	lastOptions provider.CallOptions
}

func (s *providerWireV4Stats) recordSuccess(options provider.CallOptions) {
	s.successCalls.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastOptions = options
}

func (s *providerWireV4Stats) options() provider.CallOptions {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastOptions
}

type providerWireV4Model struct {
	kind  string
	stats *providerWireV4Stats
}

func hostedMetadata() provider.ProviderMetadata {
	return provider.ProviderMetadata{"anthropic": json.RawMessage(`{"caller":{"type":"direct"},"private":"hidden"}`)}
}

func hostedStage(options provider.CallOptions) (int, error) {
	if len(options.Tools) != 1 || options.Tools[0].Type != provider.ToolTypeProvider || options.Tools[0].ID != "anthropic.code_execution_20260120" || options.Tools[0].Args == nil || len(options.ProviderOptions) != 0 {
		return 0, errors.New("invalid hosted tool request")
	}
	stage := 0
	for _, message := range options.Prompt {
		if message.Role != provider.RoleAssistant {
			continue
		}
		for _, part := range message.Content {
			if part.Type != provider.ContentPartTypeToolCall && part.Type != provider.ContentPartTypeToolResult {
				continue
			}
			metadata, ok := part.ProviderOptions["anthropic"].(provider.RawProviderOption)
			if !ok {
				return 0, errors.New("missing hosted tool metadata")
			}
			var fields struct {
				Caller struct {
					Type string `json:"type"`
				} `json:"caller"`
			}
			if json.Unmarshal(metadata.Raw, &fields) != nil || fields.Caller.Type != "direct" || part.ToolCallID != "call" || part.ToolName != "echo" {
				return 0, errors.New("invalid hosted tool history")
			}
			if part.Type == provider.ContentPartTypeToolCall {
				if !part.ProviderExecuted || stage != 0 {
					return 0, errors.New("invalid hosted call ownership")
				}
				stage = 1
			} else {
				if stage != 1 || part.Output == nil {
					return 0, errors.New("invalid hosted result")
				}
				stage = 2
			}
		}
	}
	return stage, nil
}

func hostedCallContent() provider.GenerateContentPart {
	return provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "echo", Input: json.RawMessage(`{}`), ProviderExecuted: true, Dynamic: new(true), ProviderMetadata: hostedMetadata()}
}

func hostedResultContent(isError bool) provider.GenerateContentPart {
	return provider.GenerateContentPart{Type: provider.ContentToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`{"result":"done"}`), IsError: isError, ProviderMetadata: hostedMetadata()}
}

func hostedCallStreamPart() provider.StreamPart {
	yes := true
	return provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "echo", Input: `{}`, ProviderExecuted: true, Dynamic: &yes, ProviderMetadata: hostedMetadata()}
}

func hostedResultStreamPart(isError bool) provider.StreamPart {
	return provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`{"result":"done"}`), IsError: isError, ProviderMetadata: hostedMetadata()}
}

func (*providerWireV4Model) SpecificationVersion() string               { return "v4" }
func (*providerWireV4Model) Provider() string                           { return "test" }
func (m *providerWireV4Model) ModelID() string                          { return "private-" + m.kind }
func (*providerWireV4Model) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *providerWireV4Model) DoStream(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	m.stats.streamCalls.Add(1)
	zero := 0
	one := 1
	two := 2
	switch m.kind {
	case "native-values", "native-empty", "native-partial", "native-absent":
		return scenarioNativeStream(m.kind), nil
	case "reasoning-files":
		m.stats.recordSuccess(options)
		meta := provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"file","reasoningEncryptedContent":null}`)}
		return &provider.StreamResult{Stream: scenarioStream(
			provider.StreamPart{Type: provider.PartReasoningFile, MediaType: "image/png", Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeData}, ProviderMetadata: meta},
			provider.StreamPart{Type: provider.PartReasoningFile, MediaType: "image/png", Data: &provider.StreamFileData{Bytes: []byte{1, 2, 3}}, ProviderMetadata: provider.ProviderMetadata{}},
			provider.StreamPart{Type: provider.PartReasoningFile, MediaType: "image/png", Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: "https://example.test/reasoning"}},
			provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{OutputTokens: provider.OutputTokenUsage{Total: &two, Reasoning: &two}}},
		)}, nil
	case "reasoning":
		m.stats.recordSuccess(options)
		return &provider.StreamResult{Stream: scenarioStream(
			provider.StreamPart{Type: provider.PartReasoningStart, ID: "1", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"old","reasoningEncryptedContent":null}`)}},
			provider.StreamPart{Type: provider.PartReasoningStart, ID: "2"},
			provider.StreamPart{Type: provider.PartTextStart, ID: "1"},
			provider.StreamPart{Type: provider.PartReasoningDelta, ID: "1", Delta: "first "},
			provider.StreamPart{Type: provider.PartReasoningDelta, ID: "2", Delta: "second"},
			provider.StreamPart{Type: provider.PartTextDelta, ID: "1", Delta: "answer"},
			provider.StreamPart{Type: provider.PartReasoningEnd, ID: "2", ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"signature":"end-signature"}`)}},
			provider.StreamPart{Type: provider.PartReasoningEnd, ID: "1", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"final","reasoningEncryptedContent":"opaque"}`)}},
			provider.StreamPart{Type: provider.PartTextEnd, ID: "1"},
			provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}},
		)}, nil
	case "sources":
		parts := []provider.StreamPart{}
		for _, source := range scenarioSources() {
			parts = append(parts, provider.StreamPart{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: source.SourceType, ID: source.ID, URL: source.URL, Title: source.Title, MediaType: source.MediaType, Filename: source.Filename, ProviderMetadata: source.ProviderMetadata}})
		}
		parts = append(parts, provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}})
		return &provider.StreamResult{Stream: scenarioStream(parts...)}, nil
	case "hosted-deferred", "hosted-deferred-error":
		m.stats.recordSuccess(options)
		stage, err := hostedStage(options)
		if err != nil {
			return nil, err
		}
		switch stage {
		case 0:
			return &provider.StreamResult{Stream: scenarioStream(hostedCallStreamPart(), provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}, Usage: &provider.Usage{}})}, nil
		case 1:
			return &provider.StreamResult{Stream: scenarioStream(hostedResultStreamPart(m.kind == "hosted-deferred-error"), provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}})}, nil
		default:
			return &provider.StreamResult{Stream: scenarioStream(provider.StreamPart{Type: provider.PartTextStart, ID: "final"}, provider.StreamPart{Type: provider.PartTextDelta, ID: "final", Delta: "finished"}, provider.StreamPart{Type: provider.PartTextEnd, ID: "final"}, provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}})}, nil
		}
	case "stream-tool-arguments":
		var parts []provider.StreamPart
		for i, input := range []string{"", `{"service":`, "\"\\\n\t<>&\u2028\u2029"} {
			id := string(rune('a' + i))
			parts = append(parts,
				provider.StreamPart{Type: provider.PartToolInputStart, ID: id, ToolName: "weather"},
				provider.StreamPart{Type: provider.PartToolInputDelta, ID: id, Delta: input},
				provider.StreamPart{Type: provider.PartToolInputEnd, ID: id},
				provider.StreamPart{Type: provider.PartToolCall, ToolCallID: id, ToolName: "weather", Input: input},
			)
		}
		parts = append(parts, provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}, Usage: &provider.Usage{}})
		return &provider.StreamResult{Stream: scenarioStream(parts...)}, nil
	case "stream-tool-results":
		var parts []provider.StreamPart
		for i, value := range []string{"false", "0", `""`, "[]", "{}"} {
			id := string(rune('a' + i))
			parts = append(parts,
				provider.StreamPart{Type: provider.PartToolCall, ToolCallID: id, ToolName: "weather", Input: "{}"},
				provider.StreamPart{Type: provider.PartToolResult, ToolCallID: id, ToolName: "weather", Result: json.RawMessage(value), IsError: i == 4},
			)
		}
		parts = append(parts, provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}})
		return &provider.StreamResult{Stream: scenarioStream(parts...)}, nil
	case "stream-tools":
		m.stats.recordSuccess(options)
		for _, message := range options.Prompt {
			for _, part := range message.Content {
				if part.Type == provider.ContentPartTypeToolResult {
					validOutput := part.Output != nil && (part.Output.Type == provider.ToolOutputText && part.Output.Text == "sunny" || part.Output.Type == provider.ToolOutputJSON && string(part.Output.JSON) == `"sunny"`)
					if part.ToolCallID != "call-weather" || part.ToolName != "weather" || !validOutput {
						return nil, errors.New("invalid streaming continuation")
					}
					return &provider.StreamResult{Stream: scenarioStream(
						provider.StreamPart{Type: provider.PartTextStart, ID: "final"},
						provider.StreamPart{Type: provider.PartTextDelta, ID: "final", Delta: "It is sunny."},
						provider.StreamPart{Type: provider.PartTextEnd, ID: "final"},
						provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}},
					)}, nil
				}
			}
		}
		return &provider.StreamResult{Stream: scenarioStream(
			provider.StreamPart{Type: provider.PartToolInputStart, ID: "call-weather", ToolName: "weather"},
			provider.StreamPart{Type: provider.PartToolInputDelta, ID: "call-weather", Delta: ""},
			provider.StreamPart{Type: provider.PartToolInputDelta, ID: "call-weather", Delta: `{"city":"Rio"}`},
			provider.StreamPart{Type: provider.PartToolInputEnd, ID: "call-weather"},
			provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call-weather", ToolName: "weather", Input: `{"city":"Rio"}`},
			provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}, Usage: &provider.Usage{}},
		)}, nil
	case "raw-usage", "raw-usage-empty":
		raw := syntheticRawUsage
		if m.kind == "raw-usage-empty" {
			raw = json.RawMessage(`{}`)
		}
		return &provider.StreamResult{Stream: scenarioStream(provider.StreamPart{Type: provider.PartFinish, Usage: &provider.Usage{Raw: raw}, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}})}, nil
	case "success":
		m.stats.recordSuccess(options)
		stream := make(chan provider.StreamPart, 8)
		stream <- provider.StreamPart{Type: provider.PartStreamStart, Warnings: []provider.Warning{{Type: provider.WarnOther, Message: "private warning"}}}
		stream <- provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "stream-response-1", ModelID: "private-backend", Provider: "private-provider", Timestamp: time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC)}
		stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"}
		stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: ""}
		stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "hello from Go stream"}
		stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text-1"}
		stream <- provider.StreamPart{Type: provider.PartFinish, Usage: &provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: &two, NoCache: &one, CacheRead: &one, CacheWrite: &zero},
			OutputTokens: provider.OutputTokenUsage{Total: &one, Text: &one, Reasoning: &zero},
		}, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "test-stop"}}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	case "stream-errors":
		stream := make(chan provider.StreamPart, 8)
		stream <- provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: http.StatusServiceUnavailable, Message: "private overload"})}
		stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"}
		stream <- provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: http.StatusUnauthorized, Message: "private dependency"})}
		stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "after errors"}
		stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text-1"}
		stream <- provider.StreamPart{Type: provider.PartFinish, Usage: &provider.Usage{}, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	case "stream-timeout", "stream-blocking":
		if m.kind == "stream-blocking" {
			m.stats.streamBlockingCalls.Add(1)
		}
		stream := make(chan provider.StreamPart)
		go func() {
			<-ctx.Done()
			m.stats.cancellations.Add(1)
			close(stream)
		}()
		return &provider.StreamResult{Stream: stream}, nil
	default:
		return nil, errors.New("providerwire test: unexpected stream call")
	}
}

func (m *providerWireV4Model) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	switch m.kind {
	case "native-values", "native-empty", "native-partial", "native-absent":
		return scenarioNativeValues(m.kind), nil
	case "reasoning-files":
		m.stats.recordSuccess(options)
		empty, data, url := provider.Base64DataContent(""), provider.BytesDataContent([]byte{1, 2, 3}), provider.URLDataContent("https://example.test/reasoning")
		two := 2
		return &provider.GenerateResult{Content: []provider.GenerateContentPart{
			{Type: provider.ContentReasoningFile, MediaType: "image/png", Data: &empty, ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"file","reasoningEncryptedContent":null}`)}},
			{Type: provider.ContentReasoningFile, MediaType: "image/png", Data: &data, ProviderMetadata: provider.ProviderMetadata{}},
			{Type: provider.ContentReasoningFile, MediaType: "image/png", Data: &url},
		}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: provider.Usage{OutputTokens: provider.OutputTokenUsage{Total: &two, Reasoning: &two}}}, nil
	case "reasoning":
		m.stats.recordSuccess(options)
		return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentReasoning, Text: "", ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"signature":"end-signature"}`)}}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
	case "sources":
		return &provider.GenerateResult{Content: scenarioSources(), FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
	case "hosted-deferred", "hosted-deferred-error":
		m.stats.recordSuccess(options)
		stage, err := hostedStage(options)
		if err != nil {
			return nil, err
		}
		result := &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}
		switch stage {
		case 0:
			result.Content = []provider.GenerateContentPart{hostedCallContent()}
			result.FinishReason.Unified = provider.FinishReasonToolCalls
		case 1:
			result.Content = []provider.GenerateContentPart{hostedResultContent(m.kind == "hosted-deferred-error")}
		default:
			result.Content = []provider.GenerateContentPart{{Type: provider.ContentText, Text: "finished"}}
		}
		return result, nil
	case "unary-tools", "unary-tools-provider-executed", "unary-tools-dynamic":
		m.stats.recordSuccess(options)
		for _, message := range options.Prompt {
			for _, part := range message.Content {
				if part.Type == provider.ContentPartTypeToolResult {
					if part.ToolCallID != "call-weather" || part.ToolName != "weather" || part.Output == nil || part.Output.Type != provider.ToolOutputText || part.Output.Text != "sunny" {
						return nil, errors.New("invalid function continuation")
					}
					return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "It is sunny."}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
				}
			}
		}
		call := provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: "call-weather", ToolName: "weather", Input: json.RawMessage(`{"city":"Rio"}`), ProviderMetadata: provider.ProviderMetadata{"private": json.RawMessage(`{"secret":"hidden"}`)}}
		call.ProviderExecuted = m.kind == "unary-tools-provider-executed"
		if m.kind == "unary-tools-dynamic" {
			yes := true
			call.Dynamic = &yes
		}
		return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: ""}, call}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonToolCalls}}, nil
	case "raw-usage", "raw-usage-empty":
		raw := syntheticRawUsage
		if m.kind == "raw-usage-empty" {
			raw = json.RawMessage(`{}`)
		}
		return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "ok"}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: provider.Usage{Raw: raw}}, nil
	case "success":
		m.stats.recordSuccess(options)
		zero := 0
		one := 1
		two := 2
		return &provider.GenerateResult{
			Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "hello from Go"}},
			FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "test-stop"},
			Usage: provider.Usage{
				InputTokens:  provider.InputTokenUsage{Total: &two, NoCache: &one, CacheRead: &one, CacheWrite: &zero},
				OutputTokens: provider.OutputTokenUsage{Total: &one, Text: &one, Reasoning: &zero},
			},
			Warnings: []provider.Warning{{Type: provider.WarnOther, Message: "server warning"}},
			Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{
				ID: "private-response", ModelID: "private-backend-model", Provider: "private-provider",
			}},
		}, nil
	case "blocking":
		m.stats.blockingCalls.Add(1)
		<-ctx.Done()
		m.stats.cancellations.Add(1)
		return nil, ctx.Err()
	default:
		return nil, errors.New("providerwire test: unknown model behavior")
	}
}

type providerWireV4Scenario struct {
	runtime http.Handler
	stats   *providerWireV4Stats
}

func newProviderWireV4Scenario() (*providerWireV4Scenario, error) {
	stats := &providerWireV4Stats{}
	entries := make([]catalog.StaticEntry, 0, 22)
	for _, id := range []string{"native-values", "native-empty", "native-partial", "native-absent", "reasoning-files", "reasoning", "sources", "success", "raw-usage", "raw-usage-empty", "blocking", "stream-errors", "stream-timeout", "stream-blocking", "unary-tools", "unary-tools-provider-executed", "unary-tools-dynamic", "stream-tools", "stream-tool-results", "stream-tool-arguments", "hosted-deferred", "hosted-deferred-error"} {
		entries = append(entries, catalog.StaticEntry{
			Info:  catalog.ModelInfo{ID: id},
			Model: &providerWireV4Model{kind: id, stats: stats},
		})

	}
	ordered, err := fallback.New(&providerWireV4Model{kind: "setup-failure", stats: stats}, &providerWireV4Model{kind: "success", stats: stats})
	if err != nil {
		return nil, err
	}
	entries = append(entries, catalog.StaticEntry{Info: catalog.ModelInfo{ID: "mapped-fallback"}, Model: ordered})
	resolver, err := catalog.NewStatic(entries)
	if err != nil {
		return nil, err
	}
	runtime, err := providerwirev4.New(providerwirev4.Config{
		Resolver: resolver,
		Limits: providerwirev4.Limits{
			RequestBytes:        1 << 20,
			UnaryResponseBytes:  1 << 20,
			StreamParts:         1_000,
			StreamFrameBytes:    1 << 20,
			ModelDuration:       5 * time.Second,
			StreamIdleDuration:  200 * time.Millisecond,
			StreamDrainDuration: 100 * time.Millisecond,
		},
	})
	if err != nil {
		return nil, err
	}
	return &providerWireV4Scenario{runtime: runtime, stats: stats}, nil
}

func scenarioStream(parts ...provider.StreamPart) <-chan provider.StreamPart {
	stream := make(chan provider.StreamPart, len(parts))
	for _, part := range parts {
		stream <- part
	}
	close(stream)
	return stream
}

func scenarioSources() []provider.GenerateContentPart {
	return []provider.GenerateContentPart{
		{Type: provider.ContentSource, SourceType: provider.SourceTypeURL, ID: "backend-secret", URL: "https://example.com", Title: "URL"},
		{Type: provider.ContentSource, SourceType: provider.SourceTypeDocument, ID: "backend-secret", MediaType: "text/plain", Title: "", ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"startPageNumber":1,"endPageNumber":2,"citedText":"private"}`)}},
		{Type: provider.ContentSource, SourceType: provider.SourceTypeDocument, ID: "file-native", MediaType: "application/octet-stream", Title: "file-private", Filename: "file-private", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"type":"file_path","fileId":"file-private","index":0}`)}},
	}
}

func (s *providerWireV4Scenario) register(mux *http.ServeMux) {
	strictRoute := http.StripPrefix(providerWireV4Prefix, s.runtime)
	mux.Handle("POST "+providerWireV4Prefix+providerwirev4.LanguageModelPath, strictRoute)
	mux.Handle("POST /function-tools"+providerwirev4.LanguageModelPath, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Access-Token") != "function-test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.StripPrefix("/function-tools", s.runtime).ServeHTTP(w, r)
	}))
	mux.HandleFunc("GET "+providerWireV4Prefix+"/stats", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]int64{
			"successCalls":        s.stats.successCalls.Load(),
			"streamCalls":         s.stats.streamCalls.Load(),
			"blockingCalls":       s.stats.blockingCalls.Load(),
			"streamBlockingCalls": s.stats.streamBlockingCalls.Load(),
			"cancellations":       s.stats.cancellations.Load(),
		})
	})
	// Test-only capture, never a production diagnostics surface.
	mux.HandleFunc("GET "+providerWireV4Prefix+"/options", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.stats.options())
	})
}

func registerProviderWireV4(mux *http.ServeMux) error {
	scenario, err := newProviderWireV4Scenario()
	if err != nil {
		return err
	}
	scenario.register(mux)
	return nil
}
