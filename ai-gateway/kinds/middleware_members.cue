package kinds

// middlewareMembers holds one optional Middleware spec member per public
// tenant type with a configuration schema, named after the type as
// tenanttype.Type.Member returns it. Each member mirrors the type's JSON
// Schema structurally: the same properties, JSON types, array items,
// required fields and descriptions, with every object closed. It declares
// no value constraints, such as enums, patterns or bounds, and no defaults;
// admission checks those against the type's schema. A manifestdata test
// fails when a member and its type disagree.
middlewareMembers: {
	// Default system messages for calls that have none.
	defaultInstructions?: {
		// System messages prepended, in order, to calls whose prompt has no system message.
		messages: [...{
			// The system message text.
			content: string
		}]
	}
	// Default sampling and limit settings for calls that leave them unset.
	defaultSettings?: {
		// The frequency penalty.
		frequencyPenalty?: number
		// The maximum number of tokens to generate.
		maxOutputTokens?: int
		// The presence penalty.
		presencePenalty?: number
		// The seed for deterministic sampling, where the model supports it.
		seed?: int
		// Sequences that stop generation.
		stopSequences?: [...string]
		// The sampling temperature.
		temperature?: number
		// The number of most likely tokens to sample from.
		topK?: int
		// The nucleus sampling probability mass.
		topP?: number
	}
}
