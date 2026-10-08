package v4

import (
	"context"
	_ "embed"
	"encoding/json"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
)

//go:embed schema/gateway_execution.json
var executionSchemaJSON []byte

func TestExecutionSchema_ProducedCarriers(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, failure := range []bool{false, true} {
			t.Run(map[bool]string{false: "unary", true: "stream"}[streaming]+"/"+map[bool]string{false: "success", true: "error"}[failure], func(t *testing.T) {
				nativeError := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Message: "upstream busy"})
				first := &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					if failure {
						return nil, nativeError
					}
					return validGenerateResult(), nil
				}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					if failure {
						return nil, nativeError
					}
					return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartError, APICallError: nativeError}, finishPart())}, nil
				}}
				h := invocationHarness(t, testLimits(), first)
				request := validRequest(`{"prompt":[]}`)
				if streaming {
					request = streamRequest(`{"prompt":[]}`)
				}
				response := h.serve(request)
				if failure {
					require.NoError(t, compileWireSchema(t, errorSchemaJSON).Validate(response.Body.Bytes()))
					return
				}
				if !streaming {
					require.NoError(t, compileWireSchema(t, unarySuccessSchemaJSON).Validate(response.Body.Bytes()))
					return
				}
				for _, frame := range strings.Split(strings.TrimSuffix(response.Body.String(), "\n\n"), "\n\n") {
					require.NoError(t, compileWireSchema(t, streamEventSchemaJSON).Validate([]byte(strings.TrimPrefix(frame, "data: "))))
				}
			})
		}
	}
}

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
