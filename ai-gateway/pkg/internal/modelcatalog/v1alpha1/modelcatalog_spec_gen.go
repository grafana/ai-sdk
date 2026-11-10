// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// +k8s:openapi-gen=true
type Provider struct {
	Type                      string  `json:"type"`
	BaseURL                   *string `json:"baseURL,omitempty"`
	ProviderName              *string `json:"providerName,omitempty"`
	SupportsStructuredOutputs *bool   `json:"supportsStructuredOutputs,omitempty"`
}

// NewProvider creates a new Provider object.
func NewProvider() *Provider {
	return &Provider{}
}

// OpenAPIModelName returns the OpenAPI model name for Provider.
func (Provider) OpenAPIModelName() string {
	return "com.github.grafana.ai-sdk.ai-gateway.pkg.internal.modelcatalog.v1alpha1.Provider"
}

// +k8s:openapi-gen=true
type Model struct {
	Name        string   `json:"name"`
	Description *string  `json:"description,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	Primary     Target   `json:"primary"`
	Fallback    []Target `json:"fallback,omitempty"`
}

// NewModel creates a new Model object.
func NewModel() *Model {
	return &Model{
		Primary: *NewTarget(),
	}
}

// OpenAPIModelName returns the OpenAPI model name for Model.
func (Model) OpenAPIModelName() string {
	return "com.github.grafana.ai-sdk.ai-gateway.pkg.internal.modelcatalog.v1alpha1.Model"
}

// +k8s:openapi-gen=true
type Target struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// NewTarget creates a new Target object.
func NewTarget() *Target {
	return &Target{}
}

// OpenAPIModelName returns the OpenAPI model name for Target.
func (Target) OpenAPIModelName() string {
	return "com.github.grafana.ai-sdk.ai-gateway.pkg.internal.modelcatalog.v1alpha1.Target"
}

// +k8s:openapi-gen=true
type Spec struct {
	Providers map[string]Provider `json:"providers"`
	Models    map[string]Model    `json:"models"`
}

// NewSpec creates a new Spec object.
func NewSpec() *Spec {
	return &Spec{
		Providers: map[string]Provider{},
		Models:    map[string]Model{},
	}
}

// OpenAPIModelName returns the OpenAPI model name for Spec.
func (Spec) OpenAPIModelName() string {
	return "com.github.grafana.ai-sdk.ai-gateway.pkg.internal.modelcatalog.v1alpha1.Spec"
}
