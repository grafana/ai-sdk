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
	}
	roleBindings: {
		viewer: ["aigateway:catalogreader"]
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
	}
}
