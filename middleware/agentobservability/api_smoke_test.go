package agentobservability

import (
	"context"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
)

// TestPublicAPISurface_Smoke is a compile-time assertion that every public
// symbol the spec mandates exists with the documented signature. If any of
// these references stop compiling, the spec's normative API surface has
// drifted and the spec needs an explicit update.
//
// Function-signature assertions use Go's typed-function-conversion idiom
// `_ = (func(...) ...)(SymbolName)`, which fails to compile when the
// signature drifts. Type expressions (vs. var declarations) sidestep
// staticcheck's QF1011 "redundant type annotation" hint.
func TestPublicAPISurface_Smoke(t *testing.T) {
	// ---- Types: existence checks via instantiation ----
	_ = WrapOptions{}
	_ = RecordingOptions{}
	_ = HooksOptions{
		MaxLatency: time.Duration(0),
		Enabled:    func(context.Context) bool { return true },
	}
	_ = ContextInfo{
		UserID:       "",
		Metadata:     map[string]any{},
		Tags:         map[string]string{},
		AgentName:    "",
		AgentVersion: "",
	}
	_ = HookDenialError{Reason: "", RuleID: "", Cause: nil}
	var _ *StreamRecorder

	// ---- Type aliases ----
	_ = ClientResolver(func(context.Context) *agento11y.Client { return nil })
	_ = ContextProvider(func(context.Context) ContextInfo { return ContextInfo{} })

	// ---- Functions: full-signature compile-time conversions ----
	_ = (func(RecordingOptions) middleware.Middleware)(RecordingMiddleware)
	_ = (func(HooksOptions) middleware.Middleware)(HooksMiddleware)
	_ = (func(WrapOptions) []middleware.Middleware)(Stack)
	_ = (func(provider.LanguageModel, WrapOptions) provider.LanguageModel)(Wrap)
	_ = (func(context.Context, string, string, ContextInfo) agento11y.GenerationStart)(BuildGenerationStart)
	_ = (func(provider.CallOptions, *provider.GenerateResult, ContextInfo) agento11y.Generation)(MapGenerateResult)
	_ = (func(agento11y.GenerationStart, provider.CallOptions) *StreamRecorder)(NewStreamRecorder)

	// ---- StreamRecorder method set (via method expressions) ----
	_ = (func(*StreamRecorder, provider.StreamPart))((*StreamRecorder).Observe)
	_ = (func(*StreamRecorder) time.Time)((*StreamRecorder).FirstChunkAt)
	_ = (func(*StreamRecorder) agento11y.Generation)((*StreamRecorder).Generation)

	// ---- Context helpers ----
	_ = (func(context.Context, string) context.Context)(WithGenerationID)
	_ = (func(context.Context) string)(GenerationIDFromContext)
	_ = (func() string)(NewGenerationID)
	_ = (func(context.Context, ...string) context.Context)(WithParentGenerationIDs)
	_ = (func(context.Context) []string)(ParentGenerationIDsFromContext)
	_ = (func(context.Context, string) context.Context)(WithLinkedGenerationID)

	// ---- Sentinel error ----
	_ = ErrHookDenied

	assert.True(t, true, "spec-listed public surface compiles")
}
