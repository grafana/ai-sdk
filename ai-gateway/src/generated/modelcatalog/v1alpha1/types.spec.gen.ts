// Code generated - EDITING IS FUTILE. DO NOT EDIT.

export interface Provider {
	type: string;
	baseURL?: string;
	providerName?: string;
	supportsStructuredOutputs?: boolean;
}

export const defaultProvider = (): Provider => ({
	type: "",
});

export interface Model {
	name: string;
	description?: string;
	aliases?: string[];
	primary: Target;
	fallback?: Target[];
}

export const defaultModel = (): Model => ({
	name: "",
	primary: defaultTarget(),
});

export interface Target {
	provider: string;
	model: string;
}

export const defaultTarget = (): Target => ({
	provider: "",
	model: "",
});

export interface Spec {
	providers: Record<string, Provider>;
	models: Record<string, Model>;
}

export const defaultSpec = (): Spec => ({
	providers: {},
	models: {},
});

