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
			kinds: [{kind: "ModelCatalog", permissionSet: "editor"}]
		}
		"aigateway:modelinvoker": {
			title:       "Model invoker"
			description: "Discover and invoke models from AI Gateway model catalogs"
			kinds: [
				{kind: "ModelCatalog", verbs: ["get"]},
				{kind: "ModelCatalog/language-model", verbs: ["get"]},
				{kind: "ModelCatalog/config", verbs: ["get"]},
			]
		}
	}
	roleBindings: {
		viewer: ["aigateway:catalogreader"]
		editor: ["aigateway:modelinvoker"]
		admin: ["aigateway:catalogwriter"]
	}
}

v1alpha1: {
	kinds: [modelCatalogKind]
	served: true
	openapi: paths: {
		"/namespaces/{namespace}/config": get: {
			operationId: "getConfig"
			responses: default: description: "ProviderWire discovery response"
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
		"/namespaces/{namespace}/modelcatalogs/{name}/language-model": post: {
			operationId:                            "createModelCatalogLanguageModel"
			"x-grafana-declared-authz-resource":    "modelcatalogs"
			"x-grafana-declared-authz-subresource": "language-model"
			"x-grafana-declared-authz-verb":        "get"
			responses: default: description: "ProviderWire language-model response for the catalog"
		}
	}
}
