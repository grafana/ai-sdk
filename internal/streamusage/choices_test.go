package streamusage

import (
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChoicesUsage_GlobalFinishOnly(t *testing.T) {
	zero, one := 0, 1
	aggregate := observeAll(provider.StreamPart{Type: provider.PartFinish, ChoiceIndex: &zero, Usage: tokens(99, 99)}, provider.StreamPart{Type: provider.PartFinish, ChoiceIndex: &one, Usage: &provider.Usage{}})
	_, ok := aggregate.Usage()
	assert.False(t, ok)
	aggregate.Observe(provider.StreamPart{Type: provider.PartChoicesFinish, Usage: tokens(10, 30)})
	aggregate.Observe(provider.StreamPart{Type: provider.PartRaw, Usage: tokens(90, 90)})
	usage, ok := aggregate.Usage()
	require.True(t, ok)
	assert.Equal(t, 10, *usage.InputTokens.Total)
	assert.Equal(t, 30, *usage.OutputTokens.Total)
}
