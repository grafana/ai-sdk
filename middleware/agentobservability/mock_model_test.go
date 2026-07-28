package agentobservability

import (
	"context"
	"regexp"

	"github.com/grafana/ai-sdk/provider"
)

// mockLanguageModel is a hand-written test double for provider.LanguageModel
// used across the middleware tests. Behavior is driven by the optional
// DoGenerateFunc / DoStreamFunc fields; nil fields produce minimal valid
// responses so tests can pick which axes to assert.
type mockLanguageModel struct {
	provider_   string
	modelID     string
	doGenerate  func(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error)
	doStream    func(ctx context.Context, params provider.CallOptions) (*provider.StreamResult, error)
	generateHit int
	streamHit   int
	lastParams  provider.CallOptions
}

func (m *mockLanguageModel) SpecificationVersion() string               { return "v4" }
func (m *mockLanguageModel) Provider() string                           { return m.provider_ }
func (m *mockLanguageModel) ModelID() string                            { return m.modelID }
func (m *mockLanguageModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }

func (m *mockLanguageModel) DoGenerate(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
	m.generateHit++
	m.lastParams = params
	if m.doGenerate != nil {
		return m.doGenerate(ctx, params)
	}
	return &provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{Type: provider.ContentText, Text: "hello"},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
	}, nil
}

func (m *mockLanguageModel) DoStream(ctx context.Context, params provider.CallOptions) (*provider.StreamResult, error) {
	m.streamHit++
	m.lastParams = params
	if m.doStream != nil {
		return m.doStream(ctx, params)
	}
	ch := make(chan provider.StreamPart, 4)
	ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t0"}
	ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t0", Delta: "hello"}
	ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t0"}
	fr := provider.FinishReason{Unified: provider.FinishReasonStop}
	ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &fr}
	close(ch)
	return &provider.StreamResult{Stream: ch}, nil
}

// Compile-time check.
var _ provider.LanguageModel = (*mockLanguageModel)(nil)
