// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// +k8s:openapi-gen=true
type Spec struct {
	// Default system messages for calls that have none.
	DefaultInstructions *V1alpha1SpecDefaultInstructions `json:"defaultInstructions,omitempty"`
	Type                string                           `json:"type"`
	// Default sampling and limit settings for calls that leave them unset.
	DefaultSettings *V1alpha1SpecDefaultSettings `json:"defaultSettings,omitempty"`
	Enabled         bool                         `json:"enabled"`
}

// NewSpec creates a new Spec object.
func NewSpec() *Spec {
	return &Spec{}
}

// OpenAPIModelName returns the OpenAPI model name for Spec.
func (Spec) OpenAPIModelName() string {
	return "com.github.grafana.ai-sdk.ai-gateway.pkg.internal.middleware.v1alpha1.Spec"
}

// +k8s:openapi-gen=true
type V1alpha1SpecDefaultInstructionsMessages struct {
	// The system message text.
	Content string `json:"content"`
}

// NewV1alpha1SpecDefaultInstructionsMessages creates a new V1alpha1SpecDefaultInstructionsMessages object.
func NewV1alpha1SpecDefaultInstructionsMessages() *V1alpha1SpecDefaultInstructionsMessages {
	return &V1alpha1SpecDefaultInstructionsMessages{}
}

// OpenAPIModelName returns the OpenAPI model name for V1alpha1SpecDefaultInstructionsMessages.
func (V1alpha1SpecDefaultInstructionsMessages) OpenAPIModelName() string {
	return "com.github.grafana.ai-sdk.ai-gateway.pkg.internal.middleware.v1alpha1.V1alpha1SpecDefaultInstructionsMessages"
}

// +k8s:openapi-gen=true
type V1alpha1SpecDefaultInstructions struct {
	// System messages prepended, in order, to calls whose prompt has no system message.
	Messages []V1alpha1SpecDefaultInstructionsMessages `json:"messages"`
}

// NewV1alpha1SpecDefaultInstructions creates a new V1alpha1SpecDefaultInstructions object.
func NewV1alpha1SpecDefaultInstructions() *V1alpha1SpecDefaultInstructions {
	return &V1alpha1SpecDefaultInstructions{
		Messages: []V1alpha1SpecDefaultInstructionsMessages{},
	}
}

// OpenAPIModelName returns the OpenAPI model name for V1alpha1SpecDefaultInstructions.
func (V1alpha1SpecDefaultInstructions) OpenAPIModelName() string {
	return "com.github.grafana.ai-sdk.ai-gateway.pkg.internal.middleware.v1alpha1.V1alpha1SpecDefaultInstructions"
}

// +k8s:openapi-gen=true
type V1alpha1SpecDefaultSettings struct {
	// The frequency penalty.
	FrequencyPenalty *float64 `json:"frequencyPenalty,omitempty"`
	// The maximum number of tokens to generate.
	MaxOutputTokens *int64 `json:"maxOutputTokens,omitempty"`
	// The presence penalty.
	PresencePenalty *float64 `json:"presencePenalty,omitempty"`
	// The seed for deterministic sampling, where the model supports it.
	Seed *int64 `json:"seed,omitempty"`
	// Sequences that stop generation.
	StopSequences []string `json:"stopSequences,omitempty"`
	// The sampling temperature.
	Temperature *float64 `json:"temperature,omitempty"`
	// The number of most likely tokens to sample from.
	TopK *int64 `json:"topK,omitempty"`
	// The nucleus sampling probability mass.
	TopP *float64 `json:"topP,omitempty"`
}

// NewV1alpha1SpecDefaultSettings creates a new V1alpha1SpecDefaultSettings object.
func NewV1alpha1SpecDefaultSettings() *V1alpha1SpecDefaultSettings {
	return &V1alpha1SpecDefaultSettings{}
}

// OpenAPIModelName returns the OpenAPI model name for V1alpha1SpecDefaultSettings.
func (V1alpha1SpecDefaultSettings) OpenAPIModelName() string {
	return "com.github.grafana.ai-sdk.ai-gateway.pkg.internal.middleware.v1alpha1.V1alpha1SpecDefaultSettings"
}
