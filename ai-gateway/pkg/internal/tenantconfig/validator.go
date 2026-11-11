package tenantconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/resource"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/extensionhost"
	middlewarev1alpha1 "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/middleware/v1alpha1"
)

// MaxConfigBytes bounds the JSON encoding of a Middleware's spec.config.
const MaxConfigBytes = 64 << 10

// Validator is the Middleware admission validator. It refuses invalid
// creates and updates with a 400 error whose message never contains a
// configuration value.
type Validator struct {
	tenant map[string]extension.Definition
}

// NewValidator returns a validator for the tenant-scope definitions among
// definitions. Other scopes are refused like unregistered types.
func NewValidator(definitions []extension.Definition) Validator {
	tenant := map[string]extension.Definition{}
	for _, definition := range definitions {
		if definition.Scope == extension.ScopeTenant {
			tenant[definition.Name] = definition
		}
	}
	return Validator{tenant: tenant}
}

// Validate checks creates and updates. A disable-only update, which sets
// spec.enabled to false and keeps the stored spec.config, is accepted whatever
// the stored type. Other actions are accepted.
func (v Validator) Validate(_ context.Context, request *app.AdmissionRequest) error {
	if request.Action != resource.AdmissionActionCreate && request.Action != resource.AdmissionActionUpdate {
		return nil
	}
	middleware, ok := request.Object.(*middlewarev1alpha1.Middleware)
	if !ok {
		return apierrors.NewBadRequest("middleware: the admission object is not a Middleware")
	}
	if middleware.Name != middleware.Spec.Type {
		return apierrors.NewBadRequest("middleware: metadata.name must equal spec.type")
	}
	configuration, err := encode(middleware.Spec.Config)
	if err != nil {
		return apierrors.NewBadRequest("middleware: spec.config is not a JSON object")
	}
	update := request.Action == resource.AdmissionActionUpdate
	if update {
		stored, ok := request.OldObject.(*middlewarev1alpha1.Middleware)
		if !ok {
			return apierrors.NewBadRequest("middleware: the stored object is not a Middleware")
		}
		previous, err := encode(stored.Spec.Config)
		if err == nil && !middleware.Spec.Enabled && bytes.Equal(configuration, previous) {
			return nil
		}
	}
	if len(configuration) > MaxConfigBytes {
		return apierrors.NewBadRequest(fmt.Sprintf("middleware: spec.config exceeds %d bytes", MaxConfigBytes))
	}
	definition, ok := v.tenant[middleware.Spec.Type]
	if !ok {
		message := fmt.Sprintf("middleware: spec.type %q is not a tenant extension of this Gateway", middleware.Spec.Type)
		if update {
			message += "; an update may only set spec.enabled to false and keep spec.config"
		}
		return apierrors.NewBadRequest(message)
	}
	if err := extensionhost.CheckConfig(definition, configuration); err != nil {
		return apierrors.NewBadRequest("middleware: spec.config " + err.Error())
	}
	return nil
}

func encode(configuration map[string]any) (json.RawMessage, error) {
	if configuration == nil {
		configuration = map[string]any{}
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(configuration); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(encoded.Bytes(), []byte("\n")), nil
}
