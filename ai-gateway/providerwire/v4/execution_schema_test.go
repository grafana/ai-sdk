package v4

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
)

//go:embed schema/gateway_execution.json
var executionSchemaJSON []byte

func TestExecutionSchema_ProducedNamespace(t *testing.T) {
	compiled := compileWireSchema(t, executionSchemaJSON)
	for _, outcome := range []fallback.AttemptOutcome{fallback.AttemptSelected, fallback.AttemptFailed, fallback.AttemptCanceled} {
		t.Run(string(outcome), func(t *testing.T) {
			failure := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Data: json.RawMessage(`{"error":{"message":"overloaded","code":900719925474099312345}}`)})
			overview := execution.Project("alias", "public", []fallback.Attempt{
				{Index: 1, Provider: "native", ModelID: "first", Outcome: fallback.AttemptFailed, SourceErr: failure, WillFallback: true},
				{Index: 2, Provider: "native", ModelID: "second", Outcome: outcome},
			}, nil, 4096)
			require.NotNil(t, overview)
			metadata := execution.Metadata(overview, provider.ProviderMetadata{"gateway": json.RawMessage(`{"execution":{"native":true},"null":null}`)}, func(provider.ProviderMetadata) bool { return true })
			require.NoError(t, compiled.Validate(metadata["gateway"]))
		})
	}
}
