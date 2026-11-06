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
		routes: ["getConfig", "createLanguageModel", "createChatCompletion", "listChatCompletions", "getChatCompletion", "updateChatCompletion", "deleteChatCompletion", "getChatCompletionMessages"]
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
		"/namespaces/{namespace}/v1/chat/completions": get: {
			operationId: "listChatCompletions"
			responses: default: description: "Stored Chat Completions list"
		}
		"/namespaces/{namespace}/v1/chat/completions": post: {
			operationId: "createChatCompletion"
			responses: default: description: "OpenAI Chat Completions response"
		}
		"/namespaces/{namespace}/v1/chat/completions/{completion_id}": {
			get: {
				operationId: "getChatCompletion"
				responses: default: description: "Stored Chat Completion"
			}
			post: {
				operationId: "updateChatCompletion"
				responses: default: description: "Updated metadata"
			}
			delete: {
				operationId: "deleteChatCompletion"
				responses: default: description: "Deleted Chat Completion"
			}
		}
		"/namespaces/{namespace}/v1/chat/completions/{completion_id}/messages": get: {
			operationId: "getChatCompletionMessages"
			responses: default: description: "Stored input messages"
		}
		"/namespaces/{namespace}/language-model": post: {
			operationId: "createLanguageModel"
			responses: default: description: "ProviderWire language-model response"
		}
	}
}
