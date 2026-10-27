module github.com/grafana/ai-sdk/test/workspace

go 1.27.3

replace (
	github.com/grafana/ai-sdk => ../..
	github.com/grafana/ai-sdk/providers/anthropic => ../../providers/anthropic
	github.com/grafana/ai-sdk/providers/azure => ../../providers/azure
	github.com/grafana/ai-sdk/providers/bedrock => ../../providers/bedrock
	github.com/grafana/ai-sdk/providers/grafana => ../../providers/grafana
	github.com/grafana/ai-sdk/providers/openai => ../../providers/openai
	github.com/grafana/ai-sdk/providers/openai-compatible => ../../providers/openai-compatible
	github.com/grafana/ai-sdk/middleware/agentobservability => ../../middleware/agentobservability
	github.com/grafana/ai-sdk/middleware/enrichment => ../../middleware/enrichment
	github.com/grafana/ai-sdk/middleware/logger => ../../middleware/logger
	github.com/grafana/ai-sdk/middleware/prometheus => ../../middleware/prometheus
)
