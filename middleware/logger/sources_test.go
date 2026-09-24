package logger

import (
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
)

func TestSourceIsFirstContent(t *testing.T) {
	assert.True(t, isFirstContentPart(provider.StreamPart{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "id", URL: "https://example.com"}}))
}
