package prometheus

import "github.com/grafana/ai-sdk/provider"

const (
	tokenTypeInput           = "input"
	tokenTypeInputNoCache    = "input_no_cache"
	tokenTypeInputCacheRead  = "input_cache_read"
	tokenTypeInputCacheWrite = "input_cache_write"
	tokenTypeOutput          = "output"
	tokenTypeOutputText      = "output_text"
	tokenTypeOutputReasoning = "output_reasoning"
)

func (i *instrumentation) observeUsage(operation string, id identity, usage provider.Usage) {
	i.observeToken(operation, id, tokenTypeInput, usage.InputTokens.Total)
	i.observeToken(operation, id, tokenTypeInputNoCache, usage.InputTokens.NoCache)
	i.observeToken(operation, id, tokenTypeInputCacheRead, usage.InputTokens.CacheRead)
	i.observeToken(operation, id, tokenTypeInputCacheWrite, usage.InputTokens.CacheWrite)
	i.observeToken(operation, id, tokenTypeOutput, usage.OutputTokens.Total)
	i.observeToken(operation, id, tokenTypeOutputText, usage.OutputTokens.Text)
	i.observeToken(operation, id, tokenTypeOutputReasoning, usage.OutputTokens.Reasoning)
}

func (i *instrumentation) observeToken(operation string, id identity, tokenType string, value *int) {
	if value == nil || *value <= 0 {
		return
	}
	i.collectors.tokens.WithLabelValues(operation, id.provider, id.model, tokenType).Add(float64(*value))
}
