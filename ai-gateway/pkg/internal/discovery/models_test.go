package discovery

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/openai/chatcompletions"
	modelsapi "github.com/grafana/ai-sdk/ai-gateway/openai/models"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOfficialClientModelDiscovery(t *testing.T) {
	lister := &testLister{models: []catalog.ModelInfo{{ID: "public/model", Aliases: []string{"friendly", "public/model"}, Candidates: []catalog.ConfiguredCandidate{{ProviderInstance: "private-account", ModelID: "private-native"}}}}}
	mux := http.NewServeMux()
	mux.Handle("GET /v1/models", withAccess(gatewayauth.ConfiguredAccounts, New(lister, modelsapi.Projection{}, chatcompletions.NewHostErrorWriter())))
	mux.Handle("GET /v1/models/{model}", withAccess(gatewayauth.ConfiguredAccounts, NewResource(lister, modelsapi.Projection{}, chatcompletions.NewHostErrorWriter())))
	server := httptest.NewServer(mux)
	defer server.Close()
	client := openai.NewClient(option.WithBaseURL(server.URL+"/v1"), option.WithAPIKey("dummy"), option.WithMaxRetries(0))
	page, err := client.Models.List(t.Context())
	require.NoError(t, err)
	require.Len(t, page.Data, 2)
	assert.Equal(t, "friendly", page.Data[0].ID)
	assert.Equal(t, "public/model", page.Data[1].ID)
	for _, id := range []string{"friendly", "public/model"} {
		model, err := client.Models.Get(t.Context(), id)
		require.NoError(t, err)
		assert.Equal(t, id, model.ID)
		assert.Equal(t, "grafana", model.OwnedBy)
		assert.Zero(t, model.Created)
		assert.NotContains(t, model.RawJSON(), "private-")
	}
	_, err = client.Models.Get(t.Context(), "private-native")
	var apiError *openai.Error
	require.ErrorAs(t, err, &apiError)
	assert.Equal(t, 404, apiError.StatusCode)
	assert.Equal(t, "model_not_found", apiError.Code)
}

func TestModelLookupUsesDiscoveryAccessAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		access gatewayauth.AccountAccess
		fail   bool
		status int
		calls  int32
	}{
		{"configured", gatewayauth.ConfiguredAccounts, false, 404, 1},
		{"BYOK", gatewayauth.RequestBYOK, false, 400, 0},
		{"denied", "", false, 403, 0},
		{"list failure", gatewayauth.ConfiguredAccounts, true, 500, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lister := &testLister{}
			if tc.fail {
				lister.err = errors.New("private failure")
			}
			request := httptest.NewRequest("GET", "/v1/models/private", nil)
			request.SetPathValue("model", "private")
			response := httptest.NewRecorder()
			withAccess(tc.access, NewResource(lister, modelsapi.Projection{}, chatcompletions.NewHostErrorWriter())).ServeHTTP(response, request)
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, tc.calls, lister.calls.Load())
			assert.NotContains(t, response.Body.String(), "private")
		})
	}
}
