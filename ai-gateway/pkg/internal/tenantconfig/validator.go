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
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/extensionhost"
)

// MaxMemberBytes bounds the canonical JSON encoding of a Middleware's member.
const MaxMemberBytes = 64 << 10

// Validator is the Middleware admission validator. It refuses invalid
// creates and updates with a 400 error whose message never contains a
// configuration value.
type Validator struct {
	tenant map[string]extension.Definition
}

// NewValidator returns a validator for the tenant-scope definitions among
// definitions, which startup has bound to their public tenant types. Other
// scopes are refused like unregistered types.
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
// spec.enabled to false and keeps or removes the stored member, is accepted
// whatever the stored type. Other actions are accepted.
func (v Validator) Validate(_ context.Context, request *app.AdmissionRequest) error {
	if request.Action != resource.AdmissionActionCreate && request.Action != resource.AdmissionActionUpdate {
		return nil
	}
	spec, name, err := decode(request.Object)
	if err != nil {
		return err
	}
	if name != spec.Type {
		return badRequest("middleware: metadata.name must equal spec.type")
	}
	memberName := tenanttype.Type{Name: spec.Type}.Member()
	member, hasMember := spec.Members[memberName]
	if len(spec.Members) > 1 || len(spec.Members) == 1 && !hasMember {
		return badRequest(fmt.Sprintf("middleware: spec may set only spec.%s, the member of spec.type", memberName))
	}
	update := request.Action == resource.AdmissionActionUpdate
	if update {
		stored, ok := request.OldObject.(*Middleware)
		if !ok {
			return badRequest("middleware: the stored object is not a Middleware")
		}
		if !spec.Enabled && (!hasMember || bytes.Equal(member, storedMember(stored, memberName))) {
			return nil
		}
	}
	if len(member) > MaxMemberBytes {
		return badRequest(fmt.Sprintf("middleware: spec.%s exceeds %d bytes", memberName, MaxMemberBytes))
	}
	definition, ok := v.tenant[spec.Type]
	if !ok {
		message := fmt.Sprintf("middleware: spec.type %q is not a tenant extension of this Gateway", spec.Type)
		if update {
			message += fmt.Sprintf("; an update may only set spec.enabled to false and keep or remove spec.%s", memberName)
		}
		return badRequest(message)
	}
	if definition.ConfigSchema.JSON() == nil {
		if hasMember {
			return badRequest(fmt.Sprintf("middleware: spec.type %q takes no configuration member", spec.Type))
		}
		return nil
	}
	if !hasMember {
		if spec.Enabled {
			return badRequest(fmt.Sprintf("middleware: spec.%s is required when spec.enabled is true", memberName))
		}
		return nil
	}
	if err := extensionhost.CheckConfig(definition, member); err != nil {
		return badRequest(fmt.Sprintf("middleware: spec.%s %s", memberName, err))
	}
	return nil
}

// storedMember returns the stored object's member, or nil when it has none or
// its spec can't be decoded.
func storedMember(stored *Middleware, name string) json.RawMessage {
	spec, err := DecodeSpec(stored.RawSpec)
	if err != nil {
		return nil
	}
	return spec.Members[name]
}

func decode(object resource.Object) (Spec, string, error) {
	middleware, ok := object.(*Middleware)
	if !ok {
		return Spec{}, "", badRequest("middleware: the admission object is not a Middleware")
	}
	spec, err := DecodeSpec(middleware.RawSpec)
	if err != nil {
		return Spec{}, "", badRequest(err.Error())
	}
	return spec, middleware.Name, nil
}

func badRequest(message string) error { return apierrors.NewBadRequest(message) }

// noAPIAccessReason is the refusal of every write on a Gateway that cannot
// read what it would store.
const noAPIAccessReason = "middleware: this Gateway cannot read Middleware resources on this host, so stored configuration would not be applied"

// WithoutAPIAccess is the Middleware admission validator of a Gateway the
// host gives no API access. It refuses every create and update, because
// stored configuration would not be applied. Other actions are accepted.
type WithoutAPIAccess struct{}

// Validate refuses creates and updates.
func (WithoutAPIAccess) Validate(_ context.Context, request *app.AdmissionRequest) error {
	if request.Action != resource.AdmissionActionCreate && request.Action != resource.AdmissionActionUpdate {
		return nil
	}
	return badRequest(noAPIAccessReason)
}
