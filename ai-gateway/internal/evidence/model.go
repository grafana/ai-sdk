package evidence

import (
	"context"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
)

type model struct {
	provider.LanguageModel
	candidate        catalog.ConfiguredCandidate
	ordered          bool
	protectedSources []string
}

func Wrap(lower provider.LanguageModel, candidate catalog.ConfiguredCandidate, ordered bool, protectedSources ...string) provider.LanguageModel {
	return model{LanguageModel: lower, candidate: candidate, ordered: ordered, protectedSources: append([]string(nil), protectedSources...)}
}

func (m model) DoGenerate(ctx context.Context, options provider.CallOptions) (result *provider.GenerateResult, err error) {
	s := FromContext(ctx)
	index := s.begin(m.candidate, m.protectedSources...)
	defer func() {
		s.returned(index, result, err, false, m.ordered)
		if !m.ordered && ctx.Err() != nil {
			s.cancel(index)
		}
	}()
	return m.LanguageModel.DoGenerate(ctx, options)
}

func (m model) DoStream(ctx context.Context, options provider.CallOptions) (result *provider.StreamResult, err error) {
	s := FromContext(ctx)
	index := s.begin(m.candidate, m.protectedSources...)
	defer func() {
		s.returned(index, nil, err, true, m.ordered)
		if !m.ordered && ctx.Err() != nil {
			s.cancel(index)
		}
	}()
	return m.LanguageModel.DoStream(ctx, options)
}
