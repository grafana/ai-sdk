package extensionhost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/provider"
)

func newHost(t *testing.T, definitions ...extension.Definition) (*Host, error) {
	t.Helper()
	plan, err := NewPlan(definitions, nil, nil)
	require.NoError(t, err)
	return New(Config{Plan: plan, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Registerer: prometheus.NewRegistry(), Clock: time.Now, RequestInfo: func(context.Context) extension.Call { return extension.Call{} }, DrainTimeout: time.Millisecond})
}

func TestNew_Construction(t *testing.T) {
	closed := make(chan struct{}, 1)
	built := define("a-built", &recorder{close: func(context.Context) error {
		closed <- struct{}{}
		return nil
	}})
	for _, tc := range []struct {
		name    string
		factory func(json.RawMessage, extension.Dependencies) (any, error)
		message string
	}{
		{name: "factory error", factory: func(json.RawMessage, extension.Dependencies) (any, error) { return nil, errors.New("private") }, message: `constructing extension "b-broken" failed`},
		{name: "factory panic", factory: func(json.RawMessage, extension.Dependencies) (any, error) { panic("private") }, message: `constructing extension "b-broken" failed`},
		{name: "no phase", factory: func(json.RawMessage, extension.Dependencies) (any, error) { return struct{}{}, nil }, message: `extension "b-broken" implements no phase`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			broken := define("b-broken", nil)
			broken.New = tc.factory
			_, err := newHost(t, built, broken)
			require.Error(t, err)
			assert.Equal(t, "extensions: "+tc.message, err.Error())
			select {
			case <-closed:
			case <-time.After(time.Second):
				require.FailNow(t, "an instance built before the failure was not closed")
			}
		})
	}

	t.Run("dependencies", func(t *testing.T) {
		var received extension.Dependencies
		var configuration json.RawMessage
		definition := define("quota", nil)
		definition.New = func(config json.RawMessage, deps extension.Dependencies) (any, error) {
			configuration, received = config, deps
			return &recorder{}, nil
		}
		registry := prometheus.NewRegistry()
		plan, err := NewPlan([]extension.Definition{definition}, nil, nil)
		require.NoError(t, err)
		_, err = New(Config{Plan: plan, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Registerer: registry, Clock: time.Now, RequestInfo: func(context.Context) extension.Call { return extension.Call{} }, DrainTimeout: time.Millisecond})
		require.NoError(t, err)
		assert.JSONEq(t, `{}`, string(configuration))
		require.NotNil(t, received.Logger)
		require.NotNil(t, received.Clock)
		counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "quota_reservations_total"})
		require.NoError(t, received.Registerer.Register(counter))
		counter.Inc()
		assert.Equal(t, 1.0, counterValue(registry, "quota_reservations_total", map[string]string{"extension": "quota"}))
		assert.Zero(t, counterValue(registry, "quota_reservations_total", map[string]string{"extension": "other"}))
	})

	t.Run("duplicate metric registration", func(t *testing.T) {
		registry := prometheus.NewRegistry()
		plan, err := NewPlan(nil, nil, nil)
		require.NoError(t, err)
		config := Config{Plan: plan, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Registerer: registry, Clock: time.Now, RequestInfo: func(context.Context) extension.Call { return extension.Call{} }}
		_, err = New(config)
		require.NoError(t, err)
		_, err = New(config)
		assert.ErrorContains(t, err, "extensions: registering metrics")
	})
}

func TestHost_Compose(t *testing.T) {
	operatorOnly := define("operator", &recorder{}, func(d *extension.Definition) { d.Keys = []extension.KeyOwnership{extension.KeysOperator} })
	model := &fakeModel{}

	t.Run("no extension applies", func(t *testing.T) {
		host, err := newHost(t, operatorOnly)
		require.NoError(t, err)
		composed, err := host.Compose(execution.Selection{ID: "openai/gpt-5", Model: model, Source: extension.SourceRequestBYOK})
		require.NoError(t, err)
		assert.Same(t, model, composed)
	})

	for _, tc := range []struct {
		source  extension.Source
		applies []string
	}{
		{source: extension.SourceOperatorCatalog, applies: []string{"all", "operator"}},
		{source: extension.SourceRequestBYOK, applies: []string{"all", "customer"}},
		{source: extension.SourceStackCatalog, applies: []string{"all", "customer"}},
		{source: "a-later-source", applies: []string{"all", "customer"}},
	} {
		t.Run(string(tc.source), func(t *testing.T) {
			log := &journal{}
			h := newHarness(t,
				define("all", &recorder{name: "all", journal: log}),
				define("operator", &recorder{name: "operator", journal: log}, func(d *extension.Definition) { d.Keys = operatorOnly.Keys }),
				define("customer", &recorder{name: "customer", journal: log}, func(d *extension.Definition) { d.Keys = []extension.KeyOwnership{extension.KeysCustomer} }),
			)
			_, err := h.compose(t, tc.source).DoGenerate(t.Context(), provider.CallOptions{})
			require.NoError(t, err)
			h.flush()
			ran := map[string]bool{}
			for _, entry := range log.list() {
				name, _, _ := strings.Cut(entry, ":")
				ran[name] = true
			}
			assert.Len(t, ran, len(tc.applies))
			for _, name := range tc.applies {
				assert.True(t, ran[name], name)
			}
		})
	}
}
