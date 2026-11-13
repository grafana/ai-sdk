// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// +k8s:openapi-gen=true
type Spec struct {
	Type    string `json:"type"`
	Enabled bool   `json:"enabled"`
}

// NewSpec creates a new Spec object.
func NewSpec() *Spec {
	return &Spec{}
}

// OpenAPIModelName returns the OpenAPI model name for Spec.
func (Spec) OpenAPIModelName() string {
	return "com.github.grafana.ai-sdk.ai-gateway.pkg.internal.middleware.v1alpha1.Spec"
}
