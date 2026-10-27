package execution

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rawOptions(entries map[string]string) provider.ProviderOptions {
	options := make(provider.ProviderOptions, len(entries))
	for namespace, raw := range entries {
		options[namespace] = provider.RawProviderOption{Key: namespace, Raw: json.RawMessage(raw)}
	}
	return options
}

type recordingSelector struct {
	calls   atomic.Int32
	gateway json.RawMessage
	options provider.CallOptions
	model   *generateModel
}

func (s *recordingSelector) selector() RequestSelector {
	return func(_ context.Context, id string, options provider.CallOptions, gateway json.RawMessage) (Selection, error) {
		s.calls.Add(1)
		s.gateway, s.options = gateway, options
		return Selection{ID: id, Model: s.model}, nil
	}
}

func TestRuntimeCall_ProtectedCallHeaders(t *testing.T) {
	names := []string{"Authorization", "proxy-authorization", "X-ACCESS-TOKEN", "x-grafana-id", "X-Api-Key", "api-key", "OpenAI-API-Key", "anthropic-api-key", "OpenAI-Organization", "OpenAI-Project", "X-Scope-OrgID", "X-Cloud-Org-ID", "X-Access-Policy-ID"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			selector := &recordingSelector{model: &generateModel{}}
			_, err := newRuntime(t, selector.selector(), testLimits()).Call(t.Context(), "id", provider.CallOptions{Headers: map[string]string{"X-Ordinary": "kept", name: "caller-controlled"}})
			require.ErrorIs(t, err, ErrProtectedCallHeader)
			assert.NotContains(t, err.Error(), "caller-controlled")
			assert.NotContains(t, strings.ToLower(err.Error()), strings.ToLower(name))
			category, reason := ClassifySelection(err)
			assert.Equal(t, CategoryInvalidRequest, category)
			assert.Equal(t, ReasonProtectedCallHeader, reason)
			assert.Zero(t, selector.calls.Load())
			assert.Zero(t, selector.model.callCount())
		})
	}

	t.Run("ordinary headers keep their case", func(t *testing.T) {
		selector := &recordingSelector{model: &generateModel{}}
		_, err := newRuntime(t, selector.selector(), testLimits()).Call(t.Context(), "id", provider.CallOptions{Headers: map[string]string{"X-Request-ID": "value"}})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"X-Request-ID": "value"}, selector.options.Headers)
	})
}

func TestRuntimeCall_GatewayControls(t *testing.T) {
	t.Run("reach only the selector", func(t *testing.T) {
		model := &generateModel{}
		selector := &recordingSelector{model: model}
		options := provider.CallOptions{ProviderOptions: rawOptions(map[string]string{ControlNamespace: `{"byok":{"anthropic":[]}}`, "anthropic": `{"effort":"low"}`})}
		call, err := newRuntime(t, selector.selector(), testLimits()).Call(t.Context(), "id", options)
		require.NoError(t, err)
		assert.JSONEq(t, `{"byok":{"anthropic":[]}}`, string(selector.gateway))
		assert.NotContains(t, selector.options.ProviderOptions, ControlNamespace)
		assert.NotContains(t, call.options.ProviderOptions, ControlNamespace)
		assert.Contains(t, call.options.ProviderOptions, "anthropic")
		assert.Contains(t, options.ProviderOptions, ControlNamespace, "caller options are not modified")
	})

	t.Run("only controls leave no options", func(t *testing.T) {
		selector := &recordingSelector{model: &generateModel{}}
		_, err := newRuntime(t, selector.selector(), testLimits()).Call(t.Context(), "id", provider.CallOptions{ProviderOptions: rawOptions(map[string]string{ControlNamespace: `{}`})})
		require.NoError(t, err)
		assert.JSONEq(t, `{}`, string(selector.gateway))
		assert.Nil(t, selector.options.ProviderOptions)
	})

	t.Run("absent controls are nil", func(t *testing.T) {
		selector := &recordingSelector{model: &generateModel{}}
		_, err := newRuntime(t, selector.selector(), testLimits()).Call(t.Context(), "id", provider.CallOptions{ProviderOptions: rawOptions(map[string]string{"anthropic": `{}`})})
		require.NoError(t, err)
		assert.Nil(t, selector.gateway)
	})

	t.Run("other host-like namespaces are ordinary", func(t *testing.T) {
		selector := &recordingSelector{model: &generateModel{}}
		entries := map[string]string{"grafana": `{"a":1}`, "grafana-ai-sdk": `{"b":2}`, "Gateway": `{"c":3}`}
		_, err := newRuntime(t, selector.selector(), testLimits()).Call(t.Context(), "id", provider.CallOptions{ProviderOptions: rawOptions(entries)})
		require.NoError(t, err)
		assert.Nil(t, selector.gateway)
		assert.Equal(t, rawOptions(entries), selector.options.ProviderOptions)
	})

	t.Run("catalog selector refuses them", func(t *testing.T) {
		model := &generateModel{}
		runtime := newRuntime(t, CatalogSelector(staticResolver{}), testLimits())
		_, err := runtime.Call(t.Context(), "alias", provider.CallOptions{ProviderOptions: rawOptions(map[string]string{ControlNamespace: `{}`})})
		require.ErrorIs(t, err, ErrReservedProviderOptions)
		category, reason := ClassifySelection(err)
		assert.Equal(t, CategoryInvalidRequest, category)
		assert.Equal(t, ReasonReservedProviderOptions, reason)
		assert.Zero(t, model.callCount())
	})

	t.Run("protected header wins over controls", func(t *testing.T) {
		selector := &recordingSelector{model: &generateModel{}}
		_, err := newRuntime(t, selector.selector(), testLimits()).Call(t.Context(), "id", provider.CallOptions{
			Headers:         map[string]string{"Authorization": "Bearer caller"},
			ProviderOptions: rawOptions(map[string]string{ControlNamespace: `{}`}),
		})
		require.ErrorIs(t, err, ErrProtectedCallHeader)
		assert.Zero(t, selector.calls.Load())
	})
}
