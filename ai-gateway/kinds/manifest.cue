package kinds

manifest: {
	appName:        "aigateway"
	appDisplayName: "AI Gateway"
	versions: {
		"v1alpha1": v1alpha1
	}
	// Public roles grant protocol routes only. Generated kind defaults would
	// otherwise expose stored conversation content outside the owner checks.
	roles: "aigateway:client": {
		title: "AI Gateway Client"
		description: "Use authenticated Gateway protocol routes"
		kinds: []
		routes: ["getConfig", "createLanguageModel", "createChatCompletion"]
	}
	roleBindings: {
		viewer: ["aigateway:client"]
		editor: ["aigateway:client"]
		admin: ["aigateway:client"]
	}
	extraPermissions: {
		accessKinds: []
	}
}

v1alpha1: {
	kinds: [storedChatCompletion & storedChatCompletion.versions.v1alpha1]
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
