package tenantconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/k8s"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension/tenanttype"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/extensionhost"
	middlewarev1alpha1 "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/middleware/v1alpha1"
)

const (
	// listLimit is the number of resources requested in a namespace's one
	// list. A namespace holds at most one resource per tenant type, far
	// fewer than this.
	listLimit = 100
	// maxListBytes bounds the response bytes read for one namespace.
	maxListBytes = 4 << 20
	// readQPS and readBurst limit the reader's requests per process.
	readQPS   = 20
	readBurst = 40
)

// listKind is the kind of a Middleware list response. A successful response
// of any other kind is invalid, never a namespace without resources.
var listKind = middlewarev1alpha1.Kind().Kind() + "List"

// Reader lists a namespace's Middleware resources through the platform API
// with the credentials of the kubeconfig it was built from. It decodes each
// spec.config as the JSON the platform stores, so numbers keep their literal.
type Reader struct {
	client   rest.Interface
	resource string
}

var _ extensionhost.TenantReader = (*Reader)(nil)

// NewReader returns a reader for the host's API access configuration.
func NewReader(kubeConfig rest.Config) (*Reader, error) {
	gvr := middlewarev1alpha1.Kind().GroupVersionResource()
	groupVersion := gvr.GroupVersion()
	kubeConfig.GroupVersion = &groupVersion
	kubeConfig.APIPath = "/apis"
	kubeConfig.NegotiatedSerializer = &k8s.GenericNegotiatedSerializer{}
	kubeConfig.RateLimiter = nil
	kubeConfig.QPS, kubeConfig.Burst = readQPS, readBurst
	client, err := rest.RESTClientFor(&kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("tenant configuration: building the API client: %w", err)
	}
	return &Reader{client: client, resource: gvr.Resource}, nil
}

type list struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Continue string `json:"continue"`
	} `json:"metadata"`
	Items []json.RawMessage `json:"items"`
}

// storedResource is one listed Middleware resource, with its spec kept raw.
type storedResource struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec json.RawMessage `json:"spec"`
}

// resourceFrom decodes one stored Middleware resource into its name, type,
// enabled flag and configuration: the member named after its type. A spec
// that can't be decoded, or that sets any other member, is an invalid
// resource, so its extension applies its failure policy. It errs only when
// the item is not an object with a name, which makes the whole list invalid.
func resourceFrom(raw json.RawMessage) (extensionhost.TenantResource, error) {
	var stored storedResource
	if err := json.Unmarshal(raw, &stored); err != nil || stored.Metadata.Name == "" {
		return extensionhost.TenantResource{}, errors.New("tenant configuration: a listed item has no name")
	}
	invalid := extensionhost.TenantResource{Name: stored.Metadata.Name, Invalid: true}
	spec, err := DecodeSpec(stored.Spec)
	if err != nil {
		return invalid, nil
	}
	member, found := spec.Members[tenanttype.Type{Name: spec.Type}.Member()]
	if len(spec.Members) > 1 || len(spec.Members) == 1 && !found {
		return invalid, nil
	}
	return extensionhost.TenantResource{Name: stored.Metadata.Name, Type: spec.Type, Enabled: spec.Enabled, Config: member}, nil
}

// Read lists namespace's resources in one request. A response that needs a
// continuation is invalid, since a namespace holds at most one resource per
// tenant type. Failures are the extensionhost read sentinels.
func (r *Reader) Read(ctx context.Context, namespace string) ([]extensionhost.TenantResource, error) {
	body, err := r.client.Get().Namespace(namespace).Resource(r.resource).Param("limit", strconv.Itoa(listLimit)).Stream(ctx)
	if err != nil {
		return nil, classify(ctx, err)
	}
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(body, maxListBytes+1))
	if err != nil {
		return nil, classify(ctx, err)
	}
	var decoded list
	if len(raw) > maxListBytes || json.Unmarshal(raw, &decoded) != nil || decoded.Kind != listKind || decoded.Metadata.Continue != "" {
		return nil, extensionhost.ErrReadInvalid
	}
	resources := make([]extensionhost.TenantResource, 0, len(decoded.Items))
	for _, item := range decoded.Items {
		resource, err := resourceFrom(item)
		if err != nil {
			return nil, extensionhost.ErrReadInvalid
		}
		resources = append(resources, resource)
	}
	slices.SortFunc(resources, func(a, b extensionhost.TenantResource) int { return strings.Compare(a.Name, b.Name) })
	return resources, nil
}

// classify maps a read error to its sentinel, dropping the error's text,
// which can carry the response body.
func classify(ctx context.Context, err error) error {
	var status apierrors.APIStatus
	switch {
	case ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded):
		return extensionhost.ErrReadTimeout
	case apierrors.IsUnauthorized(err), apierrors.IsForbidden(err):
		return extensionhost.ErrReadRefused
	case apierrors.IsNotFound(err):
		return extensionhost.ErrReadNotFound
	case errors.As(err, &status):
		return extensionhost.ErrReadStatus
	default:
		return extensionhost.ErrReadFailed
	}
}
