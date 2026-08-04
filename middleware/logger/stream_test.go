package logger

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
)

func TestMiddleware_StreamSuccessTeesUnmodifiedParts(t *testing.T) {
	handler := newTestHandler()
	clock := newStepClock(25 * time.Millisecond)
	inputTokens := 5
	outputTokens := 9
	reasoningTokens := 2
	finishReason := provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "end_turn"}
	parts := []provider.StreamPart{
		{Type: provider.PartResponseMeta, ResponseID: "resp-1", Provider: "test-provider", ModelID: "served-model", Timestamp: time.Unix(10, 0)},
		{Type: provider.PartTextDelta, ID: "txt", Delta: "hello"},
		{Type: provider.PartFinish, Usage: &provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: &inputTokens},
			OutputTokens: provider.OutputTokenUsage{Total: &outputTokens, Reasoning: &reasoningTokens},
		}, FinishReason: &finishReason},
	}
	request := &provider.RequestMetadata{Body: []byte(`{"request":true}`)}
	response := &provider.ResponseHeaders{Headers: map[string]string{"x-safe": "ok"}}
	model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
		if opts.IncludeRawChunks {
			t.Fatal("logger forced raw chunks")
		}
		ch := make(chan provider.StreamPart, len(parts))
		for _, part := range parts {
			ch <- part
		}
		close(ch)
		return &provider.StreamResult{Stream: ch, Request: request, Response: response}, nil
	}}
	wrapped := Wrap(model, Options{Logger: slog.New(handler), Clock: clock.Now})

	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	if err != nil {
		t.Fatalf("DoStream returned error: %v", err)
	}
	if result.Request != request || result.Response != response {
		t.Fatalf("stream metadata not preserved")
	}
	got := drainStream(result.Stream)
	if !reflect.DeepEqual(got, parts) {
		t.Fatalf("stream parts changed: got %#v want %#v", got, parts)
	}
	if model.streamCalls != 1 {
		t.Fatalf("expected one stream call, got %d", model.streamCalls)
	}

	records := handler.Records()
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d: %#v", len(records), records)
	}
	if records[0].Message != string(EventStreamStart) || records[1].Message != string(EventStreamFinish) {
		t.Fatalf("unexpected records: %#v", records)
	}
	attrs := records[1].AttrsMap()
	assertAttr(t, attrs, "ai_sdk.success", true)
	assertAttr(t, attrs, "ai_sdk.outcome", outcomeSuccess)
	assertAttr(t, attrs, "ai_sdk.duration_ms", float64(100))
	assertAttr(t, attrs, "ai_sdk.stream.time_to_first_content_ms", float64(50))
	assertAttr(t, attrs, "ai_sdk.stream.parts.count", int64(3))
	assertAttr(t, attrs, "ai_sdk.stream.parts.text_delta.count", int64(1))
	assertAttr(t, attrs, "ai_sdk.usage.input_tokens.total", int64(inputTokens))
	assertAttr(t, attrs, "gen_ai.usage.input_tokens", int64(inputTokens))
	assertAttr(t, attrs, "ai_sdk.usage.output_tokens.total", int64(outputTokens))
	assertAttr(t, attrs, "gen_ai.usage.output_tokens", int64(outputTokens))
	assertAttr(t, attrs, "ai_sdk.usage.output_tokens.reasoning", int64(reasoningTokens))
	assertAttr(t, attrs, "ai_sdk.finish_reason", string(provider.FinishReasonStop))
	assertAttr(t, attrs, "ai_sdk.response.id", "resp-1")
	assertAttr(t, attrs, "ai_sdk.provider", "test-provider")
	assertAttr(t, attrs, "ai_sdk.model", "served-model")
	assertAttr(t, attrs, "ai_sdk.transport.provider", "test")
	assertAttr(t, attrs, "ai_sdk.transport.model", "model")
	assertAttr(t, attrs, "gen_ai.system", "test-provider")
	assertAttr(t, attrs, "gen_ai.response.model", "served-model")
	if records[0].AttrsMap()["ai_sdk.call.id"] == "" || attrs["ai_sdk.call.id"] != records[0].AttrsMap()["ai_sdk.call.id"] {
		t.Fatalf("start/finish call id mismatch: start=%#v finish=%#v", records[0].AttrsMap()["ai_sdk.call.id"], attrs["ai_sdk.call.id"])
	}
	if _, ok := attrs["ai_sdk.response.headers"]; ok {
		t.Fatalf("default logging captured headers: %#v", attrs["ai_sdk.response.headers"])
	}
}

func TestMiddleware_StreamUsageAggregatesEveryPart(t *testing.T) {
	inputTotal, inputNoCache, cacheRead, cacheWrite := 120, 80, 30, 10
	outputTotal, outputText, outputReasoning := 50, 30, 20
	provisionalInput, provisionalCacheRead, provisionalCacheWrite := 100, 20, 5
	provisionalOutput, provisionalText, provisionalReasoning := 45, 25, 15
	parts := []provider.StreamPart{
		{Type: provider.PartResponseMeta, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{
			Total: &inputTotal, NoCache: &inputNoCache, CacheRead: &cacheRead, CacheWrite: &cacheWrite,
		}}},
		{Type: provider.PartTextDelta, Delta: "hello", Usage: &provider.Usage{OutputTokens: provider.OutputTokenUsage{
			Total: &outputTotal, Text: &outputText, Reasoning: &outputReasoning,
		}}},
		{Type: provider.PartFinish, Usage: &provider.Usage{
			InputTokens: provider.InputTokenUsage{
				Total: &provisionalInput, CacheRead: &provisionalCacheRead, CacheWrite: &provisionalCacheWrite,
			},
			OutputTokens: provider.OutputTokenUsage{
				Total: &provisionalOutput, Text: &provisionalText, Reasoning: &provisionalReasoning,
			},
		}},
	}
	handler := newTestHandler()
	wrapped := Wrap(streamModel(parts), Options{Logger: slog.New(handler)})

	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	if err != nil {
		t.Fatalf("DoStream returned error: %v", err)
	}
	drainStream(result.Stream)

	records := handler.Records()
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	attrs := records[1].AttrsMap()
	assertAttr(t, attrs, "ai_sdk.usage.input_tokens.total", int64(inputTotal))
	assertAttr(t, attrs, "ai_sdk.usage.input_tokens.no_cache", int64(inputNoCache))
	assertAttr(t, attrs, "ai_sdk.usage.input_tokens.cache_read", int64(cacheRead))
	assertAttr(t, attrs, "ai_sdk.usage.input_tokens.cache_write", int64(cacheWrite))
	assertAttr(t, attrs, "ai_sdk.usage.output_tokens.total", int64(outputTotal))
	assertAttr(t, attrs, "ai_sdk.usage.output_tokens.text", int64(outputText))
	assertAttr(t, attrs, "ai_sdk.usage.output_tokens.reasoning", int64(outputReasoning))
}

func TestMiddleware_StreamOpenErrorLogsAndPropagates(t *testing.T) {
	handler := newTestHandler()
	clock := newStepClock(10 * time.Millisecond)
	sentinel := errors.New("stream open failed")
	model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return nil, sentinel
	}}
	wrapped := Wrap(model, Options{Logger: slog.New(handler), Clock: clock.Now})

	_, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	records := handler.Records()
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[1].Message != string(EventStreamError) {
		t.Fatalf("expected stream error record, got %s", records[1].Message)
	}
	attrs := records[1].AttrsMap()
	assertAttr(t, attrs, "ai_sdk.success", false)
	assertAttr(t, attrs, "ai_sdk.outcome", outcomeError)
	assertAttr(t, attrs, "ai_sdk.error.type", "unknown")
	if _, ok := attrs["ai_sdk.error.message"]; ok {
		t.Fatalf("default logging captured opaque stream-open error message: %#v", attrs)
	}
	if encoded := handler.JSON(t); strings.Contains(encoded, "stream open failed") {
		t.Fatalf("default records leaked opaque stream-open error message: %s", encoded)
	}
}

func TestMiddleware_StreamPartErrorLogsTerminalError(t *testing.T) {
	handler := newTestHandler()
	messageSecret := "STREAM-PART-ERROR-MESSAGE-SECRET"
	apiErr := provider.NewAPICallError(provider.APICallErrorOptions{Message: messageSecret, StatusCode: 429})
	parts := []provider.StreamPart{
		{Type: provider.PartTextDelta, Delta: "before"},
		{Type: provider.PartError, APICallError: apiErr},
	}
	model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		ch := make(chan provider.StreamPart, len(parts))
		for _, part := range parts {
			ch <- part
		}
		close(ch)
		return &provider.StreamResult{Stream: ch}, nil
	}}
	wrapped := Wrap(model, Options{Logger: slog.New(handler)})

	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	if err != nil {
		t.Fatalf("DoStream returned error: %v", err)
	}
	got := drainStream(result.Stream)
	if !reflect.DeepEqual(got, parts) {
		t.Fatalf("stream parts changed: got %#v want %#v", got, parts)
	}
	records := handler.Records()
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[1].Message != string(EventStreamError) {
		t.Fatalf("expected stream error terminal record, got %s", records[1].Message)
	}
	attrs := records[1].AttrsMap()
	assertAttr(t, attrs, "ai_sdk.success", false)
	assertAttr(t, attrs, "ai_sdk.outcome", outcomeError)
	assertAttr(t, attrs, "ai_sdk.error.type", "api_call_error")
	assertAttr(t, attrs, "ai_sdk.stream.parts.error.count", int64(1))
	assertAttr(t, attrs, "ai_sdk.error.status_code", int64(429))
	assertAttr(t, attrs, "ai_sdk.error.retryable", true)
	if _, ok := attrs["ai_sdk.error.message"]; ok {
		t.Fatalf("default logging captured opaque stream-part error message: %#v", attrs)
	}
	if encoded := handler.JSON(t); strings.Contains(encoded, messageSecret) {
		t.Fatalf("default records leaked opaque stream-part error message: %s", encoded)
	}
}

func TestMiddleware_StreamPartErrorMessageCaptureIsOptIn(t *testing.T) {
	messageSecret := "STREAM-PART-ERROR-MESSAGE-SECRET"
	apiErr := provider.NewAPICallError(provider.APICallErrorOptions{Message: messageSecret, StatusCode: 500})
	handler := newTestHandler()
	wrapped := Wrap(streamModel([]provider.StreamPart{{Type: provider.PartError, APICallError: apiErr}}), Options{
		Logger:  slog.New(handler),
		Capture: CaptureOptions{ErrorMessages: true},
	})

	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	if err != nil {
		t.Fatalf("DoStream returned error: %v", err)
	}
	drainStream(result.Stream)

	attrs := handler.Records()[1].AttrsMap()
	message, ok := attrs["ai_sdk.error.message"].(string)
	if !ok || !strings.Contains(message, messageSecret) {
		t.Fatalf("missing opted-in stream-part error message: %#v", attrs)
	}
}

func TestMiddleware_StreamPartLoggingClassifiesNilError(t *testing.T) {
	handler := newTestHandler()
	wrapped := Wrap(streamModel([]provider.StreamPart{{Type: provider.PartError}}), Options{
		Logger:         slog.New(handler),
		LogStreamParts: true,
	})

	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	if err != nil {
		t.Fatalf("DoStream returned error: %v", err)
	}
	drainStream(result.Stream)

	records := handler.Records()
	if len(records) != 3 {
		t.Fatalf("expected start, part, and terminal records, got %d", len(records))
	}
	for _, record := range records[1:] {
		attrs := record.AttrsMap()
		assertAttr(t, attrs, "ai_sdk.error.type", "stream_part_error")
		if _, ok := attrs["ai_sdk.error.message"]; ok {
			t.Fatalf("default logging captured stream-part error message: %#v", attrs)
		}
	}
}

func TestMiddleware_StreamContextCancellationClosesIdleStream(t *testing.T) {
	handler := newTestHandler()
	upstream := make(chan provider.StreamPart)
	model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: upstream}, nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	wrapped := Wrap(model, Options{Logger: slog.New(handler)})

	result, err := wrapped.DoStream(ctx, provider.CallOptions{})
	if err != nil {
		t.Fatalf("DoStream returned error: %v", err)
	}
	cancel()
	select {
	case _, ok := <-result.Stream:
		if ok {
			t.Fatal("expected stream to close after cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cancelled stream to close")
	}
	records := waitForRecords(t, handler, 2)
	if records[1].Message != string(EventStreamCancelled) {
		t.Fatalf("expected cancellation terminal record, got %s", records[1].Message)
	}
	attrs := records[1].AttrsMap()
	assertAttr(t, attrs, "ai_sdk.outcome", outcomeCancelled)
	assertAttr(t, attrs, "ai_sdk.error.type", "context_canceled")
	if _, ok := attrs["ai_sdk.error.message"]; ok {
		t.Fatalf("default logging captured context cancellation message: %#v", attrs)
	}
}

func TestMiddleware_StreamContextDeadlineOmitsMessage(t *testing.T) {
	handler := newTestHandler()
	upstream := make(chan provider.StreamPart)
	model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: upstream}, nil
	}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	wrapped := Wrap(model, Options{Logger: slog.New(handler)})

	result, err := wrapped.DoStream(ctx, provider.CallOptions{})
	if err != nil {
		t.Fatalf("DoStream returned error: %v", err)
	}
	select {
	case _, ok := <-result.Stream:
		if ok {
			t.Fatal("expected stream to close after deadline")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for deadline stream to close")
	}

	records := waitForRecords(t, handler, 2)
	if records[1].Message != string(EventStreamError) {
		t.Fatalf("expected stream error terminal record, got %s", records[1].Message)
	}
	attrs := records[1].AttrsMap()
	assertAttr(t, attrs, "ai_sdk.outcome", outcomeTimeout)
	assertAttr(t, attrs, "ai_sdk.error.type", "context_deadline_exceeded")
	if _, ok := attrs["ai_sdk.error.message"]; ok {
		t.Fatalf("default logging captured context deadline message: %#v", attrs)
	}
}

func TestMiddleware_StreamContextCancellationDoesNotLeak(t *testing.T) {
	handler := newTestHandler()
	upstream := make(chan provider.StreamPart, streamBuffer+1)
	for i := 0; i < streamBuffer+1; i++ {
		upstream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "x"}
	}
	close(upstream)
	model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: upstream}, nil
	}}
	ctx, cancel := context.WithCancel(context.Background())
	wrapped := Wrap(model, Options{Logger: slog.New(handler)})

	result, err := wrapped.DoStream(ctx, provider.CallOptions{})
	if err != nil {
		t.Fatalf("DoStream returned error: %v", err)
	}
	cancel()
	records := waitForRecords(t, handler, 2)
	if records[1].Message != string(EventStreamCancelled) {
		t.Fatalf("expected cancellation terminal record, got %s", records[1].Message)
	}
	assertAttr(t, records[1].AttrsMap(), "ai_sdk.outcome", outcomeCancelled)
	for range result.Stream {
	}
}

func TestMiddleware_StreamPartLoggingWithoutCaptureDoesNotLogPayloadFields(t *testing.T) {
	secret := "STREAM-SECRET-VALUE"
	parts := []provider.StreamPart{{
		Type:      provider.PartToolResult,
		Delta:     secret,
		Input:     secret,
		ToolName:  secret,
		Result:    json.RawMessage(`"STREAM-SECRET-VALUE"`),
		Data:      &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: []byte(secret)},
		RawValue:  []byte(`{"secret":"STREAM-SECRET-VALUE"}`),
		MediaType: secret,
		Filename:  secret,
		Title:     secret,
		Reason:    secret,
		Source:    &provider.SourceInfo{Title: secret, URL: secret},
		APICallError: provider.NewAPICallError(provider.APICallErrorOptions{
			Message: secret,
		}),
		ProviderMetadata: provider.ProviderMetadata{"test": []byte(`{"secret":"STREAM-SECRET-VALUE"}`)},
	}}
	handler := newTestHandler()
	wrapped := Wrap(streamModel(parts), Options{Logger: slog.New(handler), LogStreamParts: true})
	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	if err != nil {
		t.Fatalf("DoStream returned error: %v", err)
	}
	drainStream(result.Stream)

	for _, record := range handler.Records() {
		if record.Message != string(EventStreamPart) {
			continue
		}
		encoded := handler.JSON(t)
		if strings.Contains(encoded, secret) {
			t.Fatalf("part logging without capture leaked payload secret: %s", encoded)
		}
		if _, ok := record.AttrsMap()["ai_sdk.stream.part"]; ok {
			t.Fatalf("unexpected full stream part capture without capture flags: %#v", record.AttrsMap())
		}
	}
}

func TestMiddleware_StreamPartLoggingWithProviderMetadataDoesNotLeakAPICallError(t *testing.T) {
	secret := "STREAM-API-SECRET"
	parts := []provider.StreamPart{{
		Type: provider.PartError,
		APICallError: provider.NewAPICallError(provider.APICallErrorOptions{
			Message:           secret,
			StatusCode:        500,
			URL:               "https://example.test/v1?access_token=" + secret,
			RequestBodyValues: json.RawMessage(`{"secret":"STREAM-API-SECRET"}`),
			ResponseHeaders:   map[string][]string{"x-api-key": {secret}},
			ResponseBody:      `{"secret":"STREAM-API-SECRET"}`,
		}),
		ProviderMetadata: provider.ProviderMetadata{"test": []byte(`{"safe":"visible"}`)},
	}}
	handler := newTestHandler()
	wrapped := Wrap(streamModel(parts), Options{
		Logger:         slog.New(handler),
		LogStreamParts: true,
		Capture:        CaptureOptions{ProviderMetadata: true},
	})

	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	if err != nil {
		t.Fatalf("DoStream returned error: %v", err)
	}
	drainStream(result.Stream)

	encoded := handler.JSON(t)
	if strings.Contains(encoded, secret) || strings.Contains(encoded, "apiCallError") || strings.Contains(encoded, "responseBody") {
		t.Fatalf("provider metadata part logging leaked gated API error fields: %s", encoded)
	}
	if !strings.Contains(encoded, "visible") {
		t.Fatalf("expected provider metadata capture in records: %s", encoded)
	}
}

func TestMiddleware_StreamPartProviderMetadataCaptureDoesNotLogPayload(t *testing.T) {
	secret := "STREAM-PAYLOAD-SECRET"
	parts := []provider.StreamPart{
		{
			Type:             provider.PartTextDelta,
			ID:               secret,
			Delta:            secret,
			ProviderMetadata: provider.ProviderMetadata{"text": []byte(`{"safe":"visible-text"}`)},
		},
		{
			Type:             provider.PartFile,
			Data:             &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: "https://example.test/" + secret},
			Filename:         secret,
			MediaType:        secret,
			Title:            secret,
			ProviderMetadata: provider.ProviderMetadata{"file": []byte(`{"safe":"visible-file"}`)},
		},
		{
			Type:       provider.PartToolResult,
			ToolCallID: secret,
			ToolName:   secret,
			Result:     json.RawMessage(`"STREAM-PAYLOAD-SECRET"`),
			Reason:     secret,
			Source:     &provider.SourceInfo{URL: secret, Title: secret},
			ProviderMetadata: provider.ProviderMetadata{
				"tool": []byte(`{"safe":"visible-tool"}`),
			},
		},
		{
			Type:            provider.PartCustom,
			ID:              secret,
			Delta:           secret,
			Input:           secret,
			Title:           secret,
			Kind:            secret,
			Reason:          secret,
			Data:            &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Bytes: []byte(secret)},
			Filename:        secret,
			RawValue:        json.RawMessage(`{"secret":"STREAM-PAYLOAD-SECRET"}`),
			ResponseHeaders: map[string]string{"x-api-key": secret},
			Source:          &provider.SourceInfo{URL: secret, Title: secret},
			ProviderMetadata: provider.ProviderMetadata{
				"custom": []byte(`{"safe":"visible-custom"}`),
			},
		},
	}
	handler := newTestHandler()
	wrapped := Wrap(streamModel(parts), Options{
		Logger:         slog.New(handler),
		LogStreamParts: true,
		Capture:        CaptureOptions{ProviderMetadata: true},
	})

	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	if err != nil {
		t.Fatalf("DoStream returned error: %v", err)
	}
	drainStream(result.Stream)

	encoded := handler.JSON(t)
	if strings.Contains(encoded, secret) {
		t.Fatalf("provider metadata part logging leaked payload: %s", encoded)
	}
	for _, want := range []string{"visible-text", "visible-file", "visible-tool", "visible-custom"} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("expected provider metadata %q in records: %s", want, encoded)
		}
	}
}

func TestMiddleware_StreamPartToolApprovalRequestUsesToolInputCapture(t *testing.T) {
	approved := false
	parts := []provider.StreamPart{{
		Type:       provider.PartToolApprovalRequest,
		ToolCallID: "tool-call-1",
		ToolName:   "lookup",
		Input:      `{"query":"safe"}`,
		ApprovalID: "approval-1",
		Approved:   &approved,
		Reason:     "needs review",
	}}
	handler := newTestHandler()
	wrapped := Wrap(streamModel(parts), Options{
		Logger:         slog.New(handler),
		LogStreamParts: true,
		Capture:        CaptureOptions{ToolInputs: true},
	})

	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	if err != nil {
		t.Fatalf("DoStream returned error: %v", err)
	}
	drainStream(result.Stream)

	encoded := handler.JSON(t)
	for _, want := range []string{"tool-call-1", "lookup", "approval-1", "needs review"} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("expected approval request field %q in records: %s", want, encoded)
		}
	}
}

func TestMiddleware_StreamPartLoggingIsOptIn(t *testing.T) {
	parts := []provider.StreamPart{{Type: provider.PartTextDelta, Delta: "visible"}}

	handlerOff := newTestHandler()
	wrappedOff := Wrap(streamModel(parts), Options{Logger: slog.New(handlerOff)})
	resultOff, err := wrappedOff.DoStream(context.Background(), provider.CallOptions{})
	if err != nil {
		t.Fatalf("DoStream returned error: %v", err)
	}
	drainStream(resultOff.Stream)
	for _, record := range handlerOff.Records() {
		if record.Message == string(EventStreamPart) {
			t.Fatalf("unexpected part record with LogStreamParts=false")
		}
	}

	handlerOn := newTestHandler()
	wrappedOn := Wrap(streamModel(parts), Options{
		Logger:         slog.New(handlerOn),
		LogStreamParts: true,
		Capture:        CaptureOptions{Outputs: true},
	})
	resultOn, err := wrappedOn.DoStream(context.Background(), provider.CallOptions{})
	if err != nil {
		t.Fatalf("DoStream returned error: %v", err)
	}
	drainStream(resultOn.Stream)
	records := handlerOn.Records()
	partRecords := 0
	for _, record := range records {
		if record.Message != string(EventStreamPart) {
			continue
		}
		partRecords++
		assertAttr(t, record.AttrsMap(), "ai_sdk.stream.text", "visible")
	}
	if partRecords != 1 {
		t.Fatalf("expected one part record, got %d in %#v", partRecords, records)
	}
}

func streamModel(parts []provider.StreamPart) *mockModel {
	return &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		ch := make(chan provider.StreamPart, len(parts))
		for _, part := range parts {
			ch <- part
		}
		close(ch)
		return &provider.StreamResult{Stream: ch}, nil
	}}
}

func drainStream(stream <-chan provider.StreamPart) []provider.StreamPart {
	var parts []provider.StreamPart
	for part := range stream {
		parts = append(parts, part)
	}
	return parts
}
