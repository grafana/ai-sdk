package tenantconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
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
	// listPageSize is the number of resources requested per list page.
	listPageSize = 100
	// maxListPages bounds the pages followed for one namespace.
	maxListPages = 64
	// maxListBytes bounds the response bytes read for one namespace.
	maxListBytes = 4 << 20
	// readQPS and readBurst limit the reader's requests per process.
	readQPS   = 20
	readBurst = 40
	// maxMediaTypeBytes bounds the response media type the reader logs.
	maxMediaTypeBytes = 64
)

// invalidReason names why a successful response was invalid.
type invalidReason string

const (
	// invalidDecode is a body that is not a JSON list object.
	invalidDecode invalidReason = "decode"
	// invalidKind is a JSON object whose kind is not the list kind.
	invalidKind invalidReason = "kind"
	// invalidSize is a response beyond the byte bound.
	invalidSize invalidReason = "size"
	// invalidItem is a listed item without a name.
	invalidItem invalidReason = "item"
	// invalidPages is a list that continues beyond the page bound.
	invalidPages invalidReason = "pages"
)

// Media types logged in place of a response's own.
const (
	mediaTypeNone    = "none"
	mediaTypeInvalid = "invalid"
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
	logger   *slog.Logger
}

var _ extensionhost.TenantReader = (*Reader)(nil)

// NewReader returns a reader for the host's API access configuration. It logs
// why a successful response is invalid, with the response's media type and
// never its content.
func NewReader(kubeConfig rest.Config, logger *slog.Logger) (*Reader, error) {
	gvr := middlewarev1alpha1.Kind().GroupVersionResource()
	groupVersion := gvr.GroupVersion()
	kubeConfig.GroupVersion = &groupVersion
	kubeConfig.APIPath = "/apis"
	kubeConfig.NegotiatedSerializer = &k8s.GenericNegotiatedSerializer{}
	kubeConfig.RateLimiter = nil
	kubeConfig.QPS, kubeConfig.Burst = readQPS, readBurst
	kubeConfig.Wrap(func(next http.RoundTripper) http.RoundTripper { return mediaTypeRecorder{next: next} })
	client, err := rest.RESTClientFor(&kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("tenant configuration: building the API client: %w", err)
	}
	return &Reader{client: client, resource: gvr.Resource, logger: logger}, nil
}

type mediaTypeKey struct{}

// mediaTypeRecorder stores each response's media type in the *string its
// request context carries under mediaTypeKey.
type mediaTypeRecorder struct {
	next http.RoundTripper
}

func (m mediaTypeRecorder) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := m.next.RoundTrip(request)
	if recorded, ok := request.Context().Value(mediaTypeKey{}).(*string); ok && err == nil {
		*recorded = mediaTypeOf(response.Header.Get("Content-Type"))
	}
	return response, err
}

// mediaTypeOf returns a Content-Type header's media type without its
// parameters, or a fixed placeholder when it is missing, malformed or too
// long.
func mediaTypeOf(header string) string {
	if header == "" {
		return mediaTypeNone
	}
	mediaType, _, err := mime.ParseMediaType(header)
	if err != nil || len(mediaType) > maxMediaTypeBytes {
		return mediaTypeInvalid
	}
	return mediaType
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

// Read lists namespace's resources, following continuation pages within the
// page and byte bounds. Failures are the extensionhost read sentinels.
func (r *Reader) Read(ctx context.Context, namespace string) ([]extensionhost.TenantResource, error) {
	mediaType := mediaTypeNone
	resources, reason, err := r.read(context.WithValue(ctx, mediaTypeKey{}, &mediaType), namespace)
	if reason != "" {
		r.logger.Warn("gateway tenant configuration response invalid", "namespace", namespace, "reason", string(reason), "content_type", mediaType)
	}
	return resources, err
}

func (r *Reader) read(ctx context.Context, namespace string) ([]extensionhost.TenantResource, invalidReason, error) {
	var resources []extensionhost.TenantResource
	remaining := int64(maxListBytes)
	next := ""
	for range maxListPages {
		page, reason, err := r.page(ctx, namespace, next, &remaining)
		if err != nil {
			return nil, reason, err
		}
		for _, item := range page.Items {
			resource, err := resourceFrom(item)
			if err != nil {
				return nil, invalidItem, extensionhost.ErrReadInvalid
			}
			resources = append(resources, resource)
		}
		if page.Metadata.Continue == "" {
			slices.SortFunc(resources, func(a, b extensionhost.TenantResource) int { return strings.Compare(a.Name, b.Name) })
			return resources, "", nil
		}
		next = page.Metadata.Continue
	}
	return nil, invalidPages, extensionhost.ErrReadInvalid
}

func (r *Reader) page(ctx context.Context, namespace, next string, remaining *int64) (list, invalidReason, error) {
	request := r.client.Get().Namespace(namespace).Resource(r.resource).Param("limit", strconv.Itoa(listPageSize))
	if next != "" {
		request = request.Param("continue", next)
	}
	body, err := request.Stream(ctx)
	if err != nil {
		return list{}, "", classify(ctx, err)
	}
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(body, *remaining+1))
	if err != nil {
		return list{}, "", classify(ctx, err)
	}
	if int64(len(raw)) > *remaining {
		return list{}, invalidSize, extensionhost.ErrReadInvalid
	}
	*remaining -= int64(len(raw))
	var decoded list
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return list{}, invalidDecode, extensionhost.ErrReadInvalid
	}
	if decoded.Kind != listKind {
		return list{}, invalidKind, extensionhost.ErrReadInvalid
	}
	return decoded, "", nil
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
