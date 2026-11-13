package kinds

// middlewareKind stores one tenant extension's configuration in a stack
// namespace. The object name equals spec.type, so a namespace holds at most
// one resource per extension. Each public tenant type with configuration has
// one optional typed member, written in middleware_members.cue; plugin
// admission, not the manifest, allows only the type's member.
middlewareKind: {
	kind:         "Middleware"
	folderScoped: false
	validation: operations: ["CREATE", "UPDATE"]
	schema: {
		spec: middlewareMembers & {
			type:    string
			enabled: bool
		}
	}
}
