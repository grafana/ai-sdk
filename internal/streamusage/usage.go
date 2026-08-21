// Package streamusage aggregates normalized usage reported across provider stream parts.
package streamusage

import (
	"encoding/json"

	"github.com/grafana/ai-sdk/internal/ptr"
	"github.com/grafana/ai-sdk/provider"
)

// Aggregator preserves the greatest observed value for each normalized token
// counter. Raw retains the most recently observed non-empty provider payload.
type Aggregator struct {
	usage    provider.Usage
	observed bool
}

// Observe folds a stream part's usage into the aggregate regardless of part type.
func (a *Aggregator) Observe(part provider.StreamPart) {
	if part.Usage == nil {
		return
	}

	usage := part.Usage
	a.observed = true
	a.usage.InputTokens.Total = maxTokenCount(a.usage.InputTokens.Total, usage.InputTokens.Total)
	a.usage.InputTokens.NoCache = maxTokenCount(a.usage.InputTokens.NoCache, usage.InputTokens.NoCache)
	a.usage.InputTokens.CacheRead = maxTokenCount(a.usage.InputTokens.CacheRead, usage.InputTokens.CacheRead)
	a.usage.InputTokens.CacheWrite = maxTokenCount(a.usage.InputTokens.CacheWrite, usage.InputTokens.CacheWrite)
	a.usage.OutputTokens.Total = maxTokenCount(a.usage.OutputTokens.Total, usage.OutputTokens.Total)
	a.usage.OutputTokens.Text = maxTokenCount(a.usage.OutputTokens.Text, usage.OutputTokens.Text)
	a.usage.OutputTokens.Reasoning = maxTokenCount(a.usage.OutputTokens.Reasoning, usage.OutputTokens.Reasoning)
	if len(usage.Raw) > 0 {
		a.usage.Raw = cloneRaw(usage.Raw)
	}
}

// Usage returns the aggregated usage and whether any usage was observed.
func (a *Aggregator) Usage() (provider.Usage, bool) {
	if !a.observed {
		return provider.Usage{}, false
	}
	return cloneUsage(a.usage), true
}

func maxTokenCount(current, observed *int) *int {
	if observed == nil {
		return current
	}
	if current != nil && *current >= *observed {
		return current
	}
	return ptr.Clone(observed)
}

func cloneUsage(usage provider.Usage) provider.Usage {
	return provider.Usage{
		InputTokens: provider.InputTokenUsage{
			Total:      ptr.Clone(usage.InputTokens.Total),
			NoCache:    ptr.Clone(usage.InputTokens.NoCache),
			CacheRead:  ptr.Clone(usage.InputTokens.CacheRead),
			CacheWrite: ptr.Clone(usage.InputTokens.CacheWrite),
		},
		OutputTokens: provider.OutputTokenUsage{
			Total:     ptr.Clone(usage.OutputTokens.Total),
			Text:      ptr.Clone(usage.OutputTokens.Text),
			Reasoning: ptr.Clone(usage.OutputTokens.Reasoning),
		},
		Raw: cloneRaw(usage.Raw),
	}
}

func cloneRaw(value json.RawMessage) json.RawMessage {
	if value == nil {
		return nil
	}
	return append(json.RawMessage(nil), value...)
}
