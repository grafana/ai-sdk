package logger

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

func TestAPI_Surface(t *testing.T) {
	mw := Middleware(Options{})
	if mw.WrapGenerate == nil {
		t.Fatal("expected WrapGenerate hook")
	}
	if mw.WrapStream == nil {
		t.Fatal("expected WrapStream hook")
	}
	if mw.TransformParams != nil {
		t.Fatal("logger must not implement TransformParams")
	}

	var redactor Redactor = RedactorFunc(func(_ context.Context, _ EventKind, attrs []slog.Attr) []slog.Attr { return attrs })
	if got := redactor.RedactAttrs(context.Background(), EventGenerateStart, nil); got != nil {
		t.Fatalf("expected nil attrs from redactor smoke test, got %#v", got)
	}
	if DefaultRedactorWithExtraKeys("x-secret") == nil {
		t.Fatal("expected extended default redactor")
	}

	base := &mockModel{}
	wrapped := Wrap(base, Options{})
	if wrapped == nil {
		t.Fatal("expected wrapped model")
	}
	wrappedByMiddleware := middleware.Wrap(middleware.WrapOptions{Model: base, Middleware: []middleware.Middleware{Middleware(Options{})}})
	if wrappedByMiddleware == nil {
		t.Fatal("expected middleware-wrapped model")
	}

	events := []EventKind{
		EventGenerateStart,
		EventGenerateFinish,
		EventGenerateError,
		EventStreamStart,
		EventStreamFinish,
		EventStreamError,
		EventStreamCancelled,
		EventStreamPart,
	}
	for _, event := range events {
		if !strings.HasPrefix(string(event), "aisdk.model.") {
			t.Fatalf("unexpected event %q", event)
		}
	}
}

func TestMiddleware_GenerateSuccessLogsStartAndFinish(t *testing.T) {
	handler := newTestHandler()
	clock := newStepClock(100 * time.Millisecond)
	inputTokens := 11
	inputNoCacheTokens := 6
	inputCacheReadTokens := 3
	inputCacheWriteTokens := 2
	outputTokens := 7
	outputTextTokens := 4
	reasoningTokens := 3
	result := &provider.GenerateResult{
		Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "secret output"}},
		FinishReason: provider.FinishReason{
			Unified: provider.FinishReasonStop,
			Raw:     "stop_sequence",
		},
		Usage: provider.Usage{
			InputTokens: provider.InputTokenUsage{
				Total:      &inputTokens,
				NoCache:    &inputNoCacheTokens,
				CacheRead:  &inputCacheReadTokens,
				CacheWrite: &inputCacheWriteTokens,
			},
			OutputTokens: provider.OutputTokenUsage{Total: &outputTokens, Text: &outputTextTokens, Reasoning: &reasoningTokens},
		},
		Warnings: []provider.Warning{{Type: provider.WarnUnsupported}},
		Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{
			ID:        "resp-1",
			Provider:  "test-provider",
			ModelID:   "served-model",
			Timestamp: time.Unix(100, 0),
		}},
	}
	model := &mockModel{generateFunc: func(_ context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
		if opts.IncludeRawChunks {
			t.Fatal("logger mutated IncludeRawChunks")
		}
		return result, nil
	}}
	wrapped := Wrap(model, Options{
		Logger:       slog.New(handler),
		Clock:        clock.Now,
		Attrs:        []slog.Attr{slog.String("component", "llm")},
		DynamicAttrs: func(context.Context) []slog.Attr { return []slog.Attr{slog.String("tenant", "safe")} },
	})
	params := provider.CallOptions{
		Prompt:           []provider.Message{provider.UserText("secret prompt")},
		Tools:            []provider.Tool{{Type: provider.ToolTypeFunction, Name: "secret-tool"}},
		IncludeRawChunks: false,
	}

	got, err := wrapped.DoGenerate(context.Background(), params)
	if err != nil {
		t.Fatalf("DoGenerate returned error: %v", err)
	}
	if got != result {
		t.Fatalf("expected original result pointer")
	}
	if model.generateCalls != 1 {
		t.Fatalf("expected one generate call, got %d", model.generateCalls)
	}
	if !reflect.DeepEqual(model.lastGenerateParams, params) {
		t.Fatalf("params mutated: got %#v want %#v", model.lastGenerateParams, params)
	}

	records := handler.Records()
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d: %#v", len(records), records)
	}
	start := records[0].AttrsMap()
	if records[0].Message != string(EventGenerateStart) || start["ai_sdk.call.type"] != "generate" {
		t.Fatalf("unexpected start record: %#v", records[0])
	}
	if start["ai_sdk.provider"] != "test" || start["ai_sdk.model"] != "model" {
		t.Fatalf("missing model identity attrs: %#v", start)
	}
	if start["component"] != "llm" || start["tenant"] != "safe" {
		t.Fatalf("missing static/dynamic attrs: %#v", start)
	}
	if start["ai_sdk.request.tools.count"] != int64(1) {
		t.Fatalf("missing tools count: %#v", start)
	}

	finish := records[1].AttrsMap()
	if records[1].Message != string(EventGenerateFinish) {
		t.Fatalf("unexpected finish message: %s", records[1].Message)
	}
	assertAttr(t, finish, "ai_sdk.success", true)
	assertAttr(t, finish, "ai_sdk.outcome", outcomeSuccess)
	assertAttr(t, finish, "ai_sdk.duration_ms", float64(100))
	assertAttr(t, finish, "ai_sdk.duration_ns", int64((100 * time.Millisecond).Nanoseconds()))
	assertAttr(t, finish, "ai_sdk.finish_reason", string(provider.FinishReasonStop))
	assertAttr(t, finish, "ai_sdk.finish_reason.raw", "stop_sequence")
	assertAttr(t, finish, "ai_sdk.usage.input_tokens.total", int64(inputTokens))
	assertAttr(t, finish, "ai_sdk.usage.input_tokens.no_cache", int64(inputNoCacheTokens))
	assertAttr(t, finish, "ai_sdk.usage.input_tokens.cache_read", int64(inputCacheReadTokens))
	assertAttr(t, finish, "ai_sdk.usage.input_tokens.cache_write", int64(inputCacheWriteTokens))
	assertAttr(t, finish, "gen_ai.usage.input_tokens", int64(inputTokens))
	assertAttr(t, finish, "ai_sdk.usage.output_tokens.total", int64(outputTokens))
	assertAttr(t, finish, "ai_sdk.usage.output_tokens.text", int64(outputTextTokens))
	assertAttr(t, finish, "gen_ai.usage.output_tokens", int64(outputTokens))
	assertAttr(t, finish, "ai_sdk.usage.output_tokens.reasoning", int64(reasoningTokens))
	assertAttr(t, finish, "ai_sdk.warnings.count", int64(1))
	assertAttr(t, finish, "ai_sdk.warnings.types", []any{string(provider.WarnUnsupported)})
	assertAttr(t, finish, "ai_sdk.response.id", "resp-1")
	assertAttr(t, finish, "ai_sdk.provider", "test-provider")
	assertAttr(t, finish, "ai_sdk.model", "served-model")
	assertAttr(t, finish, "ai_sdk.transport.provider", "test")
	assertAttr(t, finish, "ai_sdk.transport.model", "model")
	assertAttr(t, finish, "gen_ai.system", "test-provider")
	assertAttr(t, finish, "gen_ai.request.model", "model")
	assertAttr(t, finish, "gen_ai.response.model", "served-model")
	if start["ai_sdk.call.id"] == "" || finish["ai_sdk.call.id"] != start["ai_sdk.call.id"] {
		t.Fatalf("start/finish call id mismatch: start=%#v finish=%#v", start["ai_sdk.call.id"], finish["ai_sdk.call.id"])
	}
	if _, ok := finish["ai_sdk.response.content"]; ok {
		t.Fatalf("default logging captured output content: %#v", finish["ai_sdk.response.content"])
	}
}

func TestMiddleware_GenerateResponseIdentityUsesBackendAndRecordsTransport(t *testing.T) {
	handler := newTestHandler()
	model := &mockModel{
		provider_: "grafana",
		modelID:   "claude-sonnet-4-5-20250929",
		generateFunc: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return &provider.GenerateResult{
				Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{
					Provider: "anthropic",
					ModelID:  "claude-sonnet-4-5-20250929",
				}},
			}, nil
		},
	}
	wrapped := Wrap(model, Options{Logger: slog.New(handler)})

	_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
	if err != nil {
		t.Fatalf("DoGenerate returned error: %v", err)
	}
	records := handler.Records()
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	start := records[0].AttrsMap()
	finish := records[1].AttrsMap()
	assertAttr(t, start, "ai_sdk.provider", "grafana")
	assertAttr(t, finish, "ai_sdk.provider", "anthropic")
	assertAttr(t, finish, "ai_sdk.transport.provider", "grafana")
	assertAttr(t, finish, "ai_sdk.transport.model", "claude-sonnet-4-5-20250929")
	assertAttr(t, finish, "gen_ai.system", "anthropic")
}

func TestMiddleware_GenerateErrorLogsAndPropagates(t *testing.T) {
	handler := newTestHandler()
	clock := newStepClock(50 * time.Millisecond)
	sentinel := errors.New("sentinel failure")
	model := &mockModel{generateFunc: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return nil, sentinel
	}}
	wrapped := Wrap(model, Options{Logger: slog.New(handler), Clock: clock.Now})

	_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if model.generateCalls != 1 {
		t.Fatalf("expected one call, got %d", model.generateCalls)
	}
	records := handler.Records()
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[1].Message != string(EventGenerateError) {
		t.Fatalf("expected error event, got %s", records[1].Message)
	}
	attrs := records[1].AttrsMap()
	assertAttr(t, attrs, "ai_sdk.success", false)
	assertAttr(t, attrs, "ai_sdk.outcome", outcomeError)
	assertAttr(t, attrs, "ai_sdk.duration_ms", float64(50))
	assertAttr(t, attrs, "ai_sdk.error.type", "unknown")
	if !strings.Contains(attrs["ai_sdk.error.message"].(string), "sentinel failure") {
		t.Fatalf("missing error message: %#v", attrs)
	}
}

func TestMiddleware_APICallErrorURLRequiresExplicitCapture(t *testing.T) {
	handler := newTestHandler()
	secret := "TOP-SECRET-VALUE"
	apiErr := provider.NewAPICallError(provider.APICallErrorOptions{
		Message:    "failed",
		StatusCode: 401,
		URL:        "https://example.test/v1/chat?access_token=" + secret,
	})
	model := &mockModel{generateFunc: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return nil, apiErr
	}}
	wrapped := Wrap(model, Options{Logger: slog.New(handler)})

	_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
	if !errors.Is(err, apiErr) {
		t.Fatalf("expected API error, got %v", err)
	}
	encoded := handler.JSON(t)
	if strings.Contains(encoded, secret) || strings.Contains(encoded, "ai_sdk.error.url") {
		t.Fatalf("default records leaked API URL or secret: %s", encoded)
	}
}

func TestMiddleware_PrivacyDefaultsDoNotLogSensitivePayloads(t *testing.T) {
	handler := newTestHandler()
	secret := "TOP-SECRET-VALUE"
	model := &mockModel{generateFunc: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return &provider.GenerateResult{
			Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: secret}},
			Response: &provider.GenerateResponse{
				Headers: map[string]string{"x-api-key": secret},
				Body:    json.RawMessage(`{"secret":"TOP-SECRET-VALUE"}`),
			},
		}, nil
	}}
	wrapped := Wrap(model, Options{Logger: slog.New(handler)})
	params := provider.CallOptions{
		Prompt:  []provider.Message{provider.UserText(secret)},
		Headers: map[string]string{"authorization": "Bearer " + secret},
		ProviderOptions: provider.ProviderOptions{
			"test": provider.RawProviderOption{Key: "test", Raw: json.RawMessage(`{"access_token":"TOP-SECRET-VALUE"}`)},
		},
	}

	if _, err := wrapped.DoGenerate(context.Background(), params); err != nil {
		t.Fatalf("DoGenerate returned error: %v", err)
	}
	encoded := handler.JSON(t)
	if strings.Contains(encoded, secret) {
		t.Fatalf("default records leaked secret: %s", encoded)
	}
}

func TestMiddleware_CapturedPayloadsAreBounded(t *testing.T) {
	handler := newTestHandler()
	model := &mockModel{generateFunc: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: strings.Repeat("o", 80)}}}, nil
	}}
	wrapped := Wrap(model, Options{
		Logger: slog.New(handler),
		Capture: CaptureOptions{
			Inputs:       true,
			Outputs:      true,
			MaxStringLen: 16,
			MaxJSONBytes: 64,
		},
	})

	if _, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText(strings.Repeat("p", 80))},
	}); err != nil {
		t.Fatalf("DoGenerate returned error: %v", err)
	}
	records := handler.Records()
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	prompt := records[0].AttrsMap()["ai_sdk.request.prompt"]
	promptGroup, ok := prompt.(map[string]any)
	if !ok || promptGroup["truncated"] != true || promptGroup["array_length"] == nil {
		t.Fatalf("expected prompt JSON truncation group with array shape, got %#v", prompt)
	}
	content := records[1].AttrsMap()["ai_sdk.response.content"]
	contentGroup, ok := content.(map[string]any)
	if !ok || contentGroup["truncated"] != true || contentGroup["array_length"] == nil {
		t.Fatalf("expected response content JSON truncation group with array shape, got %#v", content)
	}
	if got := boundString(strings.Repeat("x", 80), 16); !strings.Contains(got, "...") || len(got) != 16 {
		t.Fatalf("expected bounded string with suffix, got %q", got)
	}
}

func TestMiddleware_PanickingRedactorFallsBackToDefaultRedactor(t *testing.T) {
	secret := "REDACTOR-PANIC-SECRET"
	handler := newTestHandler()
	wrapped := Wrap(&mockModel{}, Options{
		Logger: slog.New(handler),
		Attrs:  []slog.Attr{slog.String("authorization", "Bearer "+secret)},
		Redactor: RedactorFunc(func(context.Context, EventKind, []slog.Attr) []slog.Attr {
			panic("boom")
		}),
	})

	if _, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{}); err != nil {
		t.Fatalf("DoGenerate returned error: %v", err)
	}
	encoded := handler.JSON(t)
	if strings.Contains(encoded, secret) {
		t.Fatalf("fallback redactor leaked secret: %s", encoded)
	}
	if !strings.Contains(encoded, redactedValue) || !strings.Contains(encoded, "redactor panic") {
		t.Fatalf("expected fallback redaction marker and panic diagnostic: %s", encoded)
	}
}

func TestMiddleware_PanickingDynamicAttrsLogsDiagnostic(t *testing.T) {
	handler := newTestHandler()
	wrapped := Wrap(&mockModel{}, Options{
		Logger: slog.New(handler),
		DynamicAttrs: func(context.Context) []slog.Attr {
			panic("boom")
		},
	})

	if _, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{}); err != nil {
		t.Fatalf("DoGenerate returned error: %v", err)
	}
	encoded := handler.JSON(t)
	if !strings.Contains(encoded, "dynamic attrs panic") {
		t.Fatalf("expected dynamic attrs panic diagnostic: %s", encoded)
	}
}

func TestMiddleware_CaptureOptionsOptInPayloadsStillRedactsKnownSecrets(t *testing.T) {
	handler := newTestHandler()
	secret := "TOP-SECRET-VALUE"
	model := &mockModel{generateFunc: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "visible output"}}}, nil
	}}
	wrapped := Wrap(model, Options{
		Logger: slog.New(handler),
		Capture: CaptureOptions{
			Inputs:          true,
			Outputs:         true,
			Headers:         true,
			ProviderOptions: true,
		},
	})
	params := provider.CallOptions{
		Prompt:  []provider.Message{provider.UserText("visible prompt")},
		Headers: map[string]string{"Authorization": "Bearer " + secret},
		ProviderOptions: provider.ProviderOptions{
			"test": provider.RawProviderOption{Key: "test", Raw: json.RawMessage(`{"access_token":"TOP-SECRET-VALUE","mode":"safe"}`)},
		},
	}

	if _, err := wrapped.DoGenerate(context.Background(), params); err != nil {
		t.Fatalf("DoGenerate returned error: %v", err)
	}
	encoded := handler.JSON(t)
	if !strings.Contains(encoded, "visible prompt") || !strings.Contains(encoded, "visible output") {
		t.Fatalf("expected captured prompt/output in records: %s", encoded)
	}
	if strings.Contains(encoded, secret) {
		t.Fatalf("captured records leaked secret: %s", encoded)
	}
	if !strings.Contains(encoded, redactedValue) {
		t.Fatalf("expected redaction marker in records: %s", encoded)
	}
}

type mockModel struct {
	provider_          string
	modelID            string
	generateFunc       func(context.Context, provider.CallOptions) (*provider.GenerateResult, error)
	streamFunc         func(context.Context, provider.CallOptions) (*provider.StreamResult, error)
	generateCalls      int
	streamCalls        int
	lastGenerateParams provider.CallOptions
	lastStreamParams   provider.CallOptions
}

func (m *mockModel) SpecificationVersion() string { return "v4" }
func (m *mockModel) Provider() string {
	if m.provider_ != "" {
		return m.provider_
	}
	return "test"
}
func (m *mockModel) ModelID() string {
	if m.modelID != "" {
		return m.modelID
	}
	return "model"
}
func (m *mockModel) SupportedURLs() map[string][]*regexp.Regexp {
	return nil
}
func (m *mockModel) DoGenerate(ctx context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
	m.generateCalls++
	m.lastGenerateParams = opts
	if m.generateFunc != nil {
		return m.generateFunc(ctx, opts)
	}
	return &provider.GenerateResult{}, nil
}
func (m *mockModel) DoStream(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	m.streamCalls++
	m.lastStreamParams = opts
	if m.streamFunc != nil {
		return m.streamFunc(ctx, opts)
	}
	ch := make(chan provider.StreamPart)
	close(ch)
	return &provider.StreamResult{Stream: ch}, nil
}

type stepClock struct {
	mu      sync.Mutex
	now     time.Time
	advance time.Duration
}

func newStepClock(advance time.Duration) *stepClock {
	return &stepClock{now: time.Unix(0, 0), advance: advance}
}

func (c *stepClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now
	c.now = c.now.Add(c.advance)
	return now
}

func assertAttr(t *testing.T, attrs map[string]any, key string, want any) {
	t.Helper()
	got, ok := attrs[key]
	if !ok {
		t.Fatalf("missing attr %q in %#v", key, attrs)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("attr %q = %#v, want %#v", key, got, want)
	}
}
