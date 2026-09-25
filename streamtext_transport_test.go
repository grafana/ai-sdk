package aisdk

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamText_TransportHeaders(t *testing.T) {
	for _, tc := range []struct {
		name         string
		eventHeaders map[string]string
		want         map[string]string
	}{
		{name: "stream result fallback", want: map[string]string{"x-transport": "from-http"}},
		{name: "event overrides", eventHeaders: map[string]string{"x-event": "from-part"}, want: map[string]string{"x-event": "from-part"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				ch := make(chan provider.StreamPart, 3)
				ch <- provider.StreamPart{Type: provider.PartStreamStart}
				ch <- provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "resp_1", ResponseHeaders: tc.eventHeaders}
				ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
				close(ch)
				return &provider.StreamResult{
					Stream:   ch,
					Request:  &provider.RequestMetadata{Body: json.RawMessage(`{"prompt":"hello"}`)},
					Response: &provider.ResponseHeaders{Headers: map[string]string{"x-transport": "from-http"}},
				}, nil
			}}
			var callback OnStepFinishState
			result := StreamText(context.Background(), m,
				WithModelMessages(provider.UserText("hello")),
				OnStepFinish(func(state OnStepFinishState) { callback = state }),
			)
			for range result.FullStream() {
			}
			steps := result.Steps()
			require.Len(t, steps, 1)
			assert.Equal(t, tc.want, steps[0].Response.Headers)
			assert.Equal(t, tc.want, callback.Response.Headers)
			assert.Equal(t, tc.want, result.Response().Headers)
			assert.JSONEq(t, `{"prompt":"hello"}`, string(steps[0].Request.Body))
		})
	}
}

func TestStreamText_MultiStepTransportMetadata(t *testing.T) {
	calls := 0
	var callbacks []OnStepFinishState
	m := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		calls++
		var parts <-chan provider.StreamPart
		if calls == 1 {
			parts = toolCallStreamParts("lookup", `{}`)
		} else {
			parts = textStreamParts("done")
		}
		return &provider.StreamResult{
			Stream:   parts,
			Request:  &provider.RequestMetadata{Body: json.RawMessage(`{"step":` + strconv.Itoa(calls) + `}`)},
			Response: &provider.ResponseHeaders{Headers: map[string]string{"x-step": strconv.Itoa(calls)}},
		}, nil
	}}
	result := StreamText(t.Context(), m,
		WithModelMessages(provider.UserText("hello")),
		WithTools(ToolSet{"lookup": {
			InputSchema: testMustSchema(t, `{"type":"object"}`),
			Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				return json.RawMessage(`{"result":true}`), nil
			},
		}}),
		WithStopWhen(StepCountIs(2)),
		OnStepFinish(func(state OnStepFinishState) { callbacks = append(callbacks, state) }),
	)
	for range result.FullStream() {
	}
	require.NoError(t, result.Err())
	steps := result.Steps()
	require.Len(t, steps, 2)
	require.Len(t, callbacks, 2)
	for i, step := range steps {
		want := strconv.Itoa(i + 1)
		assert.JSONEq(t, `{"step":`+want+`}`, string(step.Request.Body))
		assert.Equal(t, map[string]string{"x-step": want}, step.Response.Headers)
		assert.Equal(t, step.Request, callbacks[i].Request)
		assert.Equal(t, step.Response.Headers, callbacks[i].Response.Headers)
	}
	assert.Equal(t, steps[1].Response.Headers, result.Response().Headers)
}

func TestGenerateTextAndAgent_TransportMetadata(t *testing.T) {
	m := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{
			Stream:   textStreamParts("hello"),
			Request:  &provider.RequestMetadata{Body: json.RawMessage(`{"prompt":"hello"}`)},
			Response: &provider.ResponseHeaders{Headers: map[string]string{"x-transport": "present"}},
		}, nil
	}}
	generated, err := GenerateText(t.Context(), m, WithModelMessages(provider.UserText("hi")))
	require.NoError(t, err)
	require.Len(t, generated.Steps, 1)
	assert.Equal(t, "present", generated.Response.Headers["x-transport"])
	assert.JSONEq(t, `{"prompt":"hello"}`, string(generated.Steps[0].Request.Body))

	agent := NewToolLoopAgent(m)
	agentResult, err := agent.Generate(t.Context(), WithAgentPrompt("hi"))
	require.NoError(t, err)
	assert.Equal(t, "present", agentResult.Response.Headers["x-transport"])
	require.Len(t, agentResult.Steps, 1)
	assert.JSONEq(t, `{"prompt":"hello"}`, string(agentResult.Steps[0].Request.Body))
}
