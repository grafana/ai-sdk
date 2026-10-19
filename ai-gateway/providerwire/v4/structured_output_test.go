package v4

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStructuredOutput_Mapping(t *testing.T) {
	for _, streaming := range []string{"false", "true"} {
		for _, format := range []string{`{"type":"json"}`, `{"type":"json","name":"","description":""}`, `{"type":"json","name":"answer","description":"result","schema":{"type":"object","x-extra":[null,false,"☃",9007199254740993],"properties":{"value":{"type":"string"}}}}`} {
			t.Run(streaming+format, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				request := validRequest(`{"prompt":[],"responseFormat":` + format + `}`)
				request.Header.Set(HeaderStreaming, streaming)
				response := harness.serve(request)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				var expected provider.ResponseFormat
				require.NoError(t, json.Unmarshal([]byte(format), &expected))
				require.Equal(t, &expected, harness.model.receivedOptions().ResponseFormat)
			})
		}
	}
}
func TestStructuredOutput_InvalidBeforeResolution(t *testing.T) {
	for _, streaming := range []string{"false", "true"} {
		for _, format := range []string{`{"type":"json","schema":null}`, `{"type":"json","schema":true}`, `{"type":"json","schema":[]}`, `{"type":"json","schema":1}`, `{"type":"json","schema":"object"}`, `{"type":"json","name":null}`, `{"type":"json","name":2}`, `{"type":"json","description":false}`, `{"type":"json","schema":`} {
			t.Run(streaming+format, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				request := validRequest(`{"prompt":[],"responseFormat":` + format + `}`)
				request.Header.Set(HeaderStreaming, streaming)
				response := harness.serve(request)
				assert.Equal(t, http.StatusBadRequest, response.Code)
				assert.Zero(t, harness.resolver.callCount())
				assert.Zero(t, harness.model.callCount())
			})
		}
	}
}
