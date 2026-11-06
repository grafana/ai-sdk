package fallback

import (
	"context"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBufferedChoices_RequiresGlobalFinish(t *testing.T) {
	zero, one := 0, 1
	finish := provider.FinishReason{Unified: provider.FinishReasonStop}
	first := provider.StreamPart{Type: provider.PartFinish, ChoiceIndex: &zero, FinishReason: &finish}
	second := provider.StreamPart{Type: provider.PartFinish, ChoiceIndex: &one, FinishReason: &finish}
	global := provider.StreamPart{Type: provider.PartChoicesFinish, Usage: &provider.Usage{}}
	for _, tc := range []struct {
		name  string
		parts []provider.StreamPart
		want  error
	}{
		{"first choice only", []provider.StreamPart{first}, ErrIncompleteStream},
		{"all choices without total", []provider.StreamPart{first, second}, ErrIncompleteStream},
		{"global before remaining choice", []provider.StreamPart{first, global}, ErrInvalidResult},
		{"duplicate finish", []provider.StreamPart{first, first, global}, ErrInvalidResult},
		{"unindexed content", []provider.StreamPart{{Type: provider.PartTextDelta, Delta: "unscoped"}, first, second, global}, ErrInvalidResult},
		{"content after aggregate", []provider.StreamPart{first, second, global, {Type: provider.PartTextDelta, Delta: "late"}}, ErrInvalidResult},
		{"metadata after aggregate", []provider.StreamPart{first, second, global, {Type: provider.PartResponseMeta}}, ErrInvalidResult},
		{"aggregate with reason", []provider.StreamPart{first, second, {Type: provider.PartChoicesFinish, Usage: &provider.Usage{}, FinishReason: &finish}}, ErrInvalidResult},
		{"trailing raw", []provider.StreamPart{second, first, global, {Type: provider.PartRaw}}, nil},
		{"complete", []provider.StreamPart{second, first, global}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collectStream(context.Background(), bufferedResult(tc.parts...), 100000, 2)
			require.ErrorIs(t, err, tc.want)
			if err == nil {
				assert.Equal(t, tc.parts, got)
			}
		})
	}
}
