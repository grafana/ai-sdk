package prometheus

import (
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestChoices_GlobalUsageAndBoundedLabels(t *testing.T) {
	zero, one := 0, 1
	stop := provider.FinishReason{Unified: provider.FinishReasonStop}
	length := provider.FinishReason{Unified: provider.FinishReasonLength}
	obs := newStreamObservation(false)
	for _, part := range []provider.StreamPart{
		{Type: provider.PartFinish, ChoiceIndex: &zero, FinishReason: &stop, Usage: &provider.Usage{}},
		{Type: provider.PartFinish, ChoiceIndex: &one, FinishReason: &length, Usage: &provider.Usage{}},
		{Type: provider.PartChoicesFinish, Usage: usagePtr(10, 30)},
	} {
		obs.observe(part, time.Now(), time.Now())
	}
	got, ok := obs.usage.Usage()
	require.True(t, ok)
	assert.Equal(t, 10, *got.InputTokens.Total)
	assert.Equal(t, 30, *got.OutputTokens.Total)
	assert.Equal(t, string(provider.FinishReasonOther), obs.finishReason)
	assert.Equal(t, 1, obs.chunkCounts[string(provider.PartChoicesFinish)])
}
