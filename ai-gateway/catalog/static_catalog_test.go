package catalog

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	_ ModelResolver          = (*staticCatalog)(nil)
	_ ModelLister            = (*staticCatalog)(nil)
	_ Catalog                = (*staticCatalog)(nil)
	_ provider.LanguageModel = (*catalogTestModel)(nil)
)

type catalogTestModel struct {
	providerName string
	modelID      string
}

func (m *catalogTestModel) SpecificationVersion() string               { return "v4" }
func (m *catalogTestModel) Provider() string                           { return m.providerName }
func (m *catalogTestModel) ModelID() string                            { return m.modelID }
func (m *catalogTestModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }

func (m *catalogTestModel) DoGenerate(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
	return &provider.GenerateResult{}, nil
}

func (m *catalogTestModel) DoStream(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart)
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func TestUnknownModelError(t *testing.T) {
	model := &catalogTestModel{modelID: "native"}
	catalog, err := NewStatic([]StaticEntry{{
		Info:  ModelInfo{ID: "available-secret"},
		Model: model,
	}})
	require.NoError(t, err)

	resolved, err := catalog.ResolveModel(context.Background(), "missing")
	require.Error(t, err)
	assert.Equal(t, ResolvedModel{}, resolved)
	assert.ErrorIs(t, err, ErrUnknownModel)

	var target *UnknownModelError
	require.ErrorAs(t, err, &target)
	assert.Equal(t, "missing", target.ModelID)
	assert.Contains(t, err.Error(), "missing")
	assert.NotContains(t, err.Error(), "available-secret")
}

func TestNewStatic_Validation(t *testing.T) {
	model := &catalogTestModel{modelID: "native"}
	var nilModel *catalogTestModel

	tests := []struct {
		name        string
		entries     []StaticEntry
		errContains string
	}{
		{
			name:        "EmptyCanonicalID",
			entries:     []StaticEntry{{Info: ModelInfo{}, Model: model}},
			errContains: "model ID is required",
		},
		{
			name: "DuplicateCanonicalID",
			entries: []StaticEntry{
				{Info: ModelInfo{ID: "shared"}, Model: model},
				{Info: ModelInfo{ID: "shared"}, Model: model},
			},
			errContains: "shared",
		},
		{
			name:        "EmptyAlias",
			entries:     []StaticEntry{{Info: ModelInfo{ID: "model", Aliases: []string{""}}, Model: model}},
			errContains: "alias is required",
		},
		{
			name: "DuplicateAlias",
			entries: []StaticEntry{
				{Info: ModelInfo{ID: "first", Aliases: []string{"shared"}}, Model: model},
				{Info: ModelInfo{ID: "second", Aliases: []string{"shared"}}, Model: model},
			},
			errContains: "shared",
		},
		{
			name: "AliasCollidesWithLaterCanonicalID",
			entries: []StaticEntry{
				{Info: ModelInfo{ID: "first", Aliases: []string{"second"}}, Model: model},
				{Info: ModelInfo{ID: "second"}, Model: model},
			},
			errContains: "second",
		},
		{
			name:        "AliasCollidesWithOwnCanonicalID",
			entries:     []StaticEntry{{Info: ModelInfo{ID: "model", Aliases: []string{"model"}}, Model: model}},
			errContains: "collides with a model ID",
		},
		{
			name:        "NilModel",
			entries:     []StaticEntry{{Info: ModelInfo{ID: "model"}, Model: nil}},
			errContains: "is nil",
		},
		{
			name:        "TypedNilModel",
			entries:     []StaticEntry{{Info: ModelInfo{ID: "model"}, Model: nilModel}},
			errContains: "is nil",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			catalog, err := NewStatic(tc.entries)
			require.Error(t, err)
			assert.Nil(t, catalog)
			assert.Contains(t, err.Error(), tc.errContains)
		})
	}
}

func TestStaticCatalog_ResolveModel(t *testing.T) {
	model := &catalogTestModel{providerName: "bedrock", modelID: "us.provider.native-v1"}
	catalog, err := NewStatic([]StaticEntry{{
		Info: ModelInfo{
			ID:      "balanced",
			Aliases: []string{"default"},
		},
		Model: model,
	}})
	require.NoError(t, err)

	tests := []struct {
		name    string
		modelID string
	}{
		{name: "CanonicalID", modelID: "balanced"},
		{name: "Alias", modelID: "default"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolved, err := catalog.ResolveModel(context.Background(), tc.modelID)
			require.NoError(t, err)
			assert.Equal(t, "balanced", resolved.ID)
			assert.Same(t, model, resolved.Model)
			assert.Equal(t, "us.provider.native-v1", resolved.Model.ModelID())
		})
	}
}

func TestStaticCatalog_ListModels(t *testing.T) {
	catalog, err := NewStatic([]StaticEntry{
		{
			Info:  ModelInfo{ID: "zeta"},
			Model: &catalogTestModel{modelID: "provider-zeta"},
		},
		{
			Info: ModelInfo{
				ID:           "alpha",
				Name:         "Alpha",
				Description:  "Public route",
				Aliases:      []string{"default"},
				Capabilities: []ModelCapability{"tools", "vision"},
			},
			Model: &catalogTestModel{modelID: "provider-alpha"},
		},
	})
	require.NoError(t, err)

	models, err := catalog.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 2)
	assert.Equal(t, []string{"alpha", "zeta"}, []string{models[0].ID, models[1].ID})
	assert.Equal(t, "Alpha", models[0].Name)
	assert.Equal(t, "Public route", models[0].Description)
	assert.Equal(t, []string{"default"}, models[0].Aliases)
	assert.Equal(t, []ModelCapability{"tools", "vision"}, models[0].Capabilities)
	assert.Empty(t, models[1].Name)
	assert.Empty(t, models[1].Description)
}

func TestStaticCatalog_Empty(t *testing.T) {
	catalog, err := NewStatic(nil)
	require.NoError(t, err)

	models, err := catalog.ListModels(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, models)
	assert.Empty(t, models)

	_, err = catalog.ResolveModel(context.Background(), "missing")
	assert.ErrorIs(t, err, ErrUnknownModel)
}

func TestStaticCatalog_DefensiveCopying(t *testing.T) {
	aliases := []string{"default"}
	capabilities := []ModelCapability{"tools"}
	entries := []StaticEntry{{
		Info: ModelInfo{
			ID:           "balanced",
			Name:         "Balanced",
			Aliases:      aliases,
			Capabilities: capabilities,
		},
		Model: &catalogTestModel{modelID: "native"},
	}}

	catalog, err := NewStatic(entries)
	require.NoError(t, err)

	entries[0].Info.ID = "changed"
	entries[0].Info.Name = "Changed"
	aliases[0] = "changed-alias"
	capabilities[0] = "changed-capability"

	models, err := catalog.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, ModelInfo{
		ID:           "balanced",
		Name:         "Balanced",
		Aliases:      []string{"default"},
		Capabilities: []ModelCapability{"tools"},
	}, models[0])

	models[0].ID = "mutated"
	models[0].Aliases[0] = "mutated-alias"
	models[0].Capabilities[0] = "mutated-capability"
	models = append(models, ModelInfo{ID: "extra"})
	assert.Len(t, models, 2)

	again, err := catalog.ListModels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []ModelInfo{{
		ID:           "balanced",
		Name:         "Balanced",
		Aliases:      []string{"default"},
		Capabilities: []ModelCapability{"tools"},
	}}, again)

	resolved, err := catalog.ResolveModel(context.Background(), "default")
	require.NoError(t, err)
	assert.Equal(t, "balanced", resolved.ID)
}

func TestStaticCatalog_CanceledContext(t *testing.T) {
	catalog, err := NewStatic([]StaticEntry{{
		Info:  ModelInfo{ID: "model"},
		Model: &catalogTestModel{modelID: "native"},
	}})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resolved, err := catalog.ResolveModel(ctx, "model")
	require.NoError(t, err)
	assert.Equal(t, "model", resolved.ID)

	models, err := catalog.ListModels(ctx)
	require.NoError(t, err)
	assert.Equal(t, []ModelInfo{{ID: "model"}}, models)
}

func TestUnknownModelError_Unwrap(t *testing.T) {
	err := &UnknownModelError{ModelID: "requested"}
	assert.True(t, errors.Is(err, ErrUnknownModel))

	var target *UnknownModelError
	require.True(t, errors.As(err, &target))
	assert.Same(t, err, target)
	assert.Equal(t, "requested", target.ModelID)
}
