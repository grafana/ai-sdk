package kinds

// middlewareKind stores one tenant extension's configuration in a stack
// namespace. The object name equals spec.type, so a namespace holds at most
// one resource per extension. config is an open object so that admission, not
// schema pruning, decides what an extension accepts; the platform still
// prunes objects nested inside it.
middlewareKind: {
	kind:         "Middleware"
	folderScoped: false
	validation: operations: ["CREATE", "UPDATE"]
	schema: {
		spec: {
			type:    string
			enabled: bool
			config?: {[string]: _}
		}
	}
}
