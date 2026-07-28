package bedrock

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModelIDs(t *testing.T) {
	ids := ModelIDs()
	assert.Len(t, ids, 78)
	assert.Contains(t, ids, "anthropic.claude-opus-4-8")
	assert.Contains(t, ids, "anthropic.claude-fable-5")
	assert.Contains(t, ids, "openai.gpt-oss-120b-1:0")
	assert.Contains(t, ids, "us.amazon.nova-premier-v1:0")
	assert.Contains(t, ids, "us.meta.llama4-maverick-17b-instruct-v1:0")
	assert.True(t, sort.StringsAreSorted(ids))

	original := append([]string(nil), ids...)
	ids[0] = "mutated"
	assert.Equal(t, original, ModelIDs())
}
