package bedrock

import (
	"testing"

	"github.com/grafana/ai-sdk/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider_ImplementsRegistry(t *testing.T) {
	var _ registry.Provider = (*Provider)(nil)
}

func TestProvider_LanguageModel(t *testing.T) {
	p := NewProvider(WithRegion("eu-west-1"))
	m, err := p.LanguageModel("anthropic.claude-sonnet-4-5-20250929-v1:0")
	require.NoError(t, err)
	assert.Equal(t, "amazon-bedrock", m.Provider())
	assert.Equal(t, "anthropic.claude-sonnet-4-5-20250929-v1:0", m.ModelID())
}

func TestProvider_OptionsAppliedToEachModel(t *testing.T) {
	p := NewProvider(WithRegion("eu-west-1"), WithBearerToken("tok"))
	a, _ := p.LanguageModel("m1")
	b, _ := p.LanguageModel("m2")
	am := a.(*model)
	bm := b.(*model)
	assert.Equal(t, "eu-west-1", am.region)
	assert.Equal(t, "eu-west-1", bm.region)
	assert.Equal(t, "tok", am.bearerToken)
	assert.Equal(t, "tok", bm.bearerToken)
	assert.NotSame(t, am, bm, "each call returns a fresh model")
}
