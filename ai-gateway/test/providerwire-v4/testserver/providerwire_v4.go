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
	"github.com/grafana/ai-sdk/provider"
)

const providerWireV4Prefix = "/providerwire-v4"

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
	entries := make([]catalog.StaticEntry, 0, 5)
	for _, id := range []string{"success", "blocking", "stream-errors", "stream-timeout", "stream-blocking", "unary-tools", "unary-tools-provider-executed", "unary-tools-dynamic", "stream-tools", "stream-tool-results"} {
		entries = append(entries, catalog.StaticEntry{
			Info:  catalog.ModelInfo{ID: id},
			Model: &providerWireV4Model{kind: id, stats: stats},
		})
	}
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
}

func registerProviderWireV4(mux *http.ServeMux) error {
	scenario, err := newProviderWireV4Scenario()
	if err != nil {
		return err
	}
	scenario.register(mux)
	return nil
}
