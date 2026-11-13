package tenantconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/k8s"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/extensionhost"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/manifestdata"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

const (
	readerAPIVersion = "aigateway.ext.grafana.app/v1alpha1"
	readerSentinel   = "sentinel-reader-body"
	listPath         = "/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-1/middlewares"
)

func newTestReader(t *testing.T, handler http.HandlerFunc) *Reader {
	t.Helper()
	reader, _ := newLoggedTestReader(t, handler)
	return reader
}

// newLoggedTestReader is newTestReader with the reader's log records.
func newLoggedTestReader(t *testing.T, handler http.HandlerFunc) (*Reader, *bytes.Buffer) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	logs := &bytes.Buffer{}
	reader, err := NewReader(rest.Config{Host: server.URL}, slog.New(slog.NewJSONHandler(logs, nil)))
	require.NoError(t, err)
	return reader, logs
}

func writeList(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}

func TestReader_Read(t *testing.T) {
	for _, tc := range []struct {
		name     string
		pages    map[string]string
		expected []extensionhost.TenantResource
	}{
		{
			name:     "no resources",
			pages:    map[string]string{"": `{"kind":"MiddlewareList","items":[]}`},
			expected: nil,
		},
		{
			name: "two pages sorted by name",
			pages: map[string]string{
				"":   `{"kind":"MiddlewareList","metadata":{"continue":"c1"},"items":[{"metadata":{"name":"model-access"},"spec":{"type":"model-access","enabled":true,"modelAccess":{"note":"x"}}}]}`,
				"c1": `{"kind":"MiddlewareList","items":[{"metadata":{"name":"marker"},"spec":{"type":"marker","enabled":false}}]}`,
			},
			expected: []extensionhost.TenantResource{
				{Name: "marker", Type: "marker"},
				{Name: "model-access", Type: "model-access", Enabled: true, Config: json.RawMessage(`{"note":"x"}`)},
			},
		},
		{
			name:  "nested member with the largest safe integer",
			pages: map[string]string{"": `{"kind":"MiddlewareList","items":[{"metadata":{"name":"model-access"},"spec":{"type":"model-access","enabled":true,"modelAccess":{ "rules" : [{"limits":{"rpm":10},"models":["m"]}], "perModel":{"m":{"maxTokens":9007199254740991}}, "ratio": 1.0 }}}]}`},
			expected: []extensionhost.TenantResource{
				{Name: "model-access", Type: "model-access", Enabled: true, Config: json.RawMessage(`{"perModel":{"m":{"maxTokens":9007199254740991}},"ratio":1,"rules":[{"limits":{"rpm":10},"models":["m"]}]}`)},
			},
		},
		{
			name:     "a list labeled with another kind of the group",
			pages:    map[string]string{"": `{"kind":"ModelCatalogList","apiVersion":"aigateway.ext.grafana.app/v1alpha1","items":[{"kind":"Middleware","apiVersion":"aigateway.ext.grafana.app/v1alpha1","metadata":{"name":"a"},"spec":{"type":"a","enabled":true}}]}`},
			expected: []extensionhost.TenantResource{{Name: "a", Type: "a", Enabled: true}},
		},
		{
			name:     "a list without a kind or item kinds",
			pages:    map[string]string{"": `{"items":[{"metadata":{"name":"a"},"spec":{"type":"a","enabled":false}}]}`},
			expected: []extensionhost.TenantResource{{Name: "a", Type: "a"}},
		},
		{
			name:     "null items",
			pages:    map[string]string{"": `{"kind":"MiddlewareList","items":null}`},
			expected: nil,
		},
		{
			name:  "one invalid resource does not fail the read",
			pages: map[string]string{"": `{"kind":"MiddlewareList","items":[{"metadata":{"name":"b"},"spec":{"type":"b","enabled":true,"other":{}}},{"metadata":{"name":"a"},"spec":{"type":"a","enabled":true}}]}`},
			expected: []extensionhost.TenantResource{
				{Name: "a", Type: "a", Enabled: true},
				{Name: "b", Invalid: true},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var queries []string
			reader := newTestReader(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, listPath, r.URL.Path)
				assert.Equal(t, "100", r.URL.Query().Get("limit"))
				continuation := r.URL.Query().Get("continue")
				queries = append(queries, continuation)
				writeList(w, tc.pages[continuation])
			})
			resources, err := reader.Read(context.Background(), "stacks-1")
			require.NoError(t, err)
			assert.Equal(t, tc.expected, resources)
			assert.Len(t, queries, len(tc.pages))
		})
	}
}

func TestReader_Failures(t *testing.T) {
	status := func(code int) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_, _ = io.WriteString(w, `{"kind":"Status","apiVersion":"v1","status":"Failure","message":"`+readerSentinel+`","code":`+strconv.Itoa(code)+`}`)
		}
	}
	plain := func(code int) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) { http.Error(w, readerSentinel, code) }
	}
	for _, tc := range []struct {
		name     string
		handler  http.HandlerFunc
		expected error
	}{
		{name: "unauthorized", handler: status(http.StatusUnauthorized), expected: extensionhost.ErrReadRefused},
		{name: "forbidden status", handler: status(http.StatusForbidden), expected: extensionhost.ErrReadRefused},
		{name: "forbidden text", handler: plain(http.StatusForbidden), expected: extensionhost.ErrReadRefused},
		{name: "not found", handler: plain(http.StatusNotFound), expected: extensionhost.ErrReadNotFound},
		{name: "server error", handler: status(http.StatusInternalServerError), expected: extensionhost.ErrReadStatus},
		{name: "an item without a name", handler: func(w http.ResponseWriter, _ *http.Request) {
			writeList(w, `{"kind":"MiddlewareList","items":[{"spec":{"type":"a","enabled":true}}]}`)
		}, expected: extensionhost.ErrReadInvalid},
		{name: "malformed", handler: func(w http.ResponseWriter, _ *http.Request) { writeList(w, `{"items":[`+readerSentinel) }, expected: extensionhost.ErrReadInvalid},
		{name: "oversized", handler: func(w http.ResponseWriter, _ *http.Request) {
			writeList(w, `{"kind":"MiddlewareList","items":[],"pad":"`+strings.Repeat("x", maxListBytes)+`"}`)
		}, expected: extensionhost.ErrReadInvalid},
		{name: "success that is not a list: null", handler: func(w http.ResponseWriter, _ *http.Request) { writeList(w, `null`) }, expected: extensionhost.ErrReadInvalid},
		{name: "success that is not a list: empty object", handler: func(w http.ResponseWriter, _ *http.Request) { writeList(w, `{}`) }, expected: extensionhost.ErrReadInvalid},
		{name: "success that is not a list: status", handler: func(w http.ResponseWriter, _ *http.Request) {
			writeList(w, `{"kind":"Status","apiVersion":"v1","status":"Success","message":"`+readerSentinel+`"}`)
		}, expected: extensionhost.ErrReadInvalid},
		{name: "success that is not a list: status with items", handler: func(w http.ResponseWriter, _ *http.Request) {
			writeList(w, `{"kind":"Status","items":[]}`)
		}, expected: extensionhost.ErrReadInvalid},
		{name: "success that is not a list: array", handler: func(w http.ResponseWriter, _ *http.Request) { writeList(w, `[]`) }, expected: extensionhost.ErrReadInvalid},
		{name: "success that is not a list: items not an array", handler: func(w http.ResponseWriter, _ *http.Request) {
			writeList(w, `{"kind":"MiddlewareList","items":{}}`)
		}, expected: extensionhost.ErrReadInvalid},
		{name: "an item of another version", handler: func(w http.ResponseWriter, _ *http.Request) {
			writeList(w, `{"kind":"MiddlewareList","items":[{"apiVersion":"aigateway.ext.grafana.app/v2","metadata":{"name":"a"},"spec":{"type":"a","enabled":true}}]}`)
		}, expected: extensionhost.ErrReadInvalid},
		{name: "endless pages", handler: func(w http.ResponseWriter, _ *http.Request) {
			writeList(w, `{"kind":"MiddlewareList","metadata":{"continue":"again"},"items":[]}`)
		}, expected: extensionhost.ErrReadInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := newTestReader(t, tc.handler)
			resources, err := reader.Read(context.Background(), "stacks-1")
			require.ErrorIs(t, err, tc.expected)
			assert.Nil(t, resources)
			assert.NotContains(t, err.Error(), readerSentinel)
		})
	}

	t.Run("timeout", func(t *testing.T) {
		received := make(chan struct{})
		release := make(chan struct{})
		defer close(release)
		reader := newTestReader(t, func(http.ResponseWriter, *http.Request) {
			close(received)
			<-release
		})
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-received
			cancel()
		}()
		_, err := reader.Read(ctx, "stacks-1")
		require.ErrorIs(t, err, extensionhost.ErrReadTimeout)
	})
}

func TestReader_LogsWhyAResponseIsInvalid(t *testing.T) {
	respond := func(contentType, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			if contentType == "" {
				w.Header()["Content-Type"] = nil
			} else {
				w.Header().Set("Content-Type", contentType)
			}
			_, _ = io.WriteString(w, body)
		}
	}
	for _, tc := range []struct {
		name      string
		handler   http.HandlerFunc
		reason    string
		mediaType string
	}{
		{name: "html", handler: respond("text/html; charset=utf-8; x="+readerSentinel, "<html>"+readerSentinel+"</html>"), reason: "decode", mediaType: "text/html"},
		{name: "kind", handler: respond("application/json;as=Table;g="+readerSentinel, `{"kind":"`+readerSentinel+`","items":[]}`), reason: "kind", mediaType: "application/json"},
		{name: "no items", handler: respond("application/json", `{"kind":"MiddlewareList","metadata":{"name":"`+readerSentinel+`"}}`), reason: "items", mediaType: "application/json"},
		{name: "item", handler: respond("application/json", `{"kind":"MiddlewareList","items":[{"spec":"`+readerSentinel+`"}]}`), reason: "item", mediaType: "application/json"},
		{name: "item of another group", handler: respond("application/json", `{"kind":"MiddlewareList","items":[{"apiVersion":"`+readerSentinel+`/v1","metadata":{"name":"a"}}]}`), reason: "item", mediaType: "application/json"},
		{name: "null body", handler: respond("application/json", `null`), reason: "decode", mediaType: "application/json"},
		{name: "size", handler: respond("application/json", `{"kind":"MiddlewareList","items":[],"pad":"`+strings.Repeat(readerSentinel, maxListBytes/len(readerSentinel)+1)+`"}`), reason: "size", mediaType: "application/json"},
		{name: "pages", handler: respond("application/json", `{"kind":"MiddlewareList","metadata":{"continue":"`+readerSentinel+`"},"items":[]}`), reason: "pages", mediaType: "application/json"},
		{name: "no content type", handler: respond("", readerSentinel), reason: "decode", mediaType: "none"},
		{name: "malformed content type", handler: respond(readerSentinel+"; ;", readerSentinel), reason: "decode", mediaType: "invalid"},
		{name: "long media type", handler: respond("application/"+strings.Repeat("x", maxMediaTypeBytes), readerSentinel), reason: "decode", mediaType: "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader, logs := newLoggedTestReader(t, tc.handler)
			_, err := reader.Read(context.Background(), "stacks-1")
			require.ErrorIs(t, err, extensionhost.ErrReadInvalid)
			var record map[string]any
			require.NoError(t, json.Unmarshal(logs.Bytes(), &record), logs.String())
			assert.Equal(t, "gateway tenant configuration response invalid", record["msg"])
			assert.Equal(t, "stacks-1", record["namespace"])
			assert.Equal(t, tc.reason, record["reason"])
			assert.Equal(t, tc.mediaType, record["content_type"])
			assert.NotContains(t, logs.String(), readerSentinel)
		})
	}

	t.Run("no record for other failures or success", func(t *testing.T) {
		for _, handler := range []http.HandlerFunc{
			func(w http.ResponseWriter, _ *http.Request) { http.Error(w, readerSentinel, http.StatusForbidden) },
			func(w http.ResponseWriter, _ *http.Request) { writeList(w, `{"kind":"MiddlewareList","items":[]}`) },
		} {
			reader, logs := newLoggedTestReader(t, handler)
			_, _ = reader.Read(context.Background(), "stacks-1")
			assert.Empty(t, logs.String())
		}
	})
}

// exchanges records the token exchange requests a test host receives.
type exchanges struct {
	mu       sync.Mutex
	requests []authn.TokenExchangeRequest
}

func (e *exchanges) handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err == nil {
			var request authn.TokenExchangeRequest
			if json.Unmarshal(body, &request) == nil {
				e.mu.Lock()
				e.requests = append(e.requests, request)
				e.mu.Unlock()
			}
		}
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		next.ServeHTTP(w, r)
	})
}

func (e *exchanges) recorded() []authn.TokenExchangeRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]authn.TokenExchangeRequest(nil), e.requests...)
}

func TestReader_ReadsAsThePluginThroughTokenExchange(t *testing.T) {
	manifest := manifestdata.LocalManifest().ManifestData
	kind := app.ManifestVersionKind{Kind: "Middleware", Plural: "middlewares", Scope: "Namespaced"}
	host, err := platformhost.New(platformhost.Config{Topology: platformhost.TopologyCloud, PluginID: "grafana-aigateway-app", Group: manifest.Group, Version: "v1alpha1", Kinds: []app.ManifestVersionKind{kind}})
	require.NoError(t, err)
	recorded := &exchanges{}
	mux := http.NewServeMux()
	mux.Handle("POST /exchange", recorded.handler(host.TokenExchangeHandler()))
	mux.Handle("/apis/", host.Handler(nil))
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	for namespace, spec := range map[string]string{"stacks-1": `{"type":"a","enabled":true,"a":{"limits":{"n":9007199254740991}}}`, "stacks-2": `{"type":"a","enabled":false}`} {
		token, err := host.ServiceToken("seed", namespace)
		require.NoError(t, err)
		request, err := http.NewRequest(http.MethodPost, server.URL+"/apis/"+manifest.Group+"/v1alpha1/namespaces/"+namespace+"/middlewares", strings.NewReader(`{"metadata":{"name":"a"},"spec":`+spec+`}`))
		require.NoError(t, err)
		request.Header.Set("X-Access-Token", token)
		response, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		_ = response.Body.Close()
		require.Equal(t, http.StatusCreated, response.StatusCode)
	}

	kubeConfig, err := k8s.NewTokenExchangeRestConfig(
		k8s.TokenExchangeCredentials{Token: host.AddAccessPolicy("grafana-aigateway-app"), TokenExchangeURL: server.URL + "/exchange"},
		k8s.RemoteServiceTarget{Host: server.URL, Audiences: []string{manifest.Group}},
	)
	require.NoError(t, err)
	reader, err := NewReader(*kubeConfig, slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	first, err := reader.Read(context.Background(), "stacks-1")
	require.NoError(t, err)
	assert.Equal(t, []extensionhost.TenantResource{{Name: "a", Type: "a", Enabled: true, Config: json.RawMessage(`{"limits":{"n":9007199254740991}}`)}}, first)
	second, err := reader.Read(context.Background(), "stacks-2")
	require.NoError(t, err)
	assert.Equal(t, []extensionhost.TenantResource{{Name: "a", Type: "a"}}, second)
	empty, err := reader.Read(context.Background(), "stacks-3")
	require.NoError(t, err)
	assert.Empty(t, empty)

	requests := recorded.recorded()
	require.NotEmpty(t, requests)
	for _, request := range requests {
		assert.Empty(t, request.SubjectToken, "reads without a caller exchange as the plugin")
		assert.Equal(t, "*", request.Namespace)
	}

	t.Run("a caller on the context would be delegated", func(t *testing.T) {
		idToken, err := host.IDToken(types.TypeUser, "u1", "stacks-1")
		require.NoError(t, err)
		ctx := k8s.ContextWithIDToken(context.Background(), idToken)
		before := len(recorded.recorded())
		_, _ = reader.Read(ctx, "stacks-1")
		after := recorded.recorded()
		require.Greater(t, len(after), before)
		assert.NotEmpty(t, after[len(after)-1].SubjectToken, "the cache must therefore never pass a request context")
	})
}

func TestResourceFrom(t *testing.T) {
	const name = `"metadata":{"name":"model-access"}`
	for _, tc := range []struct {
		name     string
		spec     string
		expected extensionhost.TenantResource
	}{
		{name: "member of the type", spec: `{"type":"model-access","enabled":true,"modelAccess":{ "limit" : 9007199254740991 }}`, expected: extensionhost.TenantResource{Name: "model-access", Type: "model-access", Enabled: true, Config: json.RawMessage(`{"limit":9007199254740991}`)}},
		{name: "nested member", spec: `{"type":"model-access","enabled":true,"modelAccess":{"rules":[{"models":["a"],"limits":{"rpm":1}}],"perModel":{"a":{"maxTokens":2}}}}`, expected: extensionhost.TenantResource{Name: "model-access", Type: "model-access", Enabled: true, Config: json.RawMessage(`{"perModel":{"a":{"maxTokens":2}},"rules":[{"limits":{"rpm":1},"models":["a"]}]}`)}},
		{name: "no member", spec: `{"type":"model-access","enabled":false}`, expected: extensionhost.TenantResource{Name: "model-access", Type: "model-access"}},
		{name: "another type's member", spec: `{"type":"model-access","enabled":true,"marker":{}}`, expected: extensionhost.TenantResource{Name: "model-access", Invalid: true}},
		{name: "two members", spec: `{"type":"model-access","enabled":true,"modelAccess":{},"marker":{}}`, expected: extensionhost.TenantResource{Name: "model-access", Invalid: true}},
		{name: "missing enabled", spec: `{"type":"model-access","modelAccess":{}}`, expected: extensionhost.TenantResource{Name: "model-access", Invalid: true}},
		{name: "repeated member", spec: `{"type":"model-access","enabled":true,"modelAccess":{},"modelAccess":{}}`, expected: extensionhost.TenantResource{Name: "model-access", Invalid: true}},
		{name: "no spec", spec: `null`, expected: extensionhost.TenantResource{Name: "model-access", Invalid: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resource, err := resourceFrom(json.RawMessage(`{`+name+`,"spec":`+tc.spec+`}`), readerAPIVersion)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, resource)
		})
	}

	for _, raw := range []string{`{"spec":{"type":"a","enabled":true}}`, `[]`, `{"apiVersion":"aigateway.ext.grafana.app/v2","metadata":{"name":"a"},"spec":{"type":"a","enabled":true}}`, `{"apiVersion":"other.ext.grafana.app/v1alpha1","metadata":{"name":"a"},"spec":{"type":"a","enabled":true}}`} {
		_, err := resourceFrom(json.RawMessage(raw), readerAPIVersion)
		assert.Error(t, err, raw)
	}
}
