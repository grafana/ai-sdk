package aisdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolCallers_DeferredDiscovery(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, mode := range []string{"stream", "generate", "agent stream", "agent generate"} {
			t.Run(fmt.Sprintf("%s/nested=%t", mode, nested), func(t *testing.T) {
				calls := 0
				runs := 0
				sameStepUnavailable := false
				var bindings [][]string
				weather := discoveryTool(t)
				weather.Execute = func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
					runs++
					return json.RawMessage(`"sunny"`), nil
				}
				weather.NeedsApproval = ApprovalRequired()
				tools := ToolSet{"search": ToolSearch(), "weather": weather, "unrelated": {DeferLoading: true}}
				routes := ToolRoutes(nil)
				if nested {
					callerSchema := testMustSchema(t, `{"type":"object","properties":{"name":{"type":"string"},"input":{"type":"object"}},"required":["name","input"],"additionalProperties":false}`)
					tools["code"] = Tool{Description: "stable", InputSchema: callerSchema, Caller: &ToolCaller{Type: ToolCallerLocal,
						Bind: func(registry ToolSet) Tool {
							bindings = append(bindings, slices.Sorted(maps.Keys(registry)))
							return Tool{Description: "bound", InputSchema: callerSchema, Execute: func(ctx context.Context, input json.RawMessage, opts ToolExecutionOptions) (json.RawMessage, error) {
								var call struct {
									Name  string
									Input json.RawMessage
								}
								if err := json.Unmarshal(input, &call); err != nil {
									return nil, err
								}
								tool, ok := registry[call.Name]
								if !ok {
									return nil, fmt.Errorf("unavailable nested tool %q", call.Name)
								}
								result, err := tool.Execute(ctx, call.Input, opts)
								if call.Name == "search" {
									_, present := registry["weather"]
									sameStepUnavailable = !present
								}
								return result, err
							}}
						},
						PrepareModelMessage: func(registry ToolSet) *string {
							msg := "Catalog: " + strings.Join(slices.Sorted(maps.Keys(registry)), ", ")
							return &msg
						},
					}}
					routes = ToolRoutes{"search": {Callers: []string{"code"}}, "weather": {Callers: []string{"code"}}, "unrelated": {Callers: []string{"code"}}}
				}
				var definitions []provider.Tool
				model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
					calls++
					assert.NotContains(t, marshalJSON(t, opts), "unrelated")
					if nested {
						assert.Equal(t, []string{"code"}, toolNames(opts.Tools))
						assert.Equal(t, "stable", opts.Tools[0].Description)
						if calls == 1 {
							definitions = opts.Tools
							assert.NotContains(t, marshalJSON(t, opts.Prompt), "weather")
						} else {
							assert.Equal(t, definitions, opts.Tools)
							assert.Contains(t, marshalJSON(t, opts.Prompt), "Catalog: search, weather")
							assert.Equal(t, 1, strings.Count(marshalJSON(t, opts.Prompt), "Catalog: search, weather"))
						}
					} else {
						if calls == 1 {
							assert.Equal(t, []string{"search"}, toolNames(opts.Tools))
						} else {
							assert.Equal(t, []string{"search", "weather"}, toolNames(opts.Tools))
						}
					}
					switch calls {
					case 1:
						if nested {
							return &provider.StreamResult{Stream: discoveryParts(
								discoveryCall("nested-search", "code", `{"name":"search","input":{"query":"weather"}}`),
								discoveryCall("nested-early", "code", `{"name":"weather","input":{}}`),
							)}, nil
						}
						return &provider.StreamResult{Stream: toolCallStreamParts("search", `{"query":"weather"}`)}, nil
					case 2:
						assert.Zero(t, runs)
						if nested {
							return &provider.StreamResult{Stream: toolCallStreamParts("code", `{"name":"weather","input":{}}`)}, nil
						}
						return &provider.StreamResult{Stream: toolCallStreamParts("weather", `{}`)}, nil
					default:
						return &provider.StreamResult{Stream: textStreamParts("sunny")}, nil
					}
				}}
				shared := []Option{WithTools(tools), WithToolRoutes(routes), WithStopWhen(StepCountIs(3)), WithToolApproval(ToolApprovalMap{"weather": ApprovalPolicy(ToolApprovalApproved)})}
				opts := make([]StreamOption, 0, len(shared))
				generateOpts := make([]GenerateOption, 0, len(shared)+1)
				for _, opt := range shared {
					opts = append(opts, opt)
					generateOpts = append(generateOpts, opt)
				}
				var text string
				switch mode {
				case "stream":
					result := StreamText(t.Context(), model, append(opts, WithModelMessages(provider.UserText("Find the forecast.")))...)
					for range result.FullStream() {
					}
					require.NoError(t, result.Err())
					text = result.Text()
				case "generate":
					generateOpts = append(generateOpts, WithModelMessages(provider.UserText("Find the forecast.")))
					result, err := GenerateText(t.Context(), model, generateOpts...)
					require.NoError(t, err)
					text = result.Text
				case "agent stream":
					agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(opts...))
					result := agent.Stream(t.Context(), WithAgentPrompt("Find the forecast."))
					for range result.FullStream() {
					}
					require.NoError(t, result.Err())
					text = result.Text()
				case "agent generate":
					agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(opts...))
					result, err := agent.Generate(t.Context(), WithAgentPrompt("Find the forecast."))
					require.NoError(t, err)
					text = result.Text
				}
				assert.Equal(t, "sunny", text)
				assert.Equal(t, 1, runs)
				assert.Equal(t, 3, calls)
				if nested {
					assert.True(t, sameStepUnavailable)
					assert.Equal(t, [][]string{{"search"}, {"search", "weather"}, {"search", "weather"}}, bindings)
				}
			})
		}
	}
}

func TestToolCallers_ContextDescriptions(t *testing.T) {
	for _, announcement := range []bool{false, true} {
		t.Run(fmt.Sprintf("announcement=%t", announcement), func(t *testing.T) {
			calls := 0
			resolutions := 0
			callee := Tool{Description: "static", DescriptionFunc: func(opts ToolDescriptionOptions) string { resolutions++; return opts.Context.(string) }}
			caller := Tool{Description: "original", Caller: &ToolCaller{Type: ToolCallerLocal, Bind: func(registry ToolSet) Tool {
				assert.Equal(t, "current", registry["callee"].Description)
				assert.Nil(t, registry["callee"].DescriptionFunc)
				return Tool{DescriptionFunc: func(opts ToolDescriptionOptions) string { return "bound " + opts.Context.(string) }}
			}}}
			if announcement {
				caller.Caller.PrepareModelMessage = func(registry ToolSet) *string {
					assert.Equal(t, "current", registry["callee"].Description)
					msg := registry["callee"].Description
					return &msg
				}
			}
			registry := ToolSet{"callee": callee, "code": caller}
			model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				calls++
				if announcement {
					assert.Equal(t, "original", opts.Tools[0].Description)
				} else {
					assert.Equal(t, "bound current", opts.Tools[0].Description)
				}
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			}}
			result := StreamText(t.Context(), model, WithTools(registry), WithToolRoutes(ToolRoutes{"callee": {Callers: []string{"code"}}}), WithPrepareStep(func(PrepareStepState) (*PrepareStepResult, error) { return &PrepareStepResult{Context: "current"}, nil }))
			for range result.FullStream() {
			}
			require.NoError(t, result.Err())
			assert.Equal(t, 1, calls)
			assert.Equal(t, 1, resolutions)
			assert.NotNil(t, registry["callee"].DescriptionFunc)
			assert.Equal(t, "static", registry["callee"].Description)
			first, err := FingerprintTools(registry)
			require.NoError(t, err)
			changed := maps.Clone(registry)
			tool := changed["callee"]
			tool.DescriptionFunc = func(ToolDescriptionOptions) string { return "different" }
			changed["callee"] = tool
			second, err := FingerprintTools(changed)
			require.NoError(t, err)
			assert.Equal(t, first, second)
		})
	}
	t.Run("agent context override", func(t *testing.T) {
		calls := 0
		tools := ToolSet{"search": ToolSearch(), "weather": {DeferLoading: true, DescriptionFunc: func(opts ToolDescriptionOptions) string {
			assert.Equal(t, "meteorology", opts.Context)
			return opts.Context.(string)
		}}}
		model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			calls++
			if calls == 1 {
				return &provider.StreamResult{Stream: toolCallStreamParts("search", `{"query":"meteorology"}`)}, nil
			}
			require.Len(t, opts.Tools, 2)
			assert.Equal(t, "meteorology", opts.Tools[1].Description)
			return &provider.StreamResult{Stream: textStreamParts("done")}, nil
		}}
		agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(WithTools(tools), WithStopWhen(StepCountIs(2))), WithToolLoopAgentRuntimeContext("default"))
		result := agent.Stream(t.Context(), WithAgentRuntimeContext("meteorology"))
		for range result.FullStream() {
		}
		require.NoError(t, result.Err())
		assert.Equal(t, 2, calls)
	})
}

func TestToolSearch_ApprovalLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status        ToolApprovalStatus
		wantDiscovery bool
		wantCalls     int
	}{
		{"pending", ToolApprovalUserApproval, false, 1}, {"denied", ToolApprovalDenied, false, 2}, {"automatic", ToolApprovalApproved, true, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			search := ToolSearch()
			search.NeedsApproval = ApprovalRequired()
			model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				calls++
				if calls == 1 {
					return &provider.StreamResult{Stream: toolCallStreamParts("search", `{"query":"weather"}`)}, nil
				}
				names := []string{"search"}
				if tc.wantDiscovery {
					names = append(names, "weather")
				}
				assert.Equal(t, names, toolNames(opts.Tools))
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			}}
			result := StreamText(t.Context(), model, WithTools(ToolSet{"search": search, "weather": discoveryTool(t)}), WithStopWhen(StepCountIs(2)), WithToolApproval(ToolApprovalMap{"search": ApprovalPolicy(tc.status)}))
			for range result.FullStream() {
			}
			require.NoError(t, result.Err())
			assert.Equal(t, tc.wantCalls, calls)
			if tc.wantDiscovery {
				require.Len(t, result.Steps()[0].ToolResults, 1)
				assert.JSONEq(t, `{"tools":[{"name":"weather","description":"Weather forecast"}]}`, string(result.Steps()[0].ToolResults[0].Output))
			}
		})
	}
	for _, tc := range []struct {
		name     string
		status   ToolApprovalStatus
		invalid  bool
		wantRuns int
	}{
		{"deferred pending", ToolApprovalUserApproval, false, 0}, {"deferred denied", ToolApprovalDenied, false, 0}, {"deferred approved", ToolApprovalApproved, false, 1}, {"deferred validation", ToolApprovalApproved, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runs := 0
			calls := 0
			weather := discoveryTool(t)
			weather.NeedsApproval = ApprovalRequired()
			weather.Execute = func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				runs++
				return json.RawMessage(`"sunny"`), nil
			}
			if tc.invalid {
				weather.ValidateInput = func(json.RawMessage) error { return errors.New("invalid weather request") }
			}
			model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				calls++
				if calls == 1 {
					return &provider.StreamResult{Stream: toolCallStreamParts("search", `{"query":"weather"}`)}, nil
				}
				return &provider.StreamResult{Stream: toolCallStreamParts("weather", `{}`)}, nil
			}}
			result := StreamText(t.Context(), model, WithTools(ToolSet{"search": ToolSearch(), "weather": weather}), WithStopWhen(StepCountIs(2)), WithToolApproval(ToolApprovalMap{"weather": ApprovalPolicy(tc.status)}))
			for range result.FullStream() {
			}
			require.NoError(t, result.Err())
			assert.Equal(t, tc.wantRuns, runs)
			if tc.invalid {
				assert.True(t, result.Steps()[1].ToolCalls[0].Invalid)
			}
		})
	}
}

func TestToolSearch_HistoricalApproval(t *testing.T) {
	for _, tc := range []struct {
		name, target   string
		status         ToolApprovalStatus
		signatureValid bool
		wantRuns       int
	}{
		{"search", "search", ToolApprovalNotApplicable, true, 0}, {"callee", "weather", ToolApprovalNotApplicable, true, 1}, {"policy denied", "weather", ToolApprovalDenied, true, 0}, {"signature rejected", "weather", ToolApprovalNotApplicable, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := json.RawMessage(`{}`)
			if tc.target == "search" {
				input = json.RawMessage(`{"query":"weather"}`)
			}
			signature, err := signToolApproval([]byte("secret"), "approval", "history", tc.target, input)
			require.NoError(t, err)
			if !tc.signatureValid {
				signature = "invalid"
			}
			runs := 0
			weather := discoveryTool(t)
			weather.Execute = func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				runs++
				return json.RawMessage(`"sunny"`), nil
			}
			model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				assert.Equal(t, []string{"search"}, toolNames(opts.Tools))
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			}}
			result := StreamText(t.Context(), model, WithTools(ToolSet{"search": ToolSearch(), "weather": weather}), WithToolApprovalSecret("secret"), WithToolApproval(ToolApprovalMap{tc.target: ApprovalPolicy(tc.status)}), WithModelMessages(
				provider.NewAssistantMessage(provider.ToolCallPart("history", tc.target, input), provider.ContentPart{Type: provider.ContentPartTypeToolApprovalRequest, ApprovalID: "approval", ToolCallID: "history", ToolName: tc.target, Signature: signature}),
				provider.NewToolMessage(provider.ToolApprovalResponsePart("approval", true, "")),
			))
			var errorsSeen []StreamToolError
			for part := range result.FullStream() {
				if err, ok := part.(StreamToolError); ok {
					errorsSeen = append(errorsSeen, err)
				}
			}
			assert.Equal(t, tc.wantRuns, runs)
			if !tc.signatureValid {
				require.ErrorContains(t, result.Err(), "invalid signature")
				assert.Zero(t, model.callCount)
			} else {
				require.NoError(t, result.Err())
				assert.Equal(t, 1, model.callCount)
				if tc.target == "search" {
					require.Len(t, errorsSeen, 1)
					assert.ErrorContains(t, errorsSeen[0].Error, "must be bound")
				}
			}
		})
	}
}

func TestToolSearch_Cancellation(t *testing.T) {
	for _, mode := range []string{"after completed search", "during search", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			aborted := false
			weather := discoveryTool(t)
			if mode == "during search" {
				weather.DescriptionFunc = func(ToolDescriptionOptions) string { cancel(); <-ctx.Done(); return "weather" }
			}
			registry := ToolSet{"search": ToolSearch(), "weather": weather}
			model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				calls++
				if mode == "timeout" {
					return &provider.StreamResult{Stream: discoveryParts(discoveryCall("s", "search", `{"query":"weather"}`), discoveryCall("wait", "wait", `{}`))}, nil
				}
				return &provider.StreamResult{Stream: toolCallStreamParts("search", `{"query":"weather"}`)}, nil
			}}
			options := []StreamOption{WithTools(registry), WithStopWhen(StepCountIs(3)), OnAbort(func(OnAbortState) { aborted = true }), OnChunk(func(state OnChunkState) {
				if result, ok := state.Chunk.(StreamToolResult); ok && result.ToolName == "search" && mode == "after completed search" {
					cancel()
				}
			})}
			if mode == "timeout" {
				registry["wait"] = Tool{Execute: func(ctx context.Context, _ json.RawMessage, _ ToolExecutionOptions) (json.RawMessage, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				}}
				options = append(options, WithTimeout(TimeoutConfig{Total: 25 * time.Millisecond}))
			}
			result := StreamText(ctx, model, options...)
			for range result.FullStream() {
			}
			assert.Equal(t, 1, calls)
			assert.True(t, aborted)
			laterModel := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				assert.Contains(t, toolNames(opts.Tools), "search")
				assert.NotContains(t, toolNames(opts.Tools), "weather")
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			}}
			later := StreamText(t.Context(), laterModel, WithTools(registry))
			for range later.FullStream() {
			}
			require.NoError(t, later.Err())
		})
	}
}
