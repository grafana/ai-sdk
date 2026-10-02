package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scopeKey struct{}

type scopedTestCatalog struct{ scopes map[string]catalog.Catalog }

func (c scopedTestCatalog) ListModels(ctx context.Context) ([]catalog.ModelInfo, error) {
	scope, _ := ctx.Value(scopeKey{}).(string)
	return c.scopes[scope].ListModels(ctx)
}
func (c scopedTestCatalog) ResolveModel(ctx context.Context, id string) (catalog.ResolvedModel, error) {
	scope, _ := ctx.Value(scopeKey{}).(string)
	return c.scopes[scope].ResolveModel(ctx, id)
}

type discoveryOnlyModel struct{ calls atomic.Int32 }

func (*discoveryOnlyModel) SpecificationVersion() string               { return "v4" }
func (*discoveryOnlyModel) Provider() string                           { return "response-provider-not-configured" }
func (*discoveryOnlyModel) ModelID() string                            { return "response-model-not-configured" }
func (*discoveryOnlyModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *discoveryOnlyModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	m.calls.Add(1)
	return nil, assert.AnError
}
func (m *discoveryOnlyModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	m.calls.Add(1)
	return nil, assert.AnError
}

func TestHandler_ScopedListingMatchesResolution(t *testing.T) {
	model := &discoveryOnlyModel{}
	scoped := scopedTestCatalog{scopes: map[string]catalog.Catalog{}}
	for _, scope := range []string{"first", "second"} {
		created, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: scope, Name: "sk-ordinary-" + scope, Aliases: []string{scope + "-alias"}, Candidates: []catalog.ConfiguredCandidate{{ProviderInstance: scope + "-instance", Provider: "anthropic", ModelID: scope + "-native"}}}, Model: model}})
		require.NoError(t, err)
		scoped.scopes[scope] = created
	}
	handler := newTestHandler(t, scoped, 1<<20)
	for _, scope := range []string{"first", "second", "first"} {
		ctx := context.WithValue(context.Background(), scopeKey{}, scope)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil).WithContext(ctx))
		require.Equal(t, 200, response.Code)
		assert.Contains(t, response.Body.String(), scope+"-instance")
		assert.Contains(t, response.Body.String(), "sk-ordinary-"+scope)
		other := "first"
		if scope == other {
			other = "second"
		}
		assert.NotContains(t, response.Body.String(), other+"-instance")
		assert.NotContains(t, response.Body.String(), "response-model-not-configured")
		resolved, err := scoped.ResolveModel(ctx, scope+"-alias")
		require.NoError(t, err)
		assert.Equal(t, scope, resolved.ID)
		_, err = scoped.ResolveModel(ctx, other)
		require.ErrorIs(t, err, catalog.ErrUnknownModel)
	}
	assert.Zero(t, model.calls.Load())
}
