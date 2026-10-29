// Package streamusage selects the normalized usage reported across provider stream parts.
package streamusage

import (
	"encoding/json"

	"github.com/grafana/ai-sdk/provider"
)

// Aggregator selects the normalized usage of a provider stream. A PartFinish
// closes a response segment: each counter comes from the finish when it
// reports one, and otherwise from the greatest value observed in that
// segment. Once a finish was observed the aggregate is the last closed
// segment's usage, matching upstream streamText, where each model call end
// replaces the step usage; usage after the last finish does not change it.
// Before any finish each counter keeps its greatest observed value. Raw
// follows the same selection with the latest non-empty payload.
type Aggregator struct {
	segment  segmentUsage
	last     segmentUsage
	finished bool
}

type segmentUsage struct {
	usage    provider.Usage
	observed bool
}

// Observe folds a stream part's usage into the aggregate regardless of part type.
func (a *Aggregator) Observe(part provider.StreamPart) {
	if part.Type == provider.PartFinish {
		closed := a.segment
		if part.Usage != nil {
			closed.replace(*part.Usage)
		}
		a.last = closed
		a.finished = true
		a.segment = segmentUsage{}
		return
	}
	if part.Usage != nil {
		a.segment.keepGreatest(*part.Usage)
	}
}

// Usage returns the aggregated usage and whether any usage was observed.
func (a *Aggregator) Usage() (provider.Usage, bool) {
	selected := a.segment
	if a.finished {
		selected = a.last
	}
	if !selected.observed {
		return provider.Usage{}, false
	}
	return cloneUsage(selected.usage), true
}

func (s *segmentUsage) keepGreatest(usage provider.Usage) {
	s.observed = true
	s.usage.InputTokens.Total = maxTokenCount(s.usage.InputTokens.Total, usage.InputTokens.Total)
	s.usage.InputTokens.NoCache = maxTokenCount(s.usage.InputTokens.NoCache, usage.InputTokens.NoCache)
	s.usage.InputTokens.CacheRead = maxTokenCount(s.usage.InputTokens.CacheRead, usage.InputTokens.CacheRead)
	s.usage.InputTokens.CacheWrite = maxTokenCount(s.usage.InputTokens.CacheWrite, usage.InputTokens.CacheWrite)
	s.usage.OutputTokens.Total = maxTokenCount(s.usage.OutputTokens.Total, usage.OutputTokens.Total)
	s.usage.OutputTokens.Text = maxTokenCount(s.usage.OutputTokens.Text, usage.OutputTokens.Text)
	s.usage.OutputTokens.Reasoning = maxTokenCount(s.usage.OutputTokens.Reasoning, usage.OutputTokens.Reasoning)
	if len(usage.Raw) > 0 {
		s.usage.Raw = cloneRaw(usage.Raw)
	}
}

func (s *segmentUsage) replace(usage provider.Usage) {
	s.observed = true
	s.usage.InputTokens.Total = reportedTokenCount(s.usage.InputTokens.Total, usage.InputTokens.Total)
	s.usage.InputTokens.NoCache = reportedTokenCount(s.usage.InputTokens.NoCache, usage.InputTokens.NoCache)
	s.usage.InputTokens.CacheRead = reportedTokenCount(s.usage.InputTokens.CacheRead, usage.InputTokens.CacheRead)
	s.usage.InputTokens.CacheWrite = reportedTokenCount(s.usage.InputTokens.CacheWrite, usage.InputTokens.CacheWrite)
	s.usage.OutputTokens.Total = reportedTokenCount(s.usage.OutputTokens.Total, usage.OutputTokens.Total)
	s.usage.OutputTokens.Text = reportedTokenCount(s.usage.OutputTokens.Text, usage.OutputTokens.Text)
	s.usage.OutputTokens.Reasoning = reportedTokenCount(s.usage.OutputTokens.Reasoning, usage.OutputTokens.Reasoning)
	if len(usage.Raw) > 0 {
		s.usage.Raw = cloneRaw(usage.Raw)
	}
}

func reportedTokenCount(current, reported *int) *int {
	if reported == nil {
		return current
	}
	value := *reported
	return &value
}

func maxTokenCount(current, observed *int) *int {
	if observed == nil {
		return current
	}
	if current != nil && *current >= *observed {
		return current
	}
	value := *observed
	return &value
}

func cloneUsage(usage provider.Usage) provider.Usage {
	return provider.Usage{
		InputTokens: provider.InputTokenUsage{
			Total:      cloneInt(usage.InputTokens.Total),
			NoCache:    cloneInt(usage.InputTokens.NoCache),
			CacheRead:  cloneInt(usage.InputTokens.CacheRead),
			CacheWrite: cloneInt(usage.InputTokens.CacheWrite),
		},
		OutputTokens: provider.OutputTokenUsage{
			Total:     cloneInt(usage.OutputTokens.Total),
			Text:      cloneInt(usage.OutputTokens.Text),
			Reasoning: cloneInt(usage.OutputTokens.Reasoning),
		},
		Raw: cloneRaw(usage.Raw),
	}
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneRaw(value json.RawMessage) json.RawMessage {
	if value == nil {
		return nil
	}
	return append(json.RawMessage(nil), value...)
}
