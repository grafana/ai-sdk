package main

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
)

var _ provider.LanguageModel = (*agentLoopModel)(nil)

func TestAgentToolLoop_RejectsCorruptedContinuation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		turn   int
		mutate func([]provider.ContentPart) []provider.ContentPart
	}{
		{"swapped result IDs", 2, func(parts []provider.ContentPart) []provider.ContentPart {
			parts[0].ToolCallID, parts[1].ToolCallID = parts[1].ToolCallID, parts[0].ToolCallID
			return parts
		}},
		{"missing error", 2, func(parts []provider.ContentPart) []provider.ContentPart { return parts[:1] }},
		{"missing recovery", 3, func(parts []provider.ContentPart) []provider.ContentPart { return parts[:0] }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			model := &agentLoopModel{mutate: func(turn int, prompt []provider.Message) []provider.Message {
				if turn != tc.turn {
					return prompt
				}
				altered := slices.Clone(prompt)
				last := &altered[len(altered)-1]
				last.Content = tc.mutate(slices.Clone(last.Content))
				return altered
			}}
			trace, err := runAgentToolLoop(ctx, model)
			require.ErrorContains(t, err, "received wrong prompt")
			require.Len(t, model.requests, tc.turn)
			require.Empty(t, trace.Text)
			if tc.turn == 2 {
				require.Len(t, trace.Executed, 2)
			} else {
				require.Len(t, trace.Executed, 3)
			}
		})
	}
}
