// Code generated - EDITING IS FUTILE. DO NOT EDIT.

export interface Spec {
	// Default system messages for calls that have none.
	defaultInstructions?: {
		// System messages prepended, in order, to calls whose prompt has no system message.
		messages: {
			// The system message text.
			content: string;
		}[];
	};
	type: string;
	// Default sampling and limit settings for calls that leave them unset.
	defaultSettings?: {
		// The frequency penalty.
		frequencyPenalty?: number;
		// The maximum number of tokens to generate.
		maxOutputTokens?: number;
		// The presence penalty.
		presencePenalty?: number;
		// The seed for deterministic sampling, where the model supports it.
		seed?: number;
		// Sequences that stop generation.
		stopSequences?: string[];
		// The sampling temperature.
		temperature?: number;
		// The number of most likely tokens to sample from.
		topK?: number;
		// The nucleus sampling probability mass.
		topP?: number;
	};
	enabled: boolean;
}

export const defaultSpec = (): Spec => ({
	type: "",
	enabled: false,
});

