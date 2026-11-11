package service

import (
	"log/slog"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

var identityModelFactory ModelFactory = ConfiguredIdentity

// NewModelObservabilityFactories returns factories that build observed models
// directly from the production identities and [ModelObservers], as the runtime
// composes them for configured and BYOK selections.
func NewModelObservabilityFactories(telemetry *Telemetry, logger *slog.Logger, runtime *AgentObservabilityRuntime, drainTimeout time.Duration) (ModelFactory, ModelFactory, error) {
	observers, err := NewModelObservers(telemetry, logger, runtime, drainTimeout)
	if err != nil {
		return nil, nil, err
	}
	observed := func(identity ModelFactory, source execution.Source) ModelFactory {
		return func(id string, lower provider.LanguageModel) (provider.LanguageModel, error) {
			model, err := identity(id, lower)
			if err != nil {
				return nil, err
			}
			return observers.Compose(execution.Selection{ID: id, Model: model, Source: source})
		}
	}
	return observed(ConfiguredIdentity, execution.SourceOperatorCatalog), observed(BYOKIdentity, execution.SourceRequestBYOK), nil
}

func (chain modelObservationChain) wrap(canonicalID string, lower provider.LanguageModel) (provider.LanguageModel, error) {
	identity, err := ConfiguredIdentity(canonicalID, lower)
	if err != nil {
		return nil, err
	}
	return chain.observe(identity), nil
}
