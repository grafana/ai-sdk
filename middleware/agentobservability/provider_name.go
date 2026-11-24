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

// otelProviderNames mirrors the unexported agento11y otelProviderStoredToWire
// table so the preflight span uses the same gen_ai.provider.name values as
// agento11y's generation span.
var otelProviderNames = map[string]string{
	"gemini":             "gcp.gemini",
	"mistral":            "mistral_ai",
	"moonshotai":         "moonshot_ai",
	"vertex":             "gcp.vertex_ai",
	"bedrock":            "aws.bedrock",
	"azure-openai":       "azure.ai.openai",
	"azure-ai-inference": "azure.ai.inference",
	"watsonx":            "ibm.watsonx.ai",
	"x-ai":               "x_ai",
}

func otelProviderName(name string) string {
	stored := agento11yProviderName(name)
	if wire, ok := otelProviderNames[stored]; ok {
		return wire
	}
	return stored
}
