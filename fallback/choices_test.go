package fallback

import (
	"context"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
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
