module github.com/grafana/ai-sdk/providers/openai-compatible

go 1.27.3

require (
	github.com/grafana/ai-sdk v0.1.0-alpha.4
	github.com/stretchr/testify v1.12.1
)

require go.yaml.in/yaml/v3 v3.0.5 // indirect

replace github.com/grafana/ai-sdk => ../..
