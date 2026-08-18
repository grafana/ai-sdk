package bedrock

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// encodeFixture rewrites a fixture-style JSON line (e.g. `{"messageStart":{...}}`)
// as an AWS event-stream binary frame using the outer key as :event-type.
// Mirrors the encoding the conformance replay server will use.
func encodeFixture(t *testing.T, line string) []byte {
	t.Helper()
	var wrapper map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(line), &wrapper))
	require.Len(t, wrapper, 1, "fixture line must have exactly one outer key")
	for eventType, payload := range wrapper {
		hdr := frameHeader{
			MessageType: "event",
			EventType:   eventType,
			ContentType: "application/json",
		}
		return encodeFrame(hdr, payload)
	}
	t.Fatal("unreachable")
	return nil
}

func encodeFixtures(t *testing.T, lines ...string) []byte {
	var buf bytes.Buffer
	for _, l := range lines {
		buf.Write(encodeFixture(t, l))
	}
	return buf.Bytes()
}

// drainStream runs the model's runStream against the given fixture body and
// returns the collected stream parts.
func drainStream(t *testing.T, body []byte, meta requestMeta) []provider.StreamPart {
	t.Helper()
	return drainStreamRaw(t, body, meta, false)
}

// drainStreamRaw is drainStream with explicit control over includeRaw.
func drainStreamRaw(t *testing.T, body []byte, meta requestMeta, includeRaw bool) []provider.StreamPart {
	t.Helper()
	return drainStreamFull(t, body, meta, nil, includeRaw)
}

// drainStreamRawWithHeaders is drainStream with explicit response headers.
func drainStreamRawWithHeaders(t *testing.T, body []byte, meta requestMeta, headers map[string][]string) []provider.StreamPart {
	t.Helper()
	return drainStreamFull(t, body, meta, headers, false)
}

// drainStreamFull runs runStream with full control over response headers and
// the includeRaw flag.
func drainStreamFull(t *testing.T, body []byte, meta requestMeta, headers map[string][]string, includeRaw bool) []provider.StreamPart {
	t.Helper()
	m := New("anthropic.claude-sonnet-4-5-20250929-v1:0").(*model)
	out := make(chan provider.StreamPart, streamBufferSize)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(out)
		m.runStream(ctx, bytes.NewReader(body), headers, meta, nil, includeRaw, out)
	}()

	var parts []provider.StreamPart
	for p := range out {
		parts = append(parts, p)
	}
	<-done
	return parts
}

// findParts returns the indices of parts matching the given type.
func findParts(parts []provider.StreamPart, kind provider.StreamPartType) []int {
	var out []int
	for i, p := range parts {
		if p.Type == kind {
			out = append(out, i)
		}
	}
	return out
}

func TestRunStream_SimpleText(t *testing.T) {
	body := encodeFixtures(t,
		`{"messageStart":{"role":"assistant"}}`,
		`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"hello"}}}`,
		`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":" world"}}}`,
		`{"contentBlockStop":{"contentBlockIndex":0}}`,
		`{"messageStop":{"stopReason":"end_turn"}}`,
		`{"metadata":{"usage":{"inputTokens":10,"outputTokens":5}}}`,
	)
	parts := drainStream(t, body, requestMeta{})

	// Expected sequence (high-level):
	//   stream-start, response-metadata, text-start, text-delta, text-delta,
	//   text-end, finish.
	require.NotEmpty(t, parts)
	assert.Equal(t, provider.PartStreamStart, parts[0].Type)
	textStarts := findParts(parts, provider.PartTextStart)
	textDeltas := findParts(parts, provider.PartTextDelta)
	textEnds := findParts(parts, provider.PartTextEnd)
	require.Len(t, textStarts, 1)
	require.Len(t, textDeltas, 2)
	require.Len(t, textEnds, 1)
	assert.Equal(t, "hello", parts[textDeltas[0]].Delta)
	assert.Equal(t, " world", parts[textDeltas[1]].Delta)

	// Finish part must be the last one.
	last := parts[len(parts)-1]
	require.Equal(t, provider.PartFinish, last.Type)
	require.NotNil(t, last.FinishReason)
	assert.Equal(t, provider.FinishReasonStop, last.FinishReason.Unified)
	require.NotNil(t, last.Usage)
	assert.Equal(t, 10, *last.Usage.InputTokens.NoCache)
	assert.Equal(t, 5, *last.Usage.OutputTokens.Total)
}

func TestRunStream_JSONInstructionExtractsObjectAcrossDeltas(t *testing.T) {
	body := encodeFixtures(t,
		`{"messageStart":{"role":"assistant"}}`,
		`{"contentBlockStart":{"contentBlockIndex":0,"start":{}}}`,
		`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"Here is the result: "}}}`,
		`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"{\"status\":{\"value\":\"ok }"}}}`,
		`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":" still string\"}} trailing text"}}}`,
		`{"contentBlockStop":{"contentBlockIndex":0}}`,
		`{"messageStop":{"stopReason":"end_turn"}}`,
	)
	parts := drainStream(t, body, requestMeta{usesJSONInstruction: true})
	textDeltas := findParts(parts, provider.PartTextDelta)
	require.Len(t, textDeltas, 2)
	assert.Equal(t, `{"status":{"value":"ok }`, parts[textDeltas[0]].Delta)
	assert.Equal(t, ` still string"}}`, parts[textDeltas[1]].Delta)
}

func TestRunStream_ToolCall(t *testing.T) {
	body := encodeFixtures(t,
		`{"messageStart":{"role":"assistant"}}`,
		`{"contentBlockStart":{"contentBlockIndex":0,"start":{"toolUse":{"toolUseId":"call-1","name":"weather"}}}}`,
		`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"toolUse":{"input":"{\"city\":\"Berlin\"}"}}}}`,
		`{"contentBlockStop":{"contentBlockIndex":0}}`,
		`{"messageStop":{"stopReason":"tool_use"}}`,
		`{"metadata":{"usage":{"inputTokens":10,"outputTokens":4}}}`,
	)
	parts := drainStream(t, body, requestMeta{})

	// Find tool input lifecycle parts.
	require.NotEmpty(t, findParts(parts, provider.PartToolInputStart))
	require.NotEmpty(t, findParts(parts, provider.PartToolInputDelta))
	require.NotEmpty(t, findParts(parts, provider.PartToolInputEnd))

	// One tool-call event with accumulated input.
	tcIdx := findParts(parts, provider.PartToolCall)
	require.Len(t, tcIdx, 1)
	tc := parts[tcIdx[0]]
	assert.Equal(t, "call-1", tc.ToolCallID)
	assert.Equal(t, "weather", tc.ToolName)
	assert.JSONEq(t, `{"city":"Berlin"}`, tc.Input)

	last := parts[len(parts)-1]
	require.Equal(t, provider.PartFinish, last.Type)
	assert.Equal(t, provider.FinishReasonToolCalls, last.FinishReason.Unified)
}

func TestRunStream_ReasoningWithSignature(t *testing.T) {
	body := encodeFixtures(t,
		`{"messageStart":{"role":"assistant"}}`,
		`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"reasoningContent":{"text":"thinking..."}}}}`,
		`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"reasoningContent":{"signature":"sig-xyz"}}}}`,
		`{"contentBlockStop":{"contentBlockIndex":0}}`,
		`{"contentBlockDelta":{"contentBlockIndex":1,"delta":{"text":"answer"}}}`,
		`{"contentBlockStop":{"contentBlockIndex":1}}`,
		`{"messageStop":{"stopReason":"end_turn"}}`,
		`{"metadata":{"usage":{"inputTokens":10,"outputTokens":5}}}`,
	)
	parts := drainStream(t, body, requestMeta{})
	require.NotEmpty(t, findParts(parts, provider.PartReasoningStart))
	require.NotEmpty(t, findParts(parts, provider.PartReasoningDelta))
	require.NotEmpty(t, findParts(parts, provider.PartReasoningEnd))

	// At least one reasoning-delta carries the signature metadata.
	sigFound := false
	for _, p := range parts {
		if p.Type == provider.PartReasoningDelta && p.ProviderMetadata != nil {
			if raw, ok := p.ProviderMetadata["amazonBedrock"]; ok {
				var meta ReasoningMetadata
				_ = json.Unmarshal(raw, &meta)
				if meta.Signature == "sig-xyz" {
					sigFound = true
				}
			}
		}
	}
	assert.True(t, sigFound, "reasoning signature metadata expected")
}

func TestRunStream_JSONResponseToolCollapse(t *testing.T) {
	body := encodeFixtures(t,
		`{"messageStart":{"role":"assistant"}}`,
		`{"contentBlockStart":{"contentBlockIndex":0,"start":{"toolUse":{"toolUseId":"call-1","name":"json"}}}}`,
		`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"toolUse":{"input":"{\"foo\":\"bar\"}"}}}}`,
		`{"contentBlockStop":{"contentBlockIndex":0}}`,
		`{"messageStop":{"stopReason":"tool_use"}}`,
		`{"metadata":{"usage":{"inputTokens":10,"outputTokens":4}}}`,
	)
	parts := drainStream(t, body, requestMeta{usesJSONResponseTool: true})
	// No tool-call events.
	assert.Empty(t, findParts(parts, provider.PartToolCall), "json response tool must not emit tool-call")
	// One text-delta carrying the JSON.
	textDeltas := findParts(parts, provider.PartTextDelta)
	require.NotEmpty(t, textDeltas)
	assert.JSONEq(t, `{"foo":"bar"}`, parts[textDeltas[0]].Delta)
	// Finish reason flipped to stop.
	last := parts[len(parts)-1]
	require.Equal(t, provider.PartFinish, last.Type)
	assert.Equal(t, provider.FinishReasonStop, last.FinishReason.Unified)
}

func TestRunStream_FinishIncludesUnavailableUsage(t *testing.T) {
	body := encodeFixtures(t,
		`{"messageStart":{"role":"assistant"}}`,
		`{"messageStop":{"stopReason":"end_turn"}}`,
	)
	parts := drainStream(t, body, requestMeta{})

	finish := parts[len(parts)-1]
	require.Equal(t, provider.PartFinish, finish.Type)
	require.NotNil(t, finish.Usage)
	assert.Nil(t, finish.Usage.InputTokens.Total)
	assert.Nil(t, finish.Usage.OutputTokens.Total)
}

func TestRunStream_ThrottlingExceptionEvent(t *testing.T) {
	// Build a payload mixing some text frames and then an exception frame.
	textFrame := encodeFixture(t, `{"messageStart":{"role":"assistant"}}`)
	excFrame := encodeFrame(frameHeader{
		MessageType:   "exception",
		ExceptionType: "throttlingException",
		ContentType:   "application/json",
	}, []byte(`{"message":"rate limited"}`))

	body := append([]byte{}, textFrame...)
	body = append(body, excFrame...)
	parts := drainStream(t, body, requestMeta{})

	errIdx := findParts(parts, provider.PartError)
	require.Len(t, errIdx, 1)
	errPart := parts[errIdx[0]]
	require.NotNil(t, errPart.APICallError)
	assert.True(t, errPart.APICallError.IsRetryable)
	assert.Contains(t, errPart.APICallError.Message, "rate limited")

	// A terminal finish event is still emitted (matching upstream), carrying
	// the error finish reason.
	last := parts[len(parts)-1]
	require.Equal(t, provider.PartFinish, last.Type)
	require.NotNil(t, last.FinishReason)
	assert.Equal(t, provider.FinishReasonError, last.FinishReason.Unified)
	require.NotNil(t, last.Usage)
	assert.Nil(t, last.Usage.InputTokens.Total)
	assert.Nil(t, last.Usage.OutputTokens.Total)
}

func TestRunStream_ValidationExceptionNotRetryable(t *testing.T) {
	excFrame := encodeFrame(frameHeader{
		MessageType:   "exception",
		ExceptionType: "validationException",
		ContentType:   "application/json",
	}, []byte(`{"message":"bad"}`))
	parts := drainStream(t, excFrame, requestMeta{})
	errIdx := findParts(parts, provider.PartError)
	require.Len(t, errIdx, 1)
	assert.False(t, parts[errIdx[0]].APICallError.IsRetryable)

	last := parts[len(parts)-1]
	require.Equal(t, provider.PartFinish, last.Type)
	require.NotNil(t, last.FinishReason)
	assert.Equal(t, provider.FinishReasonError, last.FinishReason.Unified)
}

func TestRunStream_MidStreamTransportFailureEmitsRetryable(t *testing.T) {
	// Provide truncated bytes: prelude says totalLen=N but body is shorter.
	hdr := frameHeader{MessageType: "event", EventType: "contentBlockDelta"}
	full := encodeFrame(hdr, []byte(`{"contentBlockIndex":0,"delta":{"text":"hi"}}`))
	parts := drainStream(t, full[:len(full)-5], requestMeta{})

	errIdx := findParts(parts, provider.PartError)
	require.Len(t, errIdx, 1)
	assert.True(t, parts[errIdx[0]].APICallError.IsRetryable)
}

func TestRunStream_ContextCancellation(t *testing.T) {
	body := encodeFixture(t, `{"messageStart":{"role":"assistant"}}`)
	m := New("anthropic.claude-sonnet-4-5-20250929-v1:0").(*model)
	out := make(chan provider.StreamPart, streamBufferSize)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(out)
		m.runStream(ctx, bytes.NewReader(body), nil, requestMeta{}, nil, false, out)
	}()
	// Drain channel.
	for range out {
	}
	<-done
	// No panics, exited cleanly.
}

func TestRunStream_ChannelBufferSize(t *testing.T) {
	assert.GreaterOrEqual(t, streamBufferSize, 64)
}

func TestRunStream_IncludeRawChunksEmitsRawPerFrame(t *testing.T) {
	lines := []string{
		`{"messageStart":{"role":"assistant"}}`,
		`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"hi"}}}`,
		`{"contentBlockStop":{"contentBlockIndex":0}}`,
		`{"messageStop":{"stopReason":"end_turn"}}`,
		`{"metadata":{"usage":{"inputTokens":10,"outputTokens":5}}}`,
	}
	body := encodeFixtures(t, lines...)

	// Without includeRaw: no raw parts for known events.
	plain := drainStreamRaw(t, body, requestMeta{}, false)
	assert.Empty(t, findParts(plain, provider.PartRaw), "no raw parts when includeRaw is false")

	// With includeRaw: one raw part per decoded frame.
	withRaw := drainStreamRaw(t, body, requestMeta{}, true)
	rawIdx := findParts(withRaw, provider.PartRaw)
	require.Len(t, rawIdx, len(lines), "expected one raw part per frame")
	// Raw payloads carry the decoded frame body (the event payload, with the
	// :event-type discriminator carried in the frame header).
	assert.JSONEq(t, `{"role":"assistant"}`, string(withRaw[rawIdx[0]].RawValue))
	// Normal lifecycle parts are still emitted alongside raw parts.
	require.NotEmpty(t, findParts(withRaw, provider.PartTextDelta))
	last := withRaw[len(withRaw)-1]
	require.Equal(t, provider.PartFinish, last.Type)
}

func TestRunStream_IncludeRawChunksOnException(t *testing.T) {
	textFrame := encodeFixture(t, `{"messageStart":{"role":"assistant"}}`)
	excFrame := encodeFrame(frameHeader{
		MessageType:   "exception",
		ExceptionType: "throttlingException",
		ContentType:   "application/json",
	}, []byte(`{"message":"rate limited"}`))
	body := append(append([]byte{}, textFrame...), excFrame...)

	parts := drainStreamRaw(t, body, requestMeta{}, true)
	// Two frames -> two raw parts (messageStart + exception).
	require.Len(t, findParts(parts, provider.PartRaw), 2)
	require.Len(t, findParts(parts, provider.PartError), 1)
}

func TestRunStream_StopSequenceInFinishMetadata(t *testing.T) {
	body := encodeFixtures(t,
		`{"messageStart":{"role":"assistant"}}`,
		`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"hi"}}}`,
		`{"contentBlockStop":{"contentBlockIndex":0}}`,
		`{"messageStop":{"additionalModelResponseFields":{"delta":{"stop_sequence":"END"}},"stopReason":"stop_sequence"}}`,
		`{"metadata":{"usage":{"inputTokens":10,"outputTokens":5}}}`,
	)
	parts := drainStream(t, body, requestMeta{})
	last := parts[len(parts)-1]
	require.Equal(t, provider.PartFinish, last.Type)
	require.NotNil(t, last.ProviderMetadata)
	raw, ok := last.ProviderMetadata["amazonBedrock"]
	require.True(t, ok)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	assert.Equal(t, "END", payload["stopSequence"])
}
