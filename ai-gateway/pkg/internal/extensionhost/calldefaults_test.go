package extensionhost

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/calldefaults"
	"github.com/grafana/ai-sdk/provider"
)

func TestShippedCallDefaults_EveryKeyOwnership(t *testing.T) {
	reader := newFakeReader()
	reader.set("configured",
		stored("default-instructions", true, `{"messages":[{"content":"Be brief."},{"content":"Cite sources."}]}`),
		stored("default-settings", true, `{"temperature":0.25,"stopSequences":["END"]}`),
	)
	reader.set("invalid", stored("default-settings", true, `{"temperature":"not a number"}`))
	h := newTenantHarness(t, reader, TenantCacheConfig{}, calldefaults.Definitions()...)
	user := provider.NewUserMessage(provider.TextPart("hello"))
	configured := provider.CallOptions{
		Prompt:        []provider.Message{provider.NewSystemMessage("Be brief."), provider.NewSystemMessage("Cite sources."), user},
		Temperature:   new(0.25),
		StopSequences: []string{"END"},
	}

	for _, tc := range []struct {
		namespace string
		want      provider.CallOptions
	}{
		{namespace: "configured", want: configured},
		{namespace: "none", want: provider.CallOptions{Prompt: []provider.Message{user}}},
		{namespace: "invalid", want: provider.CallOptions{Prompt: []provider.Message{user}}},
	} {
		for _, source := range []extension.Source{extension.SourceOperatorCatalog, extension.SourceRequestBYOK} {
			for _, stream := range []bool{false, true} {
				model := h.compose(t, source)
				params := provider.CallOptions{Prompt: []provider.Message{user}}
				ctx := inNamespace(t.Context(), tc.namespace)
				if stream {
					result, err := model.DoStream(ctx, params)
					require.NoError(t, err)
					collect(t, result.Stream)
				} else {
					_, err := model.DoGenerate(ctx, params)
					require.NoError(t, err)
				}
				h.model.mu.Lock()
				received := h.model.received[len(h.model.received)-1]
				h.model.mu.Unlock()
				assert.Equal(t, tc.want, received, "%s from %s, stream %t", tc.namespace, source, stream)
			}
		}
	}
	assert.Equal(t, float64(1), h.events("default-settings", "configure", "failed"), "a member that fails the schema at read time is skipped and counted once")
}
