package aisdk

import (
	"context"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateText_ToolChoice(t *testing.T) {
	for _, choice := range []*provider.ToolChoice{nil, {Type: provider.ToolChoiceAuto}, {Type: provider.ToolChoiceNone}, {Type: provider.ToolChoiceRequired}, {Type: provider.ToolChoiceTool, ToolName: "lookup"}} {
		name := "default"
		if choice != nil {
			name = string(choice.Type)
		}
		t.Run(name, func(t *testing.T) {
			var got provider.CallOptions
			model := &mockModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				got = opts
				return &provider.StreamResult{Stream: textStreamParts("done")}, nil
			}}
			opts := []GenerateOption{WithModelMessages(provider.UserText("hello"))}
			want := &provider.ToolChoice{Type: provider.ToolChoiceAuto}
			if choice != nil {
				opts = append(opts, WithToolChoice(*choice))
				want = choice
			}
			result, err := GenerateText(t.Context(), model, opts...)
			require.NoError(t, err)
			assert.Equal(t, "done", result.Text)
			assert.Empty(t, got.Tools)
			assert.Equal(t, want, got.ToolChoice)
			assert.Equal(t, 1, model.callCount)
		})
	}
}
