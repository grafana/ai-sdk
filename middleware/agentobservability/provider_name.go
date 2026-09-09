package agentobservability

func agento11yProviderName(name string) string {
	switch name {
	case "amazon-bedrock":
		return "bedrock"
	case "anthropic.vertex":
		return "vertex"
	default:
		return name
	}
}

func otelProviderName(name string) string {
	switch name {
	case "amazon-bedrock", "bedrock", "aws.bedrock":
		return "aws.bedrock"
	case "anthropic.vertex", "vertex", "gcp.vertex_ai":
		return "gcp.vertex_ai"
	default:
		return name
	}
}
