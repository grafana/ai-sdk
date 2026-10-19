package kinds

config: {
	definitions: {
		genManifest:      true
		genCRDs:          false
		manifestSchemas:  true
		manifestVersion:  "v1alpha2"
		path:             "src"
		manifestFileName: "app-sdk-manifest.json"
		encoding:         "json"
	}
	codegen: {
		goEnabled: true
		goGenPath: "cmd/grafana-ai-gateway/internal/"
		tsGenPath: "src/generated/"
	}
	kinds: grouping: "kind"
	manifestSelectors: ["manifest"]
}
