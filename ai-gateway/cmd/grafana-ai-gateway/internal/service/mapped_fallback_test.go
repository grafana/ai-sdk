package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFallbackRoute_MappedOptions(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, tc := range []struct {
			name string
			opts provider.CallOptions
		}{
			{"functions", provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{}`), ProviderOptions: mappedFallbackOptions()}}}},
			{"auto", provider.CallOptions{ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceAuto}}},
			{"none", provider.CallOptions{ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceNone}}},
			{"required", provider.CallOptions{ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceRequired}}},
			{"named", provider.CallOptions{ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "lookup"}}},
			{"history", provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(provider.ToolCallPart("call", "lookup", json.RawMessage(`{}`))), provider.NewToolMessage(provider.ToolResultPart("call", "lookup", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`null`)}))}}},
			{"files", mappedFallbackFiles()},
			{"reasoning", provider.CallOptions{Reasoning: provider.ReasoningHigh, Prompt: []provider.Message{provider.NewAssistantMessage(provider.ReasoningPart("thought"), provider.ReasoningFilePart("image/png", provider.BytesDataContent(nil)), provider.ReasoningFilePart("image/png", provider.URLDataContent("https://example.test/reasoning")))}}},
			{"headers", provider.CallOptions{Headers: map[string]string{"x-ordinary": "value"}}},
			{"scoped options", provider.CallOptions{ProviderOptions: mappedFallbackOptions(), Prompt: []provider.Message{func() provider.Message {
				message := provider.UserText("request")
				message.ProviderOptions = mappedFallbackOptions()
				message.Content[0].ProviderOptions = mappedFallbackOptions()
				return message
			}()}}},
		} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, streaming), func(t *testing.T) {
				original, err := json.Marshal(tc.opts)
				require.NoError(t, err)
				var calls []string
				created, err := buildCatalog(fallbackCatalogFile(), fallbackProviders(), http.DefaultClient, func(_ string, id string, _ ...anthropicprovider.Option) provider.LanguageModel {
					invoke := func(got provider.CallOptions) error {
						calls = append(calls, id)
						assert.Equal(t, tc.opts, got)
						if id == "backend-primary" {
							return hostileFallbackError(http.StatusServiceUnavailable)
						}
						return nil
					}
					return &observabilityTestModel{
						generate: func(_ context.Context, got provider.CallOptions) (*provider.GenerateResult, error) {
							if err := invoke(got); err != nil {
								return nil, err
							}
							return &provider.GenerateResult{}, nil
						},
						stream: func(_ context.Context, got provider.CallOptions) (*provider.StreamResult, error) {
							if err := invoke(got); err != nil {
								return nil, err
							}
							return fallbackParts(provider.StreamPart{Type: provider.PartStreamStart}), nil
						},
					}
				}, identityModelFactory)
				require.NoError(t, err)
				resolved, err := created.ResolveModel(t.Context(), "alias")
				require.NoError(t, err)
				for range 2 {
					if streaming {
						result, err := resolved.Model.DoStream(t.Context(), tc.opts)
						require.NoError(t, err)
						for range result.Stream {
						}
					} else {
						_, err := resolved.Model.DoGenerate(t.Context(), tc.opts)
						require.NoError(t, err)
					}
				}
				assert.Equal(t, []string{"backend-primary", "backend-secondary", "backend-primary", "backend-secondary"}, calls)
				after, err := json.Marshal(tc.opts)
				require.NoError(t, err)
				assert.Equal(t, original, after)
			})
		}
	}
}

func mappedFallbackOptions() provider.ProviderOptions {
	return provider.ProviderOptions{
		"vendor": provider.RawProviderOption{Key: "vendor", Raw: json.RawMessage(`{"null":null,"false":false,"zero":0,"empty":"","array":[],"object":{}}`)},
		"other":  provider.RawProviderOption{Key: "other", Raw: json.RawMessage(`{}`)},
	}
}

func mappedFallbackFiles() provider.CallOptions {
	var parts []provider.ContentPart
	for index, data := range []provider.DataContent{provider.BytesDataContent(nil), provider.TextDataContent(""), provider.URLDataContent("https://example.test/file"), provider.ReferenceDataContent(json.RawMessage(`{"openai":"file-id"}`))} {
		part := provider.FilePart("text/plain", data)
		if index > 0 {
			filename := ""
			if index > 1 {
				filename = "file.txt"
			}
			part.Filename = &filename
		}
		part.ProviderOptions = mappedFallbackOptions()
		parts = append(parts, part)
	}
	return provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(parts...)}}
}

func TestFallbackRoute_ConcurrentMappedObservation(t *testing.T) {
	const count = 12
	var logs lockedBuffer
	var output physicalBuffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	telemetry, err := NewTelemetry(logger)
	require.NoError(t, err)
	factory, err := NewModelObservabilityFactory(telemetry, logger, nil, 10*time.Millisecond)
	require.NoError(t, err)
	sink := newPhysicalAttemptSink(&output, telemetry, count*2, time.Second, time.Second)
	defer sink.Close()
	created, err := buildCatalog(fallbackCatalogFile(), fallbackProviders(), http.DefaultClient, func(_ string, id string, _ ...anthropicprovider.Option) provider.LanguageModel {
		invoke := func(ctx context.Context, opts provider.CallOptions) error {
			marker := opts.Headers["x-request"]
			assert.Equal(t, marker, observationFromContext(ctx).correlationID)
			assert.Equal(t, mappedFallbackFiles().Prompt, opts.Prompt)
			assert.Equal(t, provider.RawProviderOption{Key: "vendor", Raw: json.RawMessage(fmt.Sprintf(`{"request":%q}`, marker))}, opts.ProviderOptions["vendor"])
			if id == "backend-primary" {
				return hostileFallbackError(503)
			}
			return nil
		}
		return &observabilityTestModel{
			generate: func(ctx context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
				if err := invoke(ctx, opts); err != nil {
					return nil, err
				}
				return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
			},
			stream: func(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				if err := invoke(ctx, opts); err != nil {
					return nil, err
				}
				return fallbackParts(provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}), nil
			},
		}
	}, factory, sink)
	require.NoError(t, err)
	resolved, err := created.ResolveModel(t.Context(), "alias")
	require.NoError(t, err)
	var wg sync.WaitGroup
	for index := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			marker := fmt.Sprintf("request-%d", index)
			state := &telemetryState{}
			state.observation.Store(&requestObservation{correlationID: marker})
			ctx := context.WithValue(t.Context(), telemetryStateKey{}, state)
			opts := mappedFallbackFiles()
			opts.Headers = map[string]string{"x-request": marker}
			opts.ProviderOptions = provider.ProviderOptions{"vendor": provider.RawProviderOption{Key: "vendor", Raw: json.RawMessage(fmt.Sprintf(`{"request":%q}`, marker))}}
			original, err := json.Marshal(opts)
			require.NoError(t, err)
			if index%2 == 0 {
				result, err := resolved.Model.DoStream(ctx, opts)
				require.NoError(t, err)
				for range result.Stream {
				}
			} else {
				_, err := resolved.Model.DoGenerate(ctx, opts)
				require.NoError(t, err)
			}
			after, err := json.Marshal(opts)
			require.NoError(t, err)
			assert.Equal(t, original, after)
		}()
	}
	wg.Wait()
	sink.Close()
	for _, mode := range []string{"generate", "stream"} {
		assert.Equal(t, count/2, countModelLogEvent(t, logs.String(), "aisdk.model."+mode+".start"))
		assert.Equal(t, count/2, countModelLogEvent(t, logs.String(), "aisdk.model."+mode+".finish"))
	}
	decisions := make(map[string][]physicalAttemptRecord)
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var record physicalAttemptRecord
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		decisions[record.CorrelationID] = append(decisions[record.CorrelationID], record)
	}
	require.Len(t, decisions, count)
	for index := range count {
		records := decisions[fmt.Sprintf("request-%d", index)]
		require.Len(t, records, 2)
		assert.Equal(t, 1, records[0].CandidateIndex)
		assert.True(t, records[0].WillFallback)
		assert.Equal(t, 2, records[1].CandidateIndex)
		assert.True(t, records[1].Winner)
	}
}
