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
		"/namespaces/{namespace}/language-model": post: {
			operationId: "createLanguageModel"
			responses: default: description: "ProviderWire language-model response"
		}
	}
}
