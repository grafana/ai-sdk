// Code generated - EDITING IS FUTILE. DO NOT EDIT.

package v1alpha1

// +k8s:openapi-gen=true
type Spec struct {
	Owner     string `json:"owner"`
	Id        string `json:"id"`
	Created   int64  `json:"created"`
	ExpiresAt int64  `json:"expiresAt"`
	Model     string `json:"model"`
	// JSON contains the sanitized public record, never provider credentials.
	Record string `json:"record"`
}

// NewSpec creates a new Spec object.
func NewSpec() *Spec {
	return &Spec{}
}

// OpenAPIModelName returns the OpenAPI model name for Spec.
func (Spec) OpenAPIModelName() string {
	return "com.github.grafana.ai-sdk.ai-gateway.pkg.internal.storedchatcompletion.v1alpha1.Spec"
}
