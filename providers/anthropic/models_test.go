package anthropic

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveVertexModelID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"claude-sonnet-4", "claude-sonnet-4@20250514"},
		{"claude-sonnet-4-5", "claude-sonnet-4-5@20250929"},
		{"claude-sonnet-4-5-20250929", "claude-sonnet-4-5@20250929"},
		{"claude-sonnet-4-6", "claude-sonnet-4-6"},
		{"claude-3-haiku-20240307", "claude-3-haiku@20240307"},
		{"claude-opus-4-6", "claude-opus-4-6"},
		{"claude-opus-4-7", "claude-opus-4-7"},
		{"claude-opus-4-8", "claude-opus-4-8"},
		{"claude-fable-5", "claude-fable-5"},
		{"claude-sonnet-5", "claude-sonnet-5"},
		{"claude-opus-4-0", "claude-opus-4@20250514"},
		{"claude-3-opus", "claude-3-opus@20240229"},
		{"claude-haiku-4-5", "claude-haiku-4-5@20251001"},
		{"unknown-model", "unknown-model@latest"},
		{"claude-sonnet-4-5@20250929", "claude-sonnet-4-5@20250929"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ResolveVertexModelID(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestModelIDLists_Contracts(t *testing.T) {
	tests := []struct {
		name string
		list func() []string
		want string
	}{
		{"direct", ModelIDs, "claude-opus-4-8"},
		{"vertex", VertexModelIDs, "claude-fable-5"},
		{"dual", DualAvailableModelIDs, "claude-sonnet-5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.list()
			require.NotEmpty(t, got)
			assert.True(t, sort.StringsAreSorted(got))
			assert.Contains(t, got, tt.want)
			assert.Equal(t, got, tt.list())

			original := append([]string(nil), got...)
			got[0] = "mutated"
			assert.Equal(t, original, tt.list())
		})
	}
}

func TestDualAvailableModelIDs_Subsets(t *testing.T) {
	directIDs := stringSet(ModelIDs())
	vertexIDs := stringSet(VertexModelIDs())

	for _, id := range DualAvailableModelIDs() {
		assert.Contains(t, directIDs, id)

		vertexID := ResolveVertexModelID(id)
		assert.Contains(t, vertexIDs, vertexID)
		assert.False(t, strings.HasSuffix(vertexID, "@latest"))
	}
}

func TestModelIDLists_Advisory(t *testing.T) {
	directID := "some-future-direct-model"
	vertexID := "some-future-vertex-model"

	assert.NotContains(t, ModelIDs(), directID)
	assert.NotContains(t, VertexModelIDs(), vertexID)

	directModel := New("test-api-key", directID)

	assert.Equal(t, directID, directModel.ModelID())
	assert.Equal(t, vertexID+"@latest", ResolveVertexModelID(vertexID))
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func TestGetModelCapabilities_AllBranches(t *testing.T) {
	tests := []struct {
		id                   string
		wantMax              int
		wantAdaptiveThinking bool
		wantStructuredOutput bool
		wantRejectsSampling  bool
		wantXHighEffort      bool
		wantKnown            bool
	}{
		{"claude-opus-4-8", 128000, true, true, true, true, true},
		{"claude-opus-4-8@vertex", 128000, true, true, true, true, true},
		{"claude-opus-4-7", 128000, true, true, true, true, true},
		{"claude-opus-4-7@vertex", 128000, true, true, true, true, true},
		{"claude-fable-5", 128000, true, true, true, true, true},
		{"claude-sonnet-5", 128000, true, true, true, true, true},
		{"claude-sonnet-5-20260701", 128000, true, true, true, true, true},
		{"claude-sonnet-4-6", 128000, true, true, false, false, true},
		{"claude-sonnet-4-6-20260115", 128000, true, true, false, false, true},
		{"claude-opus-4-6", 128000, true, true, false, false, true},
		{"claude-opus-4-6@vertex", 128000, true, true, false, false, true},
		{"claude-sonnet-4-5", 64000, false, true, false, false, true},
		{"claude-sonnet-4-5@20250929", 64000, false, true, false, false, true},
		{"claude-opus-4-5", 64000, false, true, false, false, true},
		{"claude-opus-4-5@20251101", 64000, false, true, false, false, true},
		{"claude-haiku-4-5", 64000, false, true, false, false, true},
		{"claude-haiku-4-5@20251001", 64000, false, true, false, false, true},
		{"claude-opus-4-1", 32000, false, true, false, false, true},
		{"claude-opus-4-1@20250805", 32000, false, true, false, false, true},
		{"claude-sonnet-4", 128000, true, true, true, true, false},
		{"claude-sonnet-4@20250514", 128000, true, true, true, true, false},
		{"claude-opus-4", 128000, true, true, true, true, false},
		{"claude-opus-4@20250514", 128000, true, true, true, true, false},
		{"claude-sonnet-4-20250514", 64000, false, false, false, false, true},
		{"claude-sonnet-4-0", 64000, false, false, false, false, true},
		{"claude-3-haiku", 4096, false, false, false, false, true},
		{"claude-3-haiku@20240307", 4096, false, false, false, false, true},
		{"some-future-model", 4096, false, false, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := getModelCapabilities(tt.id)
			assert.Equal(t, tt.wantMax, got.maxOutputTokens)
			assert.Equal(t, tt.wantAdaptiveThinking, got.supportsAdaptiveThinking)
			assert.Equal(t, tt.wantStructuredOutput, got.supportsStructuredOutput)
			assert.Equal(t, tt.wantRejectsSampling, got.rejectsSamplingParams)
			assert.Equal(t, tt.wantXHighEffort, got.supportsXHighEffort)
			assert.Equal(t, tt.wantKnown, got.isKnownModel)
		})
	}
}
