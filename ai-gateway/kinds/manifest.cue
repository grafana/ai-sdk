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
	roles: {
		"aigateway:catalogreader": {
			title:       "Model catalog reader"
			description: "Read AI Gateway model catalogs"
			kinds: [{kind: "ModelCatalog", permissionSet: "viewer"}]
		}
		"aigateway:catalogwriter": {
			title:       "Model catalog writer"
			description: "Create, update and delete AI Gateway model catalogs"
			kinds: [
				{kind: "ModelCatalog", permissionSet: "editor"},
				{kind: "ModelCatalog/health", verbs: ["get"]},
			]
		}
		"aigateway:modelinvoker": {
			title:       "Model invoker"
			description: "Discover and invoke models from AI Gateway model catalogs"
			kinds: [
				{kind: "ModelCatalog", verbs: ["get"]},
				{kind: "ModelCatalog/language-model", verbs: ["get"]},
				{kind: "ModelCatalog/messages", verbs: ["get"]},
				{kind: "ModelCatalog/config", verbs: ["get"]},
			]
		}
		"aigateway:middlewarereader": {
			title:       "Middleware reader"
			description: "Read AI Gateway tenant extension configuration"
			kinds: [{kind: "Middleware", permissionSet: "viewer"}]
		}
		"aigateway:middlewarewriter": {
			title:       "Middleware writer"
			description: "Create, update and delete AI Gateway tenant extension configuration"
			kinds: [{kind: "Middleware", permissionSet: "editor"}]
		}
	}
	roleBindings: {
		viewer: ["aigateway:catalogreader", "aigateway:middlewarereader"]
		editor: ["aigateway:modelinvoker"]
		admin: ["aigateway:catalogwriter", "aigateway:middlewarewriter"]
	}
}

v1alpha1: {
	kinds: [modelCatalogKind, middlewareKind]
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
		"/namespaces/{namespace}/v1/messages": post: {
			operationId: "createAnthropicMessage"
			responses: default: description: "Anthropic Messages response"
		}
		"/namespaces/{namespace}/language-model": post: {
			operationId: "createLanguageModel"
			responses: default: description: "ProviderWire language-model response"
		}
		"/namespaces/{namespace}/modelcatalogs/{name}/config": get: {
			operationId:                            "getModelCatalogConfig"
			"x-grafana-declared-authz-resource":    "modelcatalogs"
			"x-grafana-declared-authz-subresource": "config"
			"x-grafana-declared-authz-verb":        "get"
			responses: default: description: "ProviderWire discovery response for the catalog"
		}
		"/namespaces/{namespace}/modelcatalogs/{name}/v1/messages": post: {
			operationId:                            "createModelCatalogAnthropicMessage"
			"x-grafana-declared-authz-resource":    "modelcatalogs"
			"x-grafana-declared-authz-subresource": "messages"
			"x-grafana-declared-authz-verb":        "get"
			responses: default: description: "Anthropic Messages response for the catalog"
		}
		"/namespaces/{namespace}/modelcatalogs/{name}/health": post: {
			operationId:                            "checkModelCatalogHealth"
			"x-grafana-declared-authz-resource":    "modelcatalogs"
			"x-grafana-declared-authz-subresource": "health"
			"x-grafana-declared-authz-verb":        "get"
			responses: default: description: "Provider and model listing outcomes for the catalog"
		}
		"/namespaces/{namespace}/modelcatalogs/{name}/language-model": post: {
			operationId:                            "createModelCatalogLanguageModel"
			"x-grafana-declared-authz-resource":    "modelcatalogs"
			"x-grafana-declared-authz-subresource": "language-model"
			"x-grafana-declared-authz-verb":        "get"
			responses: default: description: "ProviderWire language-model response for the catalog"
		}
	}
}
