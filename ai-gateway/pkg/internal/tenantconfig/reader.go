package tenantconfig

import (
	"bytes"
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

// storedResource is the stored shape of one Middleware resource that
// resourceFrom decodes.
type storedResource struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec struct {
		Type    string          `json:"type"`
		Enabled *bool           `json:"enabled"`
		Config  json.RawMessage `json:"config"`
	} `json:"spec"`
}

// resourceFrom decodes one stored Middleware resource into its name, type,
// enabled flag and configuration. It is the only code that knows how the
// stored spec carries an extension's configuration.
func resourceFrom(raw json.RawMessage) (extensionhost.TenantResource, error) {
	var stored storedResource
	if err := json.Unmarshal(raw, &stored); err != nil {
		return extensionhost.TenantResource{}, err
	}
	return extensionhost.TenantResource{Name: stored.Metadata.Name, Type: stored.Spec.Type, Enabled: stored.Spec.Enabled, Config: normalize(stored.Spec.Config)}, nil
}

// Read lists namespace's resources, following continuation pages within the
// page and byte bounds. Failures are the extensionhost read sentinels.
func (r *Reader) Read(ctx context.Context, namespace string) ([]extensionhost.TenantResource, error) {
	var resources []extensionhost.TenantResource
	remaining := int64(maxListBytes)
	next := ""
	for range maxListPages {
		page, err := r.page(ctx, namespace, next, &remaining)
		if err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			resource, err := resourceFrom(item)
			if err != nil {
				return nil, extensionhost.ErrReadInvalid
			}
			resources = append(resources, resource)
		}
		if page.Metadata.Continue == "" {
			slices.SortFunc(resources, func(a, b extensionhost.TenantResource) int { return strings.Compare(a.Name, b.Name) })
			return resources, nil
		}
		next = page.Metadata.Continue
	}
	return nil, extensionhost.ErrReadInvalid
}

func (r *Reader) page(ctx context.Context, namespace, next string, remaining *int64) (list, error) {
	request := r.client.Get().Namespace(namespace).Resource(r.resource).Param("limit", strconv.Itoa(listPageSize))
	if next != "" {
		request = request.Param("continue", next)
	}
	body, err := request.Stream(ctx)
	if err != nil {
		return list{}, classify(ctx, err)
	}
	defer func() { _ = body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(body, *remaining+1))
	if err != nil {
		return list{}, classify(ctx, err)
	}
	if int64(len(raw)) > *remaining {
		return list{}, extensionhost.ErrReadInvalid
	}
	*remaining -= int64(len(raw))
	var decoded list
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.Kind != listKind {
		return list{}, extensionhost.ErrReadInvalid
	}
	return decoded, nil
}

// normalize returns a stored configuration as compact JSON, or an empty
// object when it is absent or null. Compaction keeps number literals.
func normalize(configuration json.RawMessage) json.RawMessage {
	if len(configuration) == 0 || string(configuration) == "null" {
		return json.RawMessage(`{}`)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, configuration); err != nil {
		return configuration
	}
	return compact.Bytes()
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
