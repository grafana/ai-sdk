package discovery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ConfiguredProjection(t *testing.T) {
	var infos []catalog.ModelInfo
	require.NoError(t, json.Unmarshal([]byte(`[{"ID":"public","Name":"sk-ordinary-display","Aliases":["z-alias","a-alias"],"Candidates":[{"ProviderInstance":"primary","Provider":"anthropic","ModelID":"native-primary"},{"ProviderInstance":"backup","Provider":"openai","ModelID":"native-backup"}]}]`), &infos))
	response := httptest.NewRecorder()
	newTestHandler(t, &testLister{models: infos}, 1<<20).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil))
	require.Equal(t, http.StatusOK, response.Code)
	var document struct {
		Models []struct {
			ID            string          `json:"id"`
			Gateway       json.RawMessage `json:"gateway"`
			Specification specification   `json:"specification"`
		} `json:"models"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &document))
	require.Len(t, document.Models, 3)
	for i, id := range []string{"a-alias", "public", "z-alias"} {
		row := document.Models[i]
		assert.Equal(t, id, row.ID)
		assert.Equal(t, id, row.Specification.ModelID)
		assert.Equal(t, "grafana", row.Specification.Provider)
		require.NotEmpty(t, row.Gateway)
		assert.JSONEq(t, `{"canonicalModelId":"public","aliases":["z-alias","a-alias"],"candidates":[{"providerInstance":"primary","provider":"anthropic","modelId":"native-primary"},{"providerInstance":"backup","provider":"openai","modelId":"native-backup"}]}`, string(row.Gateway))
	}
	assert.Contains(t, response.Body.String(), "sk-ordinary-display")
}
