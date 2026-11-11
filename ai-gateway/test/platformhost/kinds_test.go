package platformhost

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana-app-sdk/app"
	"github.com/grafana/grafana-app-sdk/plugin/appadapter"
	pluginv3 "github.com/grafana/grafana-app-sdk/plugin/genproto/grafana/plugin/v3"
	"github.com/grafana/grafana-app-sdk/plugin/grpcplugin"
)

const widgetKey = "sentinel-widget-key"

type review struct {
	operation pluginv3.AdmissionReviewRequest_Operation
	object    string
	old       string
}

type stubAdmission struct {
	pluginv3.UnimplementedAdmissionServiceServer
	mu      sync.Mutex
	reviews []review
	deny    bool
}

func (s *stubAdmission) AdmissionReview(_ context.Context, request *pluginv3.AdmissionReviewRequest) (*pluginv3.AdmissionReviewResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reviews = append(s.reviews, review{operation: request.GetOperation(), object: string(request.GetObjectBytes()), old: string(request.GetOldObjectBytes())})
	response := &pluginv3.AdmissionReviewResponse{}
	if !s.deny {
		response.SetAllowed(true)
		return response, nil
	}
	status := &pluginv3.StatusResult{}
	status.SetCode(http.StatusBadRequest)
	status.SetMessage("widget refused")
	response.SetError(status)
	return response, nil
}

func (s *stubAdmission) recorded() []review {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]review(nil), s.reviews...)
}

func newKindHost(t *testing.T, admission *stubAdmission) (*Host, *httptest.Server, string) {
	t.Helper()
	kind := app.ManifestVersionKind{
		Kind:         "Widget",
		Plural:       "Widgets",
		Scope:        "Namespaced",
		Admission:    &app.AdmissionCapabilities{Validation: &app.ValidationCapability{Operations: []app.AdmissionOperation{app.AdmissionOperationCreate, app.AdmissionOperationUpdate}}},
		SecureValues: []app.ManifestVersionKindSecureValue{{Key: "*"}},
	}
	host, err := New(Config{Topology: TopologyCloud, PluginID: testPluginID, Group: testGroup, Version: testVersion, Kinds: []app.ManifestVersionKind{kind}})
	require.NoError(t, err)
	served, err := host.ServeInProcess(grpcplugin.ServeOpts{
		RouteServer:     appadapter.NewRouteAdapter(stubApp{handler: http.NotFoundHandler()}),
		AdmissionServer: admission,
		Authenticator:   host.Authenticator(),
		PluginID:        testPluginID,
	})
	require.NoError(t, err)
	server := httptest.NewServer(served.Handler())
	t.Cleanup(func() {
		server.Close()
		served.Close()
	})
	token, err := host.ServiceToken("svc", testNamespace)
	require.NoError(t, err)
	return host, server, token
}

func widgetURL(server *httptest.Server, name string) string {
	url := server.URL + "/apis/" + testGroup + "/" + testVersion + "/namespaces/" + testNamespace + "/widgets"
	if name != "" {
		url += "/" + name
	}
	return url
}

func sendWidget(t *testing.T, method, url, token, body string) (int, map[string]any) {
	t.Helper()
	request, err := http.NewRequest(method, url, strings.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("X-Access-Token", token)
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	var decoded map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&decoded))
	return response.StatusCode, decoded
}

func widget(secure string) string {
	body := `{"metadata":{"name":"w"},"spec":{"size":1}`
	if secure != "" {
		body += `,"secure":` + secure
	}
	return body + "}"
}

func TestHost_StoresManifestKinds(t *testing.T) {
	admission := &stubAdmission{}
	host, server, token := newKindHost(t, admission)

	code, created := sendWidget(t, http.MethodPost, widgetURL(server, ""), token, widget(`{"key":{"create":"`+widgetKey+`"}}`))
	require.Equal(t, http.StatusCreated, code, created)
	name := created["secure"].(map[string]any)["key"].(map[string]any)["name"].(string)
	assert.Equal(t, map[string]any{"key": map[string]any{"name": name}}, created["secure"])
	value, ok := host.SecureValue(testNamespace, name)
	require.True(t, ok)
	assert.Equal(t, widgetKey, value)

	code, updated := sendWidget(t, http.MethodPut, widgetURL(server, "w"), token, widget(""))
	require.Equal(t, http.StatusOK, code, updated)
	assert.Equal(t, created["secure"], updated["secure"], "omitted secure values are kept")

	code, read := sendWidget(t, http.MethodGet, widgetURL(server, "w"), token, "")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, created["secure"], read["secure"])
	assert.Equal(t, testNamespace, read["metadata"].(map[string]any)["namespace"])

	code, replaced := sendWidget(t, http.MethodPut, widgetURL(server, "w"), token, widget(`{"key":{"create":"other"}}`))
	require.Equal(t, http.StatusOK, code, replaced)
	replacement := replaced["secure"].(map[string]any)["key"].(map[string]any)["name"].(string)
	assert.NotEqual(t, name, replacement)
	_, ok = host.SecureValue(testNamespace, name)
	assert.False(t, ok, "a replaced secret is deleted")

	code, removed := sendWidget(t, http.MethodPut, widgetURL(server, "w"), token, widget(`{"key":{"remove":true}}`))
	require.Equal(t, http.StatusOK, code, removed)
	assert.NotContains(t, removed, "secure")

	code, _ = sendWidget(t, http.MethodDelete, widgetURL(server, "w"), token, "")
	require.Equal(t, http.StatusOK, code)
	code, _ = sendWidget(t, http.MethodGet, widgetURL(server, "w"), token, "")
	assert.Equal(t, http.StatusNotFound, code)

	reviews := admission.recorded()
	require.Len(t, reviews, 4, "delete is not declared, so it is not reviewed")
	assert.Equal(t, pluginv3.AdmissionReviewRequest_OPERATION_CREATE, reviews[0].operation)
	assert.Contains(t, reviews[0].object, widgetKey, "admission sees the incoming create value")
	assert.Empty(t, reviews[0].old)
	assert.Equal(t, pluginv3.AdmissionReviewRequest_OPERATION_UPDATE, reviews[1].operation)
	assert.Contains(t, reviews[1].old, name, "admission sees the stored names")
	assert.NotContains(t, reviews[1].old, widgetKey)
}

func TestHost_RefusesDeniedKindWrites(t *testing.T) {
	admission := &stubAdmission{deny: true}
	host, server, token := newKindHost(t, admission)

	code, status := sendWidget(t, http.MethodPost, widgetURL(server, ""), token, widget(`{"key":{"create":"`+widgetKey+`"}}`))
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Equal(t, "admission hook for Widget denied the request: widget refused", status["message"])
	code, _ = sendWidget(t, http.MethodGet, widgetURL(server, "w"), token, "")
	assert.Equal(t, http.StatusNotFound, code)
	host.mu.Lock()
	defer host.mu.Unlock()
	assert.Empty(t, host.kinds.secrets)
}

func TestHost_KindNamespaceAndMethods(t *testing.T) {
	_, server, token := newKindHost(t, &stubAdmission{})

	other := strings.Replace(widgetURL(server, ""), testNamespace, "stacks-2", 1)
	request, err := http.NewRequest(http.MethodPost, other, strings.NewReader(widget("")))
	require.NoError(t, err)
	request.Header.Set("X-Access-Token", token)
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	_ = response.Body.Close()
	assert.Equal(t, http.StatusForbidden, response.StatusCode)

	code, _ := sendWidget(t, http.MethodPatch, widgetURL(server, "w"), token, "{}")
	assert.Equal(t, http.StatusMethodNotAllowed, code)
	code, _ = sendWidget(t, http.MethodPost, widgetURL(server, ""), token, `{"metadata":{}}`)
	assert.Equal(t, http.StatusBadRequest, code)
	code, _ = sendWidget(t, http.MethodPost, widgetURL(server, ""), token, `null`)
	assert.Equal(t, http.StatusBadRequest, code)
}

func TestKindStore_RefusesWritesToAChangedObject(t *testing.T) {
	store := newKindStore(nil)
	key := objectKey{plural: "widgets", namespace: testNamespace, name: "w"}
	require.NoError(t, store.store(key, map[string]any{"metadata": map[string]any{"name": "w"}}, nil))
	reviewed := store.objects[key]
	assert.True(t, store.unchanged(key, reviewed))
	require.NoError(t, store.store(key, map[string]any{"metadata": map[string]any{"name": "w"}}, reviewed))
	assert.False(t, store.unchanged(key, reviewed))
	delete(store.objects, key)
	assert.False(t, store.unchanged(key, reviewed))
}

func TestHost_KeepsNumbersAsWritten(t *testing.T) {
	admission := &stubAdmission{}
	_, server, token := newKindHost(t, admission)
	const exact = "9007199254740993"
	body := `{"metadata":{"name":"w"},"spec":{"size":` + exact + `,"ratio":1.50}}`

	request, err := http.NewRequest(http.MethodPost, widgetURL(server, ""), strings.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("X-Access-Token", token)
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	stored, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, response.StatusCode, string(stored))
	assert.Contains(t, string(stored), `"size":`+exact)
	assert.Contains(t, string(stored), `"ratio":1.50`)
	reviews := admission.recorded()
	require.Len(t, reviews, 1)
	assert.Contains(t, reviews[0].object, `"size":`+exact, "admission sees the number as written")
}
