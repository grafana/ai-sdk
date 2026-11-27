module github.com/grafana/ai-sdk/middleware/logger

go 1.27.3

require (
	github.com/grafana/ai-sdk v0.1.0-alpha.5
	github.com/stretchr/testify v1.12.1
)

require go.yaml.in/yaml/v4 v4.0.0 // indirect

replace github.com/grafana/ai-sdk => ../..
