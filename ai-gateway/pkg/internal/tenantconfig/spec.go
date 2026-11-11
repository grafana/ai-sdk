package tenantconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"slices"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/grafana/grafana-app-sdk/resource"

	middlewarev1alpha1 "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/middleware/v1alpha1"
)

const (
	specType    = "type"
	specEnabled = "enabled"
)

// errInvalidSpec reports a spec that is not a Middleware spec. It never
// describes the content, which may hold configuration values or keys.
var errInvalidSpec = errors.New("middleware: spec is not a valid Middleware spec")

// Spec is a decoded Middleware spec.
type Spec struct {
	Type    string
	Enabled bool
	// Members holds every spec field other than type and enabled, keyed by
	// name, each in canonical JSON: compact, with object keys sorted, numbers
	// as written and no HTML escaping. Equal configurations encode equally.
	Members map[string]json.RawMessage
}

// DecodeSpec decodes a Middleware spec without decoding its members into Go
// types, so numbers keep their exact value. Errors never contain spec
// content.
func DecodeSpec(raw json.RawMessage) (Spec, error) {
	fields, err := decodeObject(raw)
	if err != nil {
		return Spec{}, errInvalidSpec
	}
	var spec Spec
	if json.Unmarshal(fields[specType], &spec.Type) != nil || spec.Type == "" {
		return Spec{}, errInvalidSpec
	}
	if json.Unmarshal(fields[specEnabled], &spec.Enabled) != nil {
		return Spec{}, errInvalidSpec
	}
	spec.Members = map[string]json.RawMessage{}
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		if name == specType || name == specEnabled {
			continue
		}
		member, err := canonical(fields[name])
		if err != nil {
			return Spec{}, errInvalidSpec
		}
		spec.Members[name] = member
	}
	return spec, nil
}

// decodeObject decodes a JSON object's fields and refuses repeated names,
// which JSON decoding would otherwise resolve silently.
func decodeObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, errInvalidSpec
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, _ := token.(string)
		if _, repeated := fields[name]; repeated {
			return nil, errInvalidSpec
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err == nil {
		return nil, errInvalidSpec
	}
	return fields, nil
}

func canonical(raw json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(encoded.Bytes(), []byte("\n")), nil
}

// Middleware is the Middleware object admission reviews: the generated
// object's metadata and methods, with the spec kept as raw JSON. Its decoder
// never decodes the spec, so a spec that doesn't match the generated types
// can't produce an error that echoes its values.
type Middleware struct {
	middlewarev1alpha1.Middleware
	RawSpec json.RawMessage
}

// UnmarshalJSON decodes the object's type and metadata and keeps its spec.
func (m *Middleware) UnmarshalJSON(data []byte) error {
	var object struct {
		APIVersion string          `json:"apiVersion"`
		Kind       string          `json:"kind"`
		Metadata   json.RawMessage `json:"metadata"`
		Spec       json.RawMessage `json:"spec"`
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return errors.New("middleware: the object is not a JSON object")
	}
	m.Middleware = middlewarev1alpha1.Middleware{}
	m.APIVersion, m.Kind = object.APIVersion, object.Kind
	if object.Metadata != nil {
		if err := json.Unmarshal(object.Metadata, &m.ObjectMeta); err != nil {
			return errors.New("middleware: the object's metadata is not valid")
		}
	}
	m.RawSpec = object.Spec
	return nil
}

// Copy returns a deep copy.
func (m *Middleware) Copy() resource.Object {
	return &Middleware{Middleware: *m.DeepCopy(), RawSpec: bytes.Clone(m.RawSpec)}
}

// DeepCopyObject returns a deep copy.
func (m *Middleware) DeepCopyObject() runtime.Object { return m.Copy() }

var kind = resource.Kind{
	Schema: resource.NewSimpleSchema(middlewarev1alpha1.Schema().Group(), middlewarev1alpha1.Schema().Version(), &Middleware{}, &middlewarev1alpha1.MiddlewareList{},
		resource.WithKind(middlewarev1alpha1.Schema().Kind()), resource.WithPlural(middlewarev1alpha1.Schema().Plural()), resource.WithScope(middlewarev1alpha1.Schema().Scope())),
	Codecs: map[resource.KindEncoding]resource.Codec{resource.KindEncodingJSON: &middlewarev1alpha1.JSONCodec{}},
}

// Kind returns the Middleware kind admission manages: the generated kind's
// schema with [Middleware] objects.
func Kind() resource.Kind { return kind }
