package kinds

manifest: {
	appName:        "aigateway"
	appDisplayName: "AI Gateway"
	versions: {
		"v1alpha1": v1alpha1
	}
	extraPermissions: {
		accessKinds: []
	}
}

v1alpha1: {
	kinds: []
	served: true
	openapi: paths: {
		"/namespaces/{namespace}/config": get: {
			operationId: "getConfig"
			responses: default: description: "ProviderWire discovery response"
		}
		"/namespaces/{namespace}/v1/chat/completions": post: {
			operationId: "createChatCompletion"
			responses: default: description: "OpenAI Chat Completions response"
		}
		"/namespaces/{namespace}/language-model": post: {
			operationId: "createLanguageModel"
			responses: default: description: "ProviderWire language-model response"
		}
	}
}
