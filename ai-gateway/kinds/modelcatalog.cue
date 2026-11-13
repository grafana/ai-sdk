package kinds

// modelCatalogKind stores one stack-configured model catalog. Provider API keys
// are secure values keyed by provider name. The endpoint and compatibility
// fields stay in the schema so that admission refuses them instead of schema
// pruning dropping them.
modelCatalogKind: {
	kind:         "ModelCatalog"
	folderScoped: false
	secure: [{key: "*", description: "Provider API key, keyed by the provider name"}]
	validation: operations: ["CREATE", "UPDATE"]
	schema: {
		#Target: {
			provider: string
			model:    string
		}
		#Provider: {
			type:                       string
			baseURL?:                   string
			providerName?:              string
			supportsStructuredOutputs?: bool
		}
		#Model: {
			name:         string
			description?: string
			aliases?: [...string]
			primary: #Target
			fallback?: [...#Target]
		}
		spec: {
			providers: [string]: #Provider
			models: [string]:    #Model
		}
	}
}
