package chatcompletions

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestExtendedUsageAcrossSegments(t *testing.T) {
	makeUsage := func() *usage {
		return &usage{Prompt: 10, Completion: 5, Total: 15, PromptDetails: &promptDetails{CacheWrite: ptr(1), Image: ptr(2), Text: ptr(3)}, CompletionDetails: &completionDetails{Text: ptr(4)}}
	}
	total := makeUsage()
	require.NoError(t, addUsage(total, makeUsage()))
	assert.Equal(t, 2, *total.PromptDetails.CacheWrite)
	assert.Equal(t, 4, *total.PromptDetails.Image)
	assert.Equal(t, 6, *total.PromptDetails.Text)
	assert.Equal(t, 8, *total.CompletionDetails.Text)
	missing := makeUsage()
	missing.PromptDetails.Image = nil
	missing.CompletionDetails.Text = nil
	require.NoError(t, addUsage(total, missing))
	assert.Nil(t, total.PromptDetails.Image)
	assert.Nil(t, total.CompletionDetails.Text)
}
