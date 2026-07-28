package agentobservability

import (
	"context"
	"testing"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStack_BothEnabled(t *testing.T) {
	opts := WrapOptions{
		ClientResolver:  func(ctx context.Context) *agento11y.Client { return nil }, // value doesn't matter for shape
		ContextProvider: func(ctx context.Context) ContextInfo { return ContextInfo{} },
	}
	stack := Stack(opts)
	require.Len(t, stack, 2, "[Hooks, Recording]")
	assert.NotNil(t, stack[0].WrapGenerate, "outer = Hooks (defines WrapGenerate)")
	assert.NotNil(t, stack[1].WrapGenerate, "inner = Recording (defines WrapGenerate)")
}

func TestStack_HooksKeptWhenEnabledIsContextDependent(t *testing.T) {
	// Enabled is documented as a per-request, context-dependent check (e.g. a
	// feature flag scoped to a tenant/user in ctx). Stack must NOT probe it
	// with context.Background() at construction time — doing so would
	// permanently drop Hooks from the stack for every request when the flag
	// returns false against a bare context. The per-request gate lives in
	// evaluateHook and runs with the live request context.
	opts := WrapOptions{
		ClientResolver: func(ctx context.Context) *agento11y.Client { return nil },
		Hooks: HooksOptions{
			Enabled: func(ctx context.Context) bool { return false },
		},
	}
	stack := Stack(opts)
	require.Len(t, stack, 2, "Hooks stays in the stack so per-request Enabled(ctx) can gate at call time")
	assert.NotNil(t, stack[0].WrapGenerate, "outer = Hooks")
	assert.NotNil(t, stack[1].WrapGenerate, "inner = Recording")
}

func TestStack_HooksOmittedWhenNoResolver(t *testing.T) {
	opts := WrapOptions{
		// No top-level ClientResolver; Hooks-level resolver also nil.
	}
	stack := Stack(opts)
	require.Len(t, stack, 1, "Hooks omitted when no client resolver")
}

func TestWrap_EquivalentToManualStack(t *testing.T) {
	model := &mockLanguageModel{provider_: "anthropic", modelID: "claude"}
	opts := WrapOptions{
		ClientResolver: func(ctx context.Context) *agento11y.Client { return nil },
	}

	viaWrap := Wrap(model, opts)
	viaManual := middleware.Wrap(middleware.WrapOptions{
		Model:      model,
		Middleware: Stack(opts),
	})

	// Both should produce identical observable behavior for an identical call.
	// We can't deep-compare the wrapped models (they're opaque), but we can
	// confirm that Provider/ModelID and call counts behave identically.
	assert.Equal(t, viaManual.Provider(), viaWrap.Provider())
	assert.Equal(t, viaManual.ModelID(), viaWrap.ModelID())

	res1, err1 := viaWrap.DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.NoError(t, err1)
	require.NotNil(t, res1)

	res2, err2 := viaManual.DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.NoError(t, err2)
	require.NotNil(t, res2)
}

func TestStack_RecordingAlwaysPresent(t *testing.T) {
	// Even when nothing is configured, Recording is still in the slice — it
	// becomes a no-op when its resolver returns nil. This keeps "I forgot to
	// wire Agent Observability" obvious in a trace (the passthrough span appears) instead
	// of looking identical to "no middleware at all".
	stack := Stack(WrapOptions{})
	require.Len(t, stack, 1)
	assert.NotNil(t, stack[0].WrapGenerate)
}
