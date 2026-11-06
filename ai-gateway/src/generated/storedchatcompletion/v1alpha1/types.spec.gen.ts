// Code generated - EDITING IS FUTILE. DO NOT EDIT.

export interface Spec {
	owner: string;
	id: string;
	created: number;
	expiresAt: number;
	model: string;
	// JSON contains the sanitized public record, never provider credentials.
	record: string;
}

export const defaultSpec = (): Spec => ({
	owner: "",
	id: "",
	created: 0,
	expiresAt: 0,
	model: "",
	record: "",
});

