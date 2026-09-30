package aisdk

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"sync"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func executeSearch(t *testing.T, tool Tool, query string) []toolSearchMatch {
	t.Helper()
	input, err := json.Marshal(map[string]string{"query": query})
	require.NoError(t, err)
	output, err := tool.Execute(t.Context(), input, ToolExecutionOptions{})
	require.NoError(t, err)
	require.NoError(t, ToolSearch().OutputSchema.Validate(output))
	var result struct {
		Tools []toolSearchMatch `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(output, &result))
	require.NotNil(t, result.Tools)
	return result.Tools
}

func discoveryTool(t *testing.T) Tool {
	t.Helper()
	return Tool{DeferLoading: true, Description: "Weather forecast", InputSchema: testMustSchema(t, `{"type":"object","properties":{},"additionalProperties":false}`), Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
		return json.RawMessage(`"sunny"`), nil
	}}
}

func discoveryCaller() Tool {
	return Tool{Caller: &ToolCaller{Type: ToolCallerLocal, Bind: func(ToolSet) Tool { return Tool{} }, PrepareModelMessage: func(ToolSet) *string { message := "catalog"; return &message }}}
}

func TestToolSearch_Contract(t *testing.T) {
	tool := ToolSearch()
	assert.Equal(t, UserToolFunction, tool.Type)
	assert.Equal(t, toolSearchDescription, tool.Description)
	for _, tc := range []struct {
		name, input string
		valid       bool
	}{
		{"query", `{"query":"weather"}`, true}, {"whitespace", `{"query":" "}`, true}, {"empty", `{"query":""}`, false}, {"extra", `{"query":"weather","limit":1}`, false}, {"missing", `{}`, false}, {"wrong type", `{"query":1}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tool.InputSchema.Validate(json.RawMessage(tc.input))
			if tc.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
	t.Run("unbound", func(t *testing.T) {
		_, err := tool.Execute(t.Context(), json.RawMessage(`{"query":"weather"}`), ToolExecutionOptions{})
		require.ErrorContains(t, err, "must be bound")
	})
	t.Run("copy and arbitrary name", func(t *testing.T) {
		copied := tool
		copied.Description = "Custom search"
		copied.NeedsApproval = ApprovalRequired()
		registry := ToolSet{"find": copied, "getWeather": discoveryTool(t)}
		state, err := newToolSearchState(registry, nil)
		require.NoError(t, err)
		first := state.prepare(registry, nil, false, nil)
		matches := executeSearch(t, first["find"], "weather")
		require.Len(t, matches, 1)
		assert.Equal(t, "getWeather", matches[0].Name)
		assert.Equal(t, "Custom search", first["find"].Description)
		assert.Equal(t, copied.NeedsApproval, first["find"].NeedsApproval)
		assert.NotContains(t, first, "getWeather")
		assert.Contains(t, state.prepare(registry, nil, false, nil), "getWeather")
		_, err = registry["find"].Execute(t.Context(), json.RawMessage(`{"query":"weather"}`), ToolExecutionOptions{})
		assert.ErrorContains(t, err, "must be bound")
	})
	t.Run("ordinary tool is not search", func(t *testing.T) {
		state, err := newToolSearchState(ToolSet{"search": {Description: tool.Description}}, nil)
		require.NoError(t, err)
		assert.Nil(t, state)
	})
}

func TestToolSearch_Descriptions(t *testing.T) {
	for _, tc := range []struct {
		name string
		tool Tool
		want *string
	}{
		{name: "absent"}, {name: "static", tool: Tool{Description: "static"}, want: new("static")},
		{name: "callback", tool: Tool{Description: "ignored", DescriptionFunc: func(opts ToolDescriptionOptions) string { return opts.Context.(string) }}, want: new("current")},
		{name: "empty callback", tool: Tool{Description: "ignored", DescriptionFunc: func(ToolDescriptionOptions) string { return "" }}, want: new("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, resolveToolDescription(tc.tool, "current"))
			original := ToolSet{"tool": tc.tool}
			resolved := resolveToolDescriptions(original, "current")
			if tc.want != nil {
				assert.Equal(t, *tc.want, resolved["tool"].Description)
			}
			assert.Nil(t, resolved["tool"].DescriptionFunc)
			if tc.tool.DescriptionFunc != nil {
				assert.NotNil(t, original["tool"].DescriptionFunc)
			}
			assert.Equal(t, tc.tool.Description, original["tool"].Description)
			defs, _ := toolSetToProviderTools(resolved)
			require.Len(t, defs, 1)
			assert.Equal(t, resolved["tool"].Description, defs[0].Description)
		})
	}
	t.Run("execution context and presence", func(t *testing.T) {
		registry := ToolSet{"search": ToolSearch(), "capability": {DeferLoading: true, DescriptionFunc: func(opts ToolDescriptionOptions) string { return opts.Context.(string) }}, "emptyWeather": {DeferLoading: true, DescriptionFunc: func(ToolDescriptionOptions) string { return "" }}, "noWeather": {DeferLoading: true}}
		state, err := newToolSearchState(registry, nil)
		require.NoError(t, err)
		first := state.prepare(registry, nil, false, "Meteorology")
		matches := executeSearch(t, first["search"], "meteorology")
		require.Len(t, matches, 1)
		assert.Equal(t, new("Meteorology"), matches[0].Description)
		matches = executeSearch(t, first["search"], "weather")
		require.Len(t, matches, 2)
		assert.Equal(t, new(""), matches[0].Description)
		assert.Nil(t, matches[1].Description)
		second := resolveToolDescriptions(state.prepare(registry, nil, false, "Atmosphere"), "Atmosphere")
		assert.Equal(t, "Atmosphere", second["capability"].Description)
		assert.NotNil(t, registry["capability"].DescriptionFunc)
	})
	t.Run("typed tool can defer", func(t *testing.T) {
		tool, err := TypedTool(TypedToolDef[struct{}, string]{Execute: func(context.Context, struct{}, ToolExecutionOptions) (string, error) { return "ok", nil }})
		require.NoError(t, err)
		tool.DeferLoading = true
		tool.DescriptionFunc = func(ToolDescriptionOptions) string { return "typed capability" }
		state, err := newToolSearchState(ToolSet{"typed": tool, "search": ToolSearch()}, nil)
		require.NoError(t, err)
		matches := executeSearch(t, state.prepare(ToolSet{"typed": tool, "search": ToolSearch()}, nil, false, nil)["search"], "capability")
		require.Len(t, matches, 1)
		assert.Equal(t, "typed", matches[0].Name)
	})
}

func TestToolSearch_RouteValidation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		caller      Tool
		deferSearch bool
		routes      ToolRoutes
		want        string
	}{
		{name: "direct"}, {name: "empty", routes: ToolRoutes{"weather": {}}}, {name: "local", caller: discoveryCaller(), routes: ToolRoutes{"weather": {Callers: []string{"code"}}}},
		{name: "provider", caller: Tool{Caller: &ToolCaller{Type: ToolCallerProvider, PrepareProviderOptions: func(opts provider.ProviderOptions) provider.ProviderOptions { return opts }}}, routes: ToolRoutes{"weather": {Callers: []string{"code"}}}, want: "local caller"},
		{name: "description-only local", caller: Tool{Caller: &ToolCaller{Type: ToolCallerLocal, Bind: func(ToolSet) Tool { return Tool{} }}}, routes: ToolRoutes{"weather": {Callers: []string{"code"}}}, want: "PrepareModelMessage"},
		{name: "unknown caller", routes: ToolRoutes{"weather": {Callers: []string{"unknown"}}}, want: "invalid caller"},
		{name: "deferred search", deferSearch: true, want: "must not defer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			search := ToolSearch()
			search.DeferLoading = tc.deferSearch
			registry := ToolSet{"search": search, "weather": discoveryTool(t), "code": tc.caller}
			model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			}}
			result := StreamText(t.Context(), model, WithTools(registry), WithToolRoutes(tc.routes), WithActiveTools())
			for range result.FullStream() {
			}
			if tc.want != "" {
				require.ErrorContains(t, result.Err(), tc.want)
				assert.Zero(t, model.callCount)
			} else {
				require.NoError(t, result.Err())
				assert.Equal(t, 1, model.callCount)
			}
		})
	}
}

func TestToolSearch_CandidateFiltering(t *testing.T) {
	for _, tc := range []struct {
		name   string
		routes ToolRoutes
		active []string
		set    bool
		want   bool
	}{
		{name: "omitted", want: true}, {name: "explicit direct", routes: ToolRoutes{"search": {Direct: true}, "weather": {Direct: true}}, want: true},
		{name: "empty search route", routes: ToolRoutes{"search": {}}}, {name: "empty candidate route", routes: ToolRoutes{"weather": {}}},
		{name: "local to direct", routes: ToolRoutes{"search": {Callers: []string{"code"}}}}, {name: "direct to local", routes: ToolRoutes{"weather": {Callers: []string{"code"}}}},
		{name: "shared local", routes: ToolRoutes{"search": {Callers: []string{"code"}}, "weather": {Callers: []string{"code"}}}, want: true},
		{name: "distinct local", routes: ToolRoutes{"search": {Callers: []string{"code"}}, "weather": {Callers: []string{"other"}}}},
		{name: "direct plus local", routes: ToolRoutes{"search": {Direct: true, Callers: []string{"code"}}}, want: true},
		{name: "inactive caller", routes: ToolRoutes{"search": {Callers: []string{"code"}}, "weather": {Callers: []string{"code"}}}, active: []string{"search", "weather"}, set: true},
		{name: "excluded", active: []string{"search"}, set: true}, {name: "empty", active: []string{}, set: true}, {name: "unknown", active: []string{"missing"}, set: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := ToolSet{"search": ToolSearch(), "weather": discoveryTool(t), "code": discoveryCaller(), "other": discoveryCaller()}
			state, err := newToolSearchState(registry, tc.routes)
			require.NoError(t, err)
			first := state.prepare(registry, tc.active, tc.set, nil)
			assert.NotContains(t, first, "weather")
			if search, ok := first["search"]; ok {
				matches := executeSearch(t, search, "weather")
				if tc.want {
					require.Len(t, matches, 1)
					assert.Equal(t, "weather", matches[0].Name)
				} else {
					assert.Empty(t, matches)
				}
			} else {
				assert.Empty(t, first)
			}
			assert.Equal(t, tc.want, state.prepare(registry, tc.active, tc.set, nil)["weather"].DeferLoading)
		})
	}
	t.Run("reactivation and repeat discovery", func(t *testing.T) {
		registry := ToolSet{"search": ToolSearch(), "weather": discoveryTool(t)}
		state, err := newToolSearchState(registry, nil)
		require.NoError(t, err)
		executeSearch(t, state.prepare(registry, nil, false, nil)["search"], "weather")
		assert.NotContains(t, state.prepare(registry, []string{"search"}, true, nil), "weather")
		restored := state.prepare(registry, nil, false, nil)
		assert.Contains(t, restored, "weather")
		assert.Len(t, executeSearch(t, restored["search"], "weather"), 1)
	})
	t.Run("no search", func(t *testing.T) {
		registry := ToolSet{"weather": discoveryTool(t)}
		state, err := newToolSearchState(registry, nil)
		require.NoError(t, err)
		assert.Empty(t, state.prepare(registry, nil, false, nil))
	})
}

func TestToolSearch_Ranking(t *testing.T) {
	for _, tc := range []struct {
		name, query string
		want        []string
	}{
		{"name before description", "WEATHER", []string{"getWeather", "a", "b", "c", "d"}},
		{"duplicate terms", "weather weather WEATHER", []string{"getWeather", "a", "b", "c", "d"}},
		{"punctuation", ".*", []string{}}, {"whitespace", "  ", []string{}}, {"unrelated", "email", []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry := ToolSet{"search": ToolSearch(), "getWeather": {DeferLoading: true, Description: "Weather weather forecast"}}
			for _, name := range []string{"f", "e", "d", "c", "b", "a"} {
				registry[name] = Tool{DeferLoading: true, Description: "Weather forecast"}
			}
			state, err := newToolSearchState(registry, nil)
			require.NoError(t, err)
			first := state.prepare(registry, nil, false, nil)
			matches := executeSearch(t, first["search"], tc.query)
			names := make([]string, 0, len(matches))
			for _, match := range matches {
				names = append(names, match.Name)
			}
			assert.Equal(t, tc.want, names)
			next := state.prepare(registry, nil, false, nil)
			assert.Len(t, next, len(matches)+1)
			assert.Equal(t, []string{"search"}, slices.Sorted(maps.Keys(first)))
			assert.Equal(t, matches, executeSearch(t, next["search"], tc.query))
		})
	}
	for _, tc := range []struct {
		input string
		want  []string
	}{
		{"getWeather2Forecast", []string{"get", "weather2", "forecast"}},
		{"HTTPWeather", []string{"httpweather"}}, {"café_日本語_١٢Ⅷ", []string{"café", "日本語", "١٢ⅷ"}},
		{"ΟΣ", []string{"ος"}}, {"İSTANBUL", []string{"i", "stanbul"}}, {"aAB", []string{"a", "ab"}},
	} {
		t.Run(tc.input, func(t *testing.T) { assert.Equal(t, tc.want, tokenizeToolSearch(tc.input)) })
	}
}

func TestToolSearch_ConcurrentState(t *testing.T) {
	registry := ToolSet{"search": ToolSearch(), "otherSearch": ToolSearch(), "weather": {DeferLoading: true}, "stock": {DeferLoading: true}}
	state, err := newToolSearchState(registry, nil)
	require.NoError(t, err)
	first := state.prepare(registry, nil, false, nil)
	var wg sync.WaitGroup
	for _, name := range []string{"weather", "stock"} {
		wg.Go(func() { executeSearch(t, first["search"], name); executeSearch(t, first["otherSearch"], name) })
	}
	wg.Wait()
	assert.Len(t, first, 2)
	assert.Len(t, state.prepare(registry, nil, false, nil), 4)
	for range 8 {
		wg.Go(func() {
			own, err := newToolSearchState(registry, nil)
			assert.NoError(t, err)
			prepared := own.prepare(registry, nil, false, nil)
			assert.Len(t, prepared, 2)
			executeSearch(t, prepared["search"], "weather")
			assert.Len(t, own.prepare(registry, nil, false, nil), 3)
		})
	}
	wg.Wait()
	_, err = registry["search"].Execute(t.Context(), json.RawMessage(`{"query":"weather"}`), ToolExecutionOptions{})
	assert.Error(t, err)
	t.Run("callbacks outside lock", func(t *testing.T) {
		registry := ToolSet{"search": ToolSearch()}
		var state *toolSearchState
		registry["weather"] = Tool{DeferLoading: true, DescriptionFunc: func(ToolDescriptionOptions) string { state.prepare(registry, nil, false, nil); return "weather" }}
		state, err = newToolSearchState(registry, nil)
		require.NoError(t, err)
		assert.Len(t, executeSearch(t, state.prepare(registry, nil, false, nil)["search"], "weather"), 1)
	})
}

func discoveryParts(calls ...provider.StreamPart) <-chan provider.StreamPart {
	stream := make(chan provider.StreamPart, len(calls)+1)
	for _, call := range calls {
		stream <- call
	}
	stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
	close(stream)
	return stream
}

func discoveryCall(id, name, input string) provider.StreamPart {
	return provider.StreamPart{Type: provider.PartToolCall, ToolCallID: id, ToolName: name, Input: input}
}

func TestStreamText_DeferredToolDiscovery(t *testing.T) {
	t.Run("early rejection and next step", func(t *testing.T) {
		runs := 0
		callbacks := 0
		weather := discoveryTool(t)
		weather.Execute = func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
			runs++
			return json.RawMessage(`"sunny"`), nil
		}
		weather.OnInputAvailable = func(json.RawMessage, ToolExecutionOptions) { callbacks++ }
		var requests []provider.CallOptions
		model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			step := len(requests)
			requests = append(requests, opts)
			switch step {
			case 0:
				return &provider.StreamResult{Stream: discoveryParts(discoveryCall("s", "search", `{"query":"weather"}`), discoveryCall("early", "weather", `{}`))}, nil
			case 1:
				assert.Zero(t, runs)
				assert.Zero(t, callbacks)
				return &provider.StreamResult{Stream: discoveryParts(discoveryCall("w", "weather", `{}`))}, nil
			default:
				return &provider.StreamResult{Stream: textStreamParts("sunny")}, nil
			}
		}}
		result := StreamText(t.Context(), model, WithTools(ToolSet{"search": ToolSearch(), "weather": weather, "unrelated": {DeferLoading: true}}), WithModelMessages(provider.UserText("weather")), WithStopWhen(StepCountIs(3)))
		var chunks []UIMessageChunk
		for chunk := range result.ToUIMessageStream() {
			chunks = append(chunks, chunk)
		}
		require.NoError(t, result.Err())
		require.Len(t, requests, 3)
		assert.Equal(t, []string{"search"}, toolNames(requests[0].Tools))
		assert.Equal(t, []string{"search", "weather"}, toolNames(requests[1].Tools))
		assert.Equal(t, requests[0].Tools[0], requests[1].Tools[0])
		assert.Equal(t, 1, runs)
		assert.Equal(t, 1, callbacks)
		for _, request := range requests {
			assert.NotContains(t, marshalJSON(t, request), "unrelated")
			count := 0
			for _, msg := range request.Prompt {
				if msg.Role == provider.RoleUser {
					count++
				}
			}
			assert.Equal(t, 1, count)
		}
		errors := 0
		for _, chunk := range chunks {
			if chunk.ToolCallID == "early" {
				if chunk.Type == ChunkToolInputError || chunk.Type == ChunkToolOutputError {
					errors++
					assert.Nil(t, chunk.Dynamic)
				}
			}
		}
		assert.Equal(t, 2, errors)
	})
	for _, tc := range []struct {
		name string
		opts []StreamOption
		want [][]string
	}{
		{name: "default one step", want: [][]string{{"search"}}},
		{name: "empty", opts: []StreamOption{WithActiveTools()}, want: [][]string{nil}},
		{name: "reactivation", opts: []StreamOption{WithStopWhen(StepCountIs(3)), WithPrepareStep(func(state PrepareStepState) (*PrepareStepResult, error) {
			if state.StepNumber == 1 {
				return &PrepareStepResult{ActiveTools: []string{"search"}}, nil
			}
			return nil, nil
		})}, want: [][]string{{"search"}, {"search"}, {"search", "weather"}}},
		{name: "empty step", opts: []StreamOption{WithStopWhen(StepCountIs(3)), WithPrepareStep(func(state PrepareStepState) (*PrepareStepResult, error) {
			if state.StepNumber == 1 {
				return &PrepareStepResult{ActiveTools: []string{}}, nil
			}
			return nil, nil
		})}, want: [][]string{{"search"}, nil}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var seen [][]string
			model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				seen = append(seen, toolNames(opts.Tools))
				if len(seen) < 3 && len(opts.Tools) > 0 {
					return &provider.StreamResult{Stream: toolCallStreamParts("search", `{"query":"weather"}`)}, nil
				}
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			}}
			opts := append([]StreamOption{WithTools(ToolSet{"search": ToolSearch(), "weather": discoveryTool(t)})}, tc.opts...)
			result := StreamText(t.Context(), model, opts...)
			for range result.FullStream() {
			}
			require.NoError(t, result.Err())
			assert.Equal(t, tc.want, seen)
		})
	}
}

func TestToolSearch_StreamingInputClassification(t *testing.T) {
	for _, tc := range []struct {
		name        string
		toolType    UserToolType
		wantDynamic *bool
	}{
		{name: "static", toolType: UserToolFunction},
		{name: "dynamic", toolType: UserToolDynamic, wantDynamic: new(true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			callbacks := 0
			weather := discoveryTool(t)
			weather.Type = tc.toolType
			weather.OnInputStart = func(ToolExecutionOptions) { callbacks++ }
			weather.OnInputDelta = func(string, ToolExecutionOptions) { callbacks++ }
			weather.OnInputAvailable = func(json.RawMessage, ToolExecutionOptions) { callbacks++ }
			model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: discoveryParts(
					provider.StreamPart{Type: provider.PartToolInputStart, ID: "early", ToolName: "weather"},
					provider.StreamPart{Type: provider.PartToolInputDelta, ID: "early", Delta: `{}`},
					discoveryCall("early", "weather", `{}`),
				)}, nil
			}}
			result := StreamText(t.Context(), model, WithTools(ToolSet{"weather": weather}))
			seen := 0
			for chunk := range result.ToUIMessageStream() {
				switch chunk.Type {
				case ChunkToolInputStart, ChunkToolInputError, ChunkToolOutputError:
					seen++
					assert.Equal(t, tc.wantDynamic, chunk.Dynamic)
				}
			}
			require.NoError(t, result.Err())
			assert.Equal(t, 3, seen)
			assert.Zero(t, callbacks)
		})
	}
}

func TestToolSearch_SiblingExecutions(t *testing.T) {
	calls := 0
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		calls++
		if calls == 1 {
			assert.Equal(t, []string{"otherSearch", "search"}, toolNames(opts.Tools))
			return &provider.StreamResult{Stream: discoveryParts(discoveryCall("s1", "search", `{"query":"weather"}`), discoveryCall("s2", "otherSearch", `{"query":"stock"}`))}, nil
		}
		assert.Equal(t, []string{"otherSearch", "search", "stock", "weather"}, toolNames(opts.Tools))
		return &provider.StreamResult{Stream: textStreamParts("done")}, nil
	}}
	result := StreamText(t.Context(), model, WithTools(ToolSet{"search": ToolSearch(), "otherSearch": ToolSearch(), "weather": {DeferLoading: true}, "stock": {DeferLoading: true}}), WithStopWhen(StepCountIs(2)))
	for range result.FullStream() {
	}
	require.NoError(t, result.Err())
	assert.Equal(t, 2, calls)
	require.Len(t, result.Steps()[0].ToolResults, 2)
}

func TestToolSearch_ProviderLifecycle(t *testing.T) {
	executed := false
	tools := ToolSet{"search": ToolSearch(), "web": {Type: UserToolProvider, ID: "test.web", Args: map[string]json.RawMessage{"a": json.RawMessage(`true`)}, DeferLoading: true, Description: "web", Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
		executed = true
		return nil, nil
	}}}
	var seen [][]provider.Tool
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		seen = append(seen, opts.Tools)
		if len(seen) == 1 {
			return &provider.StreamResult{Stream: toolCallStreamParts("search", `{"query":"web"}`)}, nil
		}
		call := discoveryCall("w", "web", `{}`)
		call.ProviderExecuted = true
		unknown := discoveryCall("u", "unknown", `{}`)
		unknown.ProviderExecuted = true
		unknown.Dynamic = new(true)
		return &provider.StreamResult{Stream: discoveryParts(call, unknown)}, nil
	}}
	result := StreamText(t.Context(), model, WithTools(tools), WithStopWhen(StepCountIs(2)))
	for range result.FullStream() {
	}
	require.NoError(t, result.Err())
	require.Len(t, seen, 2)
	require.Len(t, seen[1], 2)
	assert.Equal(t, provider.ToolTypeProvider, seen[1][1].Type)
	assert.Equal(t, "test.web", seen[1][1].ID)
	assert.Equal(t, tools["web"].Args, seen[1][1].Args)
	assert.False(t, executed)
	require.Len(t, result.ToolCalls(), 2)
	assert.Equal(t, new(true), result.ToolCalls()[1].Dynamic)
}

func TestToolSearch_IndependentGenerations(t *testing.T) {
	registry := ToolSet{"search": ToolSearch(), "weather": discoveryTool(t)}
	var wg sync.WaitGroup
	for index := range 8 {
		wg.Go(func() {
			calls := 0
			model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				calls++
				if calls == 1 {
					assert.Equal(t, []string{"search"}, toolNames(opts.Tools))
					if index%2 == 0 {
						return &provider.StreamResult{Stream: toolCallStreamParts("search", `{"query":"weather"}`)}, nil
					}
				} else {
					assert.Equal(t, []string{"search", "weather"}, toolNames(opts.Tools))
				}
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			}}
			result := StreamText(t.Context(), model, WithTools(registry), WithStopWhen(StepCountIs(2)))
			for range result.FullStream() {
			}
			assert.NoError(t, result.Err())
		})
	}
	wg.Wait()
	assert.NotNil(t, registry["weather"].Execute)
}

func TestToolSearch_InvalidInputDoesNotDiscover(t *testing.T) {
	for _, input := range []string{`{"query":""}`, `{"query":"weather","extra":true}`, `{"query":1}`} {
		t.Run(input, func(t *testing.T) {
			calls := 0
			model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				assert.Equal(t, []string{"search"}, toolNames(opts.Tools))
				calls++
				if calls == 1 {
					return &provider.StreamResult{Stream: toolCallStreamParts("search", input)}, nil
				}
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			}}
			result := StreamText(t.Context(), model, WithTools(ToolSet{"search": ToolSearch(), "weather": discoveryTool(t)}), WithStopWhen(StepCountIs(2)))
			for range result.FullStream() {
			}
			require.NoError(t, result.Err())
			require.NotEmpty(t, result.Steps()[0].ToolCalls)
			assert.True(t, result.Steps()[0].ToolCalls[0].Invalid)
		})
	}
}
