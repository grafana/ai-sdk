module github.com/grafana/ai-sdk/providers/openai-compatible

go 1.26.3

replace github.com/grafana/ai-sdk => ../../

require (
	github.com/grafana/ai-sdk v0.0.0-20260529150048-3b7c97543d0e
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
