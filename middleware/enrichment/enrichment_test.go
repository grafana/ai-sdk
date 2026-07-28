package enrichment

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/registry"
)

func TestPublicAPI_Smoke(t *testing.T) {
	var _ Redactor = RedactorFunc(func(context.Context, Value) (Value, bool) { return Value{}, false })
	var _ Redactor = defaultRedactor{}

	ctx := context.Background()
	ctx = WithValue(ctx, "secret", "value", Sensitive(), WithCardinality(CardinalityHigh))
	ctx = WithValues(ctx, Value{Key: "service", Value: "api", Cardinality: CardinalityLow})
	values := ValuesFromContext(ctx)
	if len(values) != 2 {
		t.Fatalf("expected 2 context values, got %d", len(values))
	}

	model := &captureModel{}
	wrapped := Wrap(model, Options{
		Values: []Value{{Key: "service", Value: "api"}},
		Headers: HeaderOptions{
			Map: map[string]string{"service": "X-Service"},
		},
	})
	if wrapped == nil {
		t.Fatal("expected wrapped model")
	}
	_ = Middleware(Options{
		DynamicValues: func(context.Context, CallInput) ([]Value, error) { return nil, nil },
		Filter:        FilterOptions{Include: []string{"service"}},
		ProviderOptions: ProviderOptionsConfig{
			ProviderKey: "test",
			ObjectKey:   "enrichment",
			Map:         map[string]string{"service": "serviceName"},
			Conflict:    ConflictCallerWins,
		},
	})
}

func TestContextHelpers_DefensiveCopyAndOptIn(t *testing.T) {
	ctx := WithValue(context.Background(), "request_id", "req-1", WithCardinality(CardinalityHigh))
	values := ValuesFromContext(ctx)
	values[0].Value = "mutated"
	if got := ValuesFromContext(ctx)[0].Value; got != "req-1" {
		t.Fatalf("context values mutated through returned slice: %q", got)
	}

	model := &captureModel{}
	_, err := Wrap(model, Options{
		Headers: HeaderOptions{Map: map[string]string{"request_id": "X-Request-Id"}},
	}).DoGenerate(ctx, provider.CallOptions{})
	if err != nil {
		t.Fatalf("generate without context values opt-in: %v", err)
	}
	if model.generateParams.Headers != nil {
		t.Fatalf("expected no headers without ContextValues opt-in, got %#v", model.generateParams.Headers)
	}

	_, err = Wrap(model, Options{
		ContextValues: true,
		Headers:       HeaderOptions{Map: map[string]string{"request_id": "X-Request-Id"}},
	}).DoGenerate(ctx, provider.CallOptions{})
	if err != nil {
		t.Fatalf("generate with context values opt-in: %v", err)
	}
	if got := model.generateParams.Headers["X-Request-Id"]; got != "req-1" {
		t.Fatalf("expected context header req-1, got %q", got)
	}
}

func TestDynamicValues_ReceivesCallMetadata(t *testing.T) {
	model := &captureModel{}
	var input CallInput
	_, err := Wrap(model, Options{
		DynamicValues: func(_ context.Context, in CallInput) ([]Value, error) {
			input = in
			return []Value{{Key: "route", Value: "primary"}}, nil
		},
		Headers: HeaderOptions{Map: map[string]string{"route": "X-Route"}},
	}).DoGenerate(context.Background(), provider.CallOptions{Headers: map[string]string{"Existing": "true"}})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if input.Type != middleware.CallTypeGenerate {
		t.Fatalf("expected generate call type, got %q", input.Type)
	}
	if input.Model != model {
		t.Fatal("expected input model")
	}
	if got := input.Params.Headers["Existing"]; got != "true" {
		t.Fatalf("expected current params in dynamic input, got %q", got)
	}
}

func TestFiltering_DefaultDenySelectionAndNormalization(t *testing.T) {
	t.Run("default deny", func(t *testing.T) {
		params, err := transformForTest(Options{
			Values:          []Value{{Key: "tenant", Value: "acme"}},
			Headers:         HeaderOptions{Prefix: "X-AI-"},
			ProviderOptions: ProviderOptionsConfig{ProviderKey: "test", ObjectKey: "enrichment"},
		})
		if err != nil {
			t.Fatalf("transform: %v", err)
		}
		if params.Headers != nil || params.ProviderOptions != nil {
			t.Fatalf("expected no output, got headers=%#v providerOptions=%#v", params.Headers, params.ProviderOptions)
		}
	})

	t.Run("include exclude", func(t *testing.T) {
		params, err := transformForTest(Options{
			Values: []Value{{Key: "tenant", Value: "acme"}, {Key: "drop", Value: "no"}},
			Filter: FilterOptions{
				Include: []string{"tenant", "drop"},
				Exclude: []string{"drop"},
			},
			Headers: HeaderOptions{Prefix: "X-AI-"},
		})
		if err != nil {
			t.Fatalf("transform: %v", err)
		}
		if got := params.Headers["X-Ai-Tenant"]; got != "acme" {
			t.Fatalf("expected tenant header, got %q", got)
		}
		if _, ok := params.Headers["X-Ai-Drop"]; ok {
			t.Fatal("excluded value was emitted")
		}
	})

	t.Run("output mapping isolation", func(t *testing.T) {
		params, err := transformForTest(Options{
			Values: []Value{{Key: "request_id", Value: "req-1"}, {Key: "tenant", Value: "acme"}},
			Headers: HeaderOptions{Map: map[string]string{
				"request_id": "X-Request-Id",
			}},
			ProviderOptions: ProviderOptionsConfig{
				ProviderKey: "test",
				ObjectKey:   "enrichment",
				Map:         map[string]string{"tenant": "tenantName"},
			},
		})
		if err != nil {
			t.Fatalf("transform: %v", err)
		}
		if got := params.Headers["X-Request-Id"]; got != "req-1" {
			t.Fatalf("expected header request id, got %q", got)
		}
		obj := rawProviderOptionObject(t, params.ProviderOptions, "test")
		enrichment := enrichmentObjectForTest(t, obj)
		if got := stringValueFromRaw(t, enrichment["tenantName"]); got != "acme" {
			t.Fatalf("expected mapped tenant, got %q", got)
		}
		if _, ok := enrichment["request_id"]; ok {
			t.Fatal("header-only value leaked into provider options")
		}
	})

	t.Run("redaction drop cardinality and length", func(t *testing.T) {
		params, err := transformForTest(Options{
			Values: []Value{
				{Key: "api_token", Value: "secret"},
				{Key: "dropped", Value: "drop-me"},
				{Key: "trace", Value: "trace-1", Cardinality: CardinalityHigh},
				{Key: "long", Value: "123456789012345678901"},
				{Key: "ok", Value: "yes"},
			},
			Filter: FilterOptions{
				Include:             []string{"api_token", "dropped", "trace", "long", "ok"},
				RedactSensitive:     true,
				DropHighCardinality: true,
				MaxValueLength:      20,
			},
			Redactor: RedactorFunc(func(_ context.Context, value Value) (Value, bool) {
				if value.Key == "dropped" {
					return value, false
				}
				return DefaultRedactor().RedactValue(context.Background(), value)
			}),
			Headers: HeaderOptions{Prefix: "X-AI-"},
		})
		if err != nil {
			t.Fatalf("transform: %v", err)
		}
		if got := params.Headers["X-Ai-Api_token"]; got != redactedValue {
			t.Fatalf("expected redacted token, got %q", got)
		}
		for _, header := range []string{"X-Ai-Dropped", "X-Ai-Trace", "X-Ai-Long"} {
			if _, ok := params.Headers[header]; ok {
				t.Fatalf("unexpected header %s", header)
			}
		}
		if got := params.Headers["X-Ai-Ok"]; got != "yes" {
			t.Fatalf("expected ok header, got %q", got)
		}
	})
}

func TestErrorHandling_DefaultFailClosedAndOnError(t *testing.T) {
	dynamicErr := errors.New("dynamic failed")
	model := &captureModel{}
	_, err := Wrap(model, Options{
		DynamicValues: func(context.Context, CallInput) ([]Value, error) { return nil, dynamicErr },
	}).DoGenerate(context.Background(), provider.CallOptions{})
	if !errors.Is(err, dynamicErr) {
		t.Fatalf("expected dynamic error, got %v", err)
	}
	if model.generateCalls != 0 {
		t.Fatal("inner model called after fail-closed dynamic error")
	}

	model = &captureModel{}
	_, err = Wrap(model, Options{
		DynamicValues: func(context.Context, CallInput) ([]Value, error) { return nil, dynamicErr },
		OnError:       func(context.Context, error) error { return nil },
	}).DoGenerate(context.Background(), provider.CallOptions{})
	if err != nil {
		t.Fatalf("expected fail-open nil error, got %v", err)
	}
	if model.generateCalls != 1 {
		t.Fatal("inner model not called after fail-open dynamic error")
	}

	replacement := errors.New("replacement")
	model = &captureModel{}
	_, err = Wrap(model, Options{
		Values:  []Value{{Key: "route", Value: "new"}},
		Headers: HeaderOptions{Map: map[string]string{"route": "X-Route"}, Conflict: ConflictError},
		OnError: func(context.Context, error) error { return replacement },
	}).DoGenerate(context.Background(), provider.CallOptions{Headers: map[string]string{"X-Route": "old"}})
	if !errors.Is(err, replacement) {
		t.Fatalf("expected replacement error, got %v", err)
	}
}

func TestHeaderOutput_MergeSemantics(t *testing.T) {
	t.Run("caller wins by default", func(t *testing.T) {
		params, err := transformForTestWithParams(Options{
			Values:  []Value{{Key: "request_id", Value: "enriched"}},
			Headers: HeaderOptions{Map: map[string]string{"request_id": "X-Request-Id"}},
		}, provider.CallOptions{Headers: map[string]string{"X-Request-Id": "caller"}})
		if err != nil {
			t.Fatalf("transform: %v", err)
		}
		if got := params.Headers["X-Request-Id"]; got != "caller" {
			t.Fatalf("expected caller header, got %q", got)
		}
	})

	t.Run("case insensitive conflict and enrichment wins", func(t *testing.T) {
		original := map[string]string{"x-request-id": "caller"}
		params, err := transformForTestWithParams(Options{
			Values: []Value{{Key: "request_id", Value: "enriched"}},
			Headers: HeaderOptions{
				Map:      map[string]string{"request_id": "X-Request-Id"},
				Conflict: ConflictEnrichmentWins,
			},
		}, provider.CallOptions{Headers: original})
		if err != nil {
			t.Fatalf("transform: %v", err)
		}
		if got := params.Headers["X-Request-Id"]; got != "enriched" {
			t.Fatalf("expected enriched header, got %q", got)
		}
		if _, ok := params.Headers["x-request-id"]; ok {
			t.Fatal("expected canonical header replacement")
		}
		if original["x-request-id"] != "caller" {
			t.Fatal("original header map mutated")
		}
	})

	t.Run("conflict error", func(t *testing.T) {
		_, err := transformForTestWithParams(Options{
			Values:  []Value{{Key: "request_id", Value: "enriched"}},
			Headers: HeaderOptions{Map: map[string]string{"request_id": "X-Request-Id"}, Conflict: ConflictError},
		}, provider.CallOptions{Headers: map[string]string{"X-Request-Id": "caller"}})
		if err == nil {
			t.Fatal("expected conflict error")
		}
	})

	t.Run("protected absent and overwrite", func(t *testing.T) {
		params, err := transformForTestWithParams(Options{
			Values: []Value{{Key: "auth", Value: "secret"}, {Key: "access", Value: "secret"}, {Key: "content", Value: "text/plain"}},
			Headers: HeaderOptions{
				Map: map[string]string{
					"auth":    "Authorization",
					"access":  "X-Access-Token",
					"content": "Content-Type",
				},
				Conflict: ConflictEnrichmentWins,
			},
		}, provider.CallOptions{Headers: map[string]string{"Authorization": "caller"}})
		if err != nil {
			t.Fatalf("transform: %v", err)
		}
		if got := params.Headers["Authorization"]; got != "caller" {
			t.Fatalf("expected caller protected header, got %q", got)
		}
		if _, ok := params.Headers["X-Access-Token"]; ok {
			t.Fatal("absent protected header was written")
		}
		if _, ok := params.Headers["Content-Type"]; ok {
			t.Fatal("protected content-type was written")
		}
	})

	t.Run("additional protected and prefix mode", func(t *testing.T) {
		params, err := transformForTest(Options{
			Values: []Value{{Key: "tenant", Value: "acme"}, {Key: "secret", Value: "no"}},
			Filter: FilterOptions{Include: []string{"tenant", "secret"}},
			Headers: HeaderOptions{
				Prefix:              "X-AI-",
				AdditionalProtected: []string{"X-AI-Secret"},
			},
		})
		if err != nil {
			t.Fatalf("transform: %v", err)
		}
		if got := params.Headers["X-Ai-Tenant"]; got != "acme" {
			t.Fatalf("expected prefix header, got %q", got)
		}
		if _, ok := params.Headers["X-Ai-Secret"]; ok {
			t.Fatal("additional protected header was written")
		}
	})
}

func TestProviderOptionsOutput_MergeSemantics(t *testing.T) {
	t.Run("absent provider key creates raw option", func(t *testing.T) {
		params, err := transformForTest(Options{
			Values: []Value{{Key: "tenant", Value: "acme"}},
			Filter: FilterOptions{Include: []string{"tenant"}},
			ProviderOptions: ProviderOptionsConfig{
				ProviderKey: "test",
				ObjectKey:   "enrichment",
			},
		})
		if err != nil {
			t.Fatalf("transform: %v", err)
		}
		obj := rawProviderOptionObject(t, params.ProviderOptions, "test")
		enrichment := enrichmentObjectForTest(t, obj)
		if got := stringValueFromRaw(t, enrichment["tenant"]); got != "acme" {
			t.Fatalf("expected tenant, got %q", got)
		}
	})

	t.Run("raw object merge preserves unrelated fields", func(t *testing.T) {
		existingRaw := json.RawMessage(`{"existing":"keep","enrichment":{"old":"value"}}`)
		params, err := transformForTestWithParams(Options{
			Values: []Value{{Key: "tenant", Value: "acme"}},
			Filter: FilterOptions{Include: []string{"tenant"}},
			ProviderOptions: ProviderOptionsConfig{
				ProviderKey: "test",
				ObjectKey:   "enrichment",
			},
		}, provider.CallOptions{ProviderOptions: provider.ProviderOptions{
			"test": provider.RawProviderOption{Key: "test", Raw: existingRaw},
		}})
		if err != nil {
			t.Fatalf("transform: %v", err)
		}
		obj := rawProviderOptionObject(t, params.ProviderOptions, "test")
		if got := stringValueFromRaw(t, obj["existing"]); got != "keep" {
			t.Fatalf("expected existing field, got %q", got)
		}
		enrichment := enrichmentObjectForTest(t, obj)
		if got := stringValueFromRaw(t, enrichment["old"]); got != "value" {
			t.Fatalf("expected old field, got %q", got)
		}
		if got := stringValueFromRaw(t, enrichment["tenant"]); got != "acme" {
			t.Fatalf("expected tenant, got %q", got)
		}
	})

	t.Run("typed option merge and resolve option", func(t *testing.T) {
		params, err := transformForTestWithParams(Options{
			Values: []Value{{Key: "tenant", Value: "acme"}},
			Filter: FilterOptions{Include: []string{"tenant"}},
			ProviderOptions: ProviderOptionsConfig{
				ProviderKey: "test",
				ObjectKey:   "enrichment",
			},
		}, provider.CallOptions{ProviderOptions: provider.ProviderOptions{
			"test": testProviderOption{Existing: "keep"},
		}})
		if err != nil {
			t.Fatalf("transform: %v", err)
		}
		if _, ok := params.ProviderOptions["test"].(provider.RawProviderOption); !ok {
			t.Fatalf("expected raw provider option, got %T", params.ProviderOptions["test"])
		}
		resolved, ok, err := provider.ResolveOption[testProviderOption](params.ProviderOptions, "test")
		if err != nil || !ok {
			t.Fatalf("resolve option ok=%v err=%v", ok, err)
		}
		if resolved.Existing != "keep" || resolved.Enrichment["tenant"] != "acme" {
			t.Fatalf("unexpected resolved option: %#v", resolved)
		}
	})

	t.Run("non object conflict policies", func(t *testing.T) {
		base := provider.CallOptions{ProviderOptions: provider.ProviderOptions{
			"test": provider.RawProviderOption{Key: "test", Raw: json.RawMessage(`"not-object"`)},
		}}
		params, err := transformForTestWithParams(Options{
			Values:          []Value{{Key: "tenant", Value: "acme"}},
			Filter:          FilterOptions{Include: []string{"tenant"}},
			ProviderOptions: ProviderOptionsConfig{ProviderKey: "test", Conflict: ConflictCallerWins},
		}, base)
		if err != nil {
			t.Fatalf("caller wins non object: %v", err)
		}
		obj := params.ProviderOptions["test"].(provider.RawProviderOption)
		if string(obj.Raw) != `"not-object"` {
			t.Fatalf("caller wins should preserve raw, got %s", obj.Raw)
		}

		params, err = transformForTestWithParams(Options{
			Values:          []Value{{Key: "tenant", Value: "acme"}},
			Filter:          FilterOptions{Include: []string{"tenant"}},
			ProviderOptions: ProviderOptionsConfig{ProviderKey: "test", Conflict: ConflictEnrichmentWins},
		}, base)
		if err != nil {
			t.Fatalf("enrichment wins non object: %v", err)
		}
		out := rawProviderOptionObject(t, params.ProviderOptions, "test")
		if got := stringValueFromRaw(t, out["tenant"]); got != "acme" {
			t.Fatalf("expected replacement tenant, got %q", got)
		}

		_, err = transformForTestWithParams(Options{
			Values:          []Value{{Key: "tenant", Value: "acme"}},
			Filter:          FilterOptions{Include: []string{"tenant"}},
			ProviderOptions: ProviderOptionsConfig{ProviderKey: "test", Conflict: ConflictError},
		}, base)
		if err == nil {
			t.Fatal("expected non-object conflict error")
		}
	})

	t.Run("top level merge mapped field and copy", func(t *testing.T) {
		original := provider.ProviderOptions{"other": testProviderOption{Existing: "other"}}
		params, err := transformForTestWithParams(Options{
			Values: []Value{{Key: "tenant", Value: "acme"}, {Key: "region", Value: "us"}},
			Filter: FilterOptions{Include: []string{"region"}},
			ProviderOptions: ProviderOptionsConfig{
				ProviderKey: "test",
				Map:         map[string]string{"tenant": "tenantName"},
			},
		}, provider.CallOptions{ProviderOptions: original})
		if err != nil {
			t.Fatalf("transform: %v", err)
		}
		obj := rawProviderOptionObject(t, params.ProviderOptions, "test")
		if got := stringValueFromRaw(t, obj["tenantName"]); got != "acme" {
			t.Fatalf("expected mapped tenant, got %q", got)
		}
		if got := stringValueFromRaw(t, obj["region"]); got != "us" {
			t.Fatalf("expected included region, got %q", got)
		}
		if _, ok := original["test"]; ok {
			t.Fatal("original provider options map mutated")
		}
	})

	t.Run("grafana controls preserved", func(t *testing.T) {
		existing := json.RawMessage(`{"agentObservability":{"captureMode":"metadata_only"},"tracing":{"disabled":true},"metrics":{"disabled":true},"usage":{"project":"p"}}`)
		params, err := transformForTestWithParams(Options{
			Values: []Value{{Key: "request_id", Value: "req-1"}},
			Filter: FilterOptions{Include: []string{"request_id"}},
			ProviderOptions: ProviderOptionsConfig{
				ProviderKey: "grafana",
				ObjectKey:   "enrichment",
			},
		}, provider.CallOptions{ProviderOptions: provider.ProviderOptions{
			"grafana": provider.RawProviderOption{Key: "grafana", Raw: existing},
		}})
		if err != nil {
			t.Fatalf("transform: %v", err)
		}
		obj := rawProviderOptionObject(t, params.ProviderOptions, "grafana")
		for _, key := range []string{"agentObservability", "tracing", "metrics", "usage"} {
			if _, ok := obj[key]; !ok {
				t.Fatalf("missing grafana control %q", key)
			}
		}
		enrichment := enrichmentObjectForTest(t, obj)
		if got := stringValueFromRaw(t, enrichment["request_id"]); got != "req-1" {
			t.Fatalf("expected request id, got %q", got)
		}
	})
}

func TestMiddlewareIntegration_GenerateStreamNoMutationRegistryAndOrdering(t *testing.T) {
	t.Run("generate and stream receive enriched params", func(t *testing.T) {
		model := &captureModel{}
		wrapped := Wrap(model, Options{
			Values:  []Value{{Key: "tenant", Value: "acme"}},
			Headers: HeaderOptions{Map: map[string]string{"tenant": "X-Tenant"}},
			ProviderOptions: ProviderOptionsConfig{
				ProviderKey: "test",
				Map:         map[string]string{"tenant": "tenantName"},
			},
		})
		if _, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{}); err != nil {
			t.Fatalf("generate: %v", err)
		}
		if got := model.generateParams.Headers["X-Tenant"]; got != "acme" {
			t.Fatalf("expected generate header, got %q", got)
		}
		if _, err := wrapped.DoStream(context.Background(), provider.CallOptions{}); err != nil {
			t.Fatalf("stream: %v", err)
		}
		if got := model.streamParams.Headers["X-Tenant"]; got != "acme" {
			t.Fatalf("expected stream header, got %q", got)
		}
	})

	t.Run("prompt tools and original maps are not mutated", func(t *testing.T) {
		model := &captureModel{}
		params := provider.CallOptions{
			Prompt:  []provider.Message{provider.UserText("hello")},
			Tools:   []provider.Tool{{Type: provider.ToolTypeFunction, Name: "tool"}},
			Headers: map[string]string{"Existing": "true"},
			ProviderOptions: provider.ProviderOptions{
				"other": testProviderOption{Existing: "keep"},
			},
		}
		original := params
		original.Prompt = append([]provider.Message(nil), params.Prompt...)
		original.Tools = append([]provider.Tool(nil), params.Tools...)
		original.Headers = cloneHeaders(params.Headers)
		original.ProviderOptions = cloneProviderOptions(params.ProviderOptions)

		_, err := Wrap(model, Options{
			Values:  []Value{{Key: "tenant", Value: "acme"}},
			Headers: HeaderOptions{Map: map[string]string{"tenant": "X-Tenant"}},
			ProviderOptions: ProviderOptionsConfig{
				ProviderKey: "test",
				Map:         map[string]string{"tenant": "tenant"},
			},
		}).DoGenerate(context.Background(), params)
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if !reflect.DeepEqual(params.Prompt, original.Prompt) || !reflect.DeepEqual(params.Tools, original.Tools) {
			t.Fatal("prompt or tools mutated")
		}
		if !reflect.DeepEqual(params.Headers, original.Headers) {
			t.Fatalf("headers mutated: %#v", params.Headers)
		}
		if !reflect.DeepEqual(params.ProviderOptions, original.ProviderOptions) {
			t.Fatalf("provider options mutated: %#v", params.ProviderOptions)
		}
	})

	t.Run("registry composition", func(t *testing.T) {
		base := &captureModel{}
		reg := registry.NewProviderRegistry(map[string]registry.Provider{
			"test": registryProvider{model: base},
		}, registry.WithLanguageModelMiddleware(Middleware(Options{
			Values:  []Value{{Key: "tenant", Value: "acme"}},
			Headers: HeaderOptions{Map: map[string]string{"tenant": "X-Tenant"}},
		})))
		model, err := reg.LanguageModel("test:model")
		if err != nil {
			t.Fatalf("registry model: %v", err)
		}
		if _, err := model.DoGenerate(context.Background(), provider.CallOptions{}); err != nil {
			t.Fatalf("generate: %v", err)
		}
		if got := base.generateParams.Headers["X-Tenant"]; got != "acme" {
			t.Fatalf("expected registry header, got %q", got)
		}
	})

	t.Run("middleware ordering", func(t *testing.T) {
		base := &captureModel{}
		var beforeHeaders map[string]string
		observer := middleware.Middleware{TransformParams: func(_ context.Context, input middleware.TransformParamsInput) (provider.CallOptions, error) {
			beforeHeaders = cloneHeaders(input.Params.Headers)
			return input.Params, nil
		}}
		enrich := Middleware(Options{
			Values:  []Value{{Key: "tenant", Value: "acme"}},
			Headers: HeaderOptions{Map: map[string]string{"tenant": "X-Tenant"}},
		})

		_, err := middleware.WrapLanguageModel(base, enrich, observer).DoGenerate(context.Background(), provider.CallOptions{})
		if err != nil {
			t.Fatalf("enrichment before observer: %v", err)
		}
		if got := beforeHeaders["X-Tenant"]; got != "acme" {
			t.Fatalf("expected observer to see enriched header, got %q", got)
		}

		beforeHeaders = nil
		_, err = middleware.WrapLanguageModel(base, observer, enrich).DoGenerate(context.Background(), provider.CallOptions{})
		if err != nil {
			t.Fatalf("observer before enrichment: %v", err)
		}
		if beforeHeaders != nil {
			t.Fatalf("expected observer to see original headers, got %#v", beforeHeaders)
		}
	})
}

func transformForTest(opts Options) (provider.CallOptions, error) {
	return transformForTestWithParams(opts, provider.CallOptions{})
}

func transformForTestWithParams(opts Options, params provider.CallOptions) (provider.CallOptions, error) {
	mw := Middleware(opts)
	return mw.TransformParams(context.Background(), middleware.TransformParamsInput{
		Type:   middleware.CallTypeGenerate,
		Params: params,
		Model:  &captureModel{},
	})
}

type captureModel struct {
	generateParams provider.CallOptions
	streamParams   provider.CallOptions
	generateCalls  int
	streamCalls    int
}

func (m *captureModel) SpecificationVersion() string { return "v4" }
func (m *captureModel) Provider() string             { return "test" }
func (m *captureModel) ModelID() string              { return "model" }
func (m *captureModel) SupportedURLs() map[string][]*regexp.Regexp {
	return nil
}
func (m *captureModel) DoGenerate(_ context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
	m.generateParams = params
	m.generateCalls++
	return &provider.GenerateResult{}, nil
}
func (m *captureModel) DoStream(_ context.Context, params provider.CallOptions) (*provider.StreamResult, error) {
	m.streamParams = params
	m.streamCalls++
	ch := make(chan provider.StreamPart)
	close(ch)
	return &provider.StreamResult{Stream: ch}, nil
}

type registryProvider struct{ model provider.LanguageModel }

func (p registryProvider) LanguageModel(string) (provider.LanguageModel, error) { return p.model, nil }

type testProviderOption struct {
	Existing   string            `json:"existing,omitempty"`
	Enrichment map[string]string `json:"enrichment,omitempty"`
}

func (testProviderOption) ProviderKey() string { return "test" }

func rawProviderOptionObject(t *testing.T, opts provider.ProviderOptions, key string) map[string]json.RawMessage {
	t.Helper()
	opt, ok := opts[key]
	if !ok {
		t.Fatalf("missing provider option %q", key)
	}
	raw, ok := opt.(provider.RawProviderOption)
	if !ok {
		t.Fatalf("expected raw provider option, got %T", opt)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw.Raw, &obj); err != nil {
		t.Fatalf("unmarshal raw provider option: %v", err)
	}
	return obj
}

func enrichmentObjectForTest(t *testing.T, obj map[string]json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	raw, ok := obj["enrichment"]
	if !ok {
		t.Fatalf("missing nested object %q in %s", "enrichment", mustJSON(obj))
	}
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err != nil {
		t.Fatalf("unmarshal nested object %q: %v", "enrichment", err)
	}
	return nested
}

func stringValueFromRaw(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("unmarshal string value %s: %v", raw, err)
	}
	return value
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return err.Error()
	}
	return string(data)
}

func TestDefaultRedactor_MarksSecretLookingKeys(t *testing.T) {
	redacted, ok := DefaultRedactor().RedactValue(context.Background(), Value{Key: "access_token", Value: "secret"})
	if !ok {
		t.Fatal("default redactor dropped value")
	}
	if !redacted.Sensitive {
		t.Fatal("default redactor did not mark token sensitive")
	}
	ordinary, ok := DefaultRedactor().RedactValue(context.Background(), Value{Key: "tenant", Value: "acme"})
	if !ok || ordinary.Sensitive || strings.Compare(ordinary.Value, "acme") != 0 {
		t.Fatalf("ordinary value changed unexpectedly: %#v ok=%v", ordinary, ok)
	}
}
